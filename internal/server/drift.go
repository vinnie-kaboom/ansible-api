package server

import (
	"ansible-api/internal/githubapp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/src-d/go-git.v4"
)

// NewDriftDetector creates a new drift detector
func NewDriftDetector(server *Server) *DriftDetector {
	return &DriftDetector{
		server:    server,
		stateFile: filepath.Join(os.TempDir(), "default_system_state.json"),
		logger:    log.With().Str("component", "drift").Logger(),
	}
}

// Start begins the drift detection process
func (d *DriftDetector) Start() {
	go d.run()
}

// run executes drift detection in a loop
func (d *DriftDetector) run() {
	// Run first detection immediately
	d.detect()

	// Then run every 3 minutes
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		d.detect()
	}
}

// detect performs drift detection on all playbooks
func (d *DriftDetector) detect() {
	d.logger.Info().Msg("Starting drift detection")

	state, err := d.loadState()
	if err != nil {
		d.logger.Error().Err(err).Msg("Failed to load state file")
		return
	}

	d.logger.Info().Int("playbook_count", len(state)).Msg("Loaded playbooks from state file")

	changed := false
	for logicalPath, playbookState := range state {
		if d.checkPlaybookDrift(logicalPath, &playbookState) {
			state[logicalPath] = playbookState
			changed = true
		}
	}

	if changed {
		if err := d.saveState(state); err != nil {
			d.logger.Error().Err(err).Msg("Failed to save state file")
		}
	}
}

// checkPlaybookDrift checks for drift in a single playbook
func (d *DriftDetector) checkPlaybookDrift(logicalPath string, playbookState *PlaybookState) bool {
	d.logger.Info().Str("playbook", logicalPath).Msg("Checking playbook for drift")

	// Get current commit hash
	currentCommitHash, err := d.getRemoteCommitHash(playbookState.Repo, "main")
	if err != nil {
		d.logger.Warn().Str("repo", playbookState.Repo).Err(err).Msg("Failed to get remote commit hash")
		currentCommitHash = ""
	}

	// Check if repository has changed
	repoChanged := playbookState.PlaybookCommit != currentCommitHash

	// Always run infrastructure checks - simpler and more reliable
	shouldRunCheck := true

	// Log the decision
	if playbookState.PlaybookCommit != "" {
		if repoChanged {
			d.logger.Info().
				Str("playbook", logicalPath).
				Str("old_commit", playbookState.PlaybookCommit).
				Str("new_commit", currentCommitHash).
				Msg("Repository changed - running drift check")
		} else {
			d.logger.Info().
				Str("playbook", logicalPath).
				Str("last_full_check", playbookState.LastFullCheck).
				Int("interval_minutes", d.server.Config.DriftPeriodicCheckInterval).
				Msg("Running periodic infrastructure check")
		}
	}

	// Run drift check if needed
	var driftDetected bool
	var remediationStatus string
	var remediationTime string

	if shouldRunCheck {
		driftDetected, remediationStatus, remediationTime = d.runDriftCheck(logicalPath, playbookState)
	} else {
		driftDetected = false
		remediationStatus = "ok"
		remediationTime = ""
	}

	// Update playbook state
	hash, _ := d.fileHash(filepath.Join(os.TempDir(), logicalPath))
	*playbookState = PlaybookState{
		Repo:                  playbookState.Repo,
		LastRun:               time.Now().UTC().Format(time.RFC3339),
		LastHash:              hash,
		LastStatus:            remediationStatus,
		LastRemediation:       remediationTime,
		LastRemediationStatus: remediationStatus,
		DriftDetected:         driftDetected,
		LastTargets:           []string{},
		PlaybookCommit:        currentCommitHash,
		TargetHosts:           playbookState.TargetHosts,
		LastFullCheck:         time.Now().UTC().Format(time.RFC3339), // Update last full check time
	}

	d.logger.Info().
		Str("playbook", logicalPath).
		Bool("drift_detected", driftDetected).
		Str("status", remediationStatus).
		Bool("repo_changed", repoChanged).
		Msg("Drift check completed")

	return true
}

// runDriftCheck executes Ansible check mode and remediation if needed
func (d *DriftDetector) runDriftCheck(logicalPath string, playbookState *PlaybookState) (bool, string, string) {
	tmpDir, err := os.MkdirTemp("", "repo-drift-")
	if err != nil {
		d.logger.Error().Err(err).Msg("Failed to create temp directory")
		return false, "error", ""
	}
	defer os.RemoveAll(tmpDir)

	// Clone repository
	if err := d.cloneRepository(playbookState.Repo, tmpDir); err != nil {
		d.logger.Error().Err(err).Msg("Failed to clone repository")
		return false, "error", ""
	}

	// Find playbook and inventory files
	playbookPath := filepath.Join(tmpDir, logicalPath)
	inventoryPath, err := d.findInventoryFile(tmpDir)
	if err != nil {
		d.logger.Error().Err(err).Msg("Failed to find inventory file")
		return false, "error", ""
	}

	// Run Ansible check mode
	driftDetected, remediationStatus, remediationTime := d.runAnsibleCheck(playbookPath, inventoryPath, playbookState.TargetHosts)

	return driftDetected, remediationStatus, remediationTime
}

// cloneRepository clones a repository using GitHub App authentication
func (d *DriftDetector) cloneRepository(repoURL, tmpDir string) error {
	token, err := d.getGitHubToken()
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	repoPath := d.extractRepoPath(repoURL)
	host := d.extractHost(repoURL)
	cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

	d.logger.Info().Str("repo", repoURL).Msg("Cloning repository")

	_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
		URL: cloneURL,
	})

	return err
}

// findInventoryFile locates the inventory file in the repository
func (d *DriftDetector) findInventoryFile(tmpDir string) (string, error) {
	// Check for inventory/hosts.ini first
	inventoryPath := filepath.Join(tmpDir, "inventory", "hosts.ini")
	if _, err := os.Stat(inventoryPath); err == nil {
		return inventoryPath, nil
	}

	// Fallback to inventory.ini in root
	inventoryPath = filepath.Join(tmpDir, "inventory.ini")
	if _, err := os.Stat(inventoryPath); err == nil {
		return inventoryPath, nil
	}

	return "", fmt.Errorf("no inventory file found")
}

// runAnsibleCheck executes Ansible check mode and handles remediation
func (d *DriftDetector) runAnsibleCheck(playbookPath, inventoryPath, targetHosts string) (bool, string, string) {
	d.logger.Info().Str("playbook", playbookPath).Msg("Running Ansible check mode")

	cmd := exec.Command("ansible-playbook", playbookPath, "--check", "--diff", "--inventory", inventoryPath)
	if targetHosts != "" {
		cmd.Args = append(cmd.Args, "--limit", targetHosts)
	}

	// Set working directory to the cloned repository root
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(playbookPath))) // Go up from playbooks/webservers/deploy.yml to repo root
	cmd.Dir = repoRoot

	// Set environment variables for authentication (same as processor.go)
	rolesPath := filepath.Join(repoRoot, "roles") + ":" + filepath.Join(repoRoot, "playbooks", "roles") + ":~/.ansible/roles:/usr/share/ansible/roles:/etc/ansible/roles"
	cmd.Env = append(os.Environ(),
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_ROLES_PATH="+rolesPath,
	)

	// Pass SSH credentials from Vault via environment variables (same as processor.go)
	if d.server != nil && d.server.VaultClient != nil {
		if credentials, err := d.server.VaultClient.GetSecret("ansible/credentials"); err == nil {
			if username, ok := credentials["username"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_SSH_USER="+username.(string))
			}
			if password, ok := credentials["password"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_SSH_PASSWORD="+password.(string))
			}
			if sudoPassword, ok := credentials["sudo_password"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_BECOME_PASSWORD="+sudoPassword.(string))
			}
		}
	}

	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)

	d.logAnsibleSummary(playbookPath, output)

	if err != nil {
		d.logger.Error().Str("playbook", playbookPath).Str("ansible_output", output).Err(err).Msg("Ansible check mode failed")
		return false, "error", ""
	}

	// Check for changes
	if strings.Contains(output, "changed=0") {
		d.logger.Info().Str("playbook", playbookPath).Msg("No drift detected")
		return false, "ok", ""
	}

	if strings.Contains(output, "changed=") && !strings.Contains(output, "changed=0") {
		if d.areChangesIgnorable(output) {
			d.logger.Info().Str("playbook", playbookPath).Msg("No drift - ignorable changes only")
			return false, "ok", ""
		}

		// Log the specific changes that triggered drift detection for debugging
		d.logger.Warn().Str("playbook", playbookPath).Str("ansible_output", output).Msg("Drift detected - running remediation")
		remediationStatus, remediationTime := d.remediateDrift(playbookPath, inventoryPath, targetHosts)
		return true, remediationStatus, remediationTime
	}

	d.logger.Info().Str("playbook", playbookPath).Msg("No drift detected")
	return false, "ok", ""
}

// remediateDrift runs Ansible to fix detected drift
func (d *DriftDetector) remediateDrift(playbookPath, inventoryPath, targetHosts string) (string, string) {
	cmd := exec.Command("ansible-playbook", playbookPath, "--inventory", inventoryPath)
	if targetHosts != "" {
		cmd.Args = append(cmd.Args, "--limit", targetHosts)
	}

	// Set working directory to the cloned repository root
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(playbookPath))) // Go up from playbooks/webservers/deploy.yml to repo root
	cmd.Dir = repoRoot

	// Set environment variables for authentication (same as processor.go)
	rolesPath := filepath.Join(repoRoot, "roles") + ":" + filepath.Join(repoRoot, "playbooks", "roles") + ":~/.ansible/roles:/usr/share/ansible/roles:/etc/ansible/roles"
	cmd.Env = append(os.Environ(),
		"ANSIBLE_HOST_KEY_CHECKING=False",
		"ANSIBLE_ROLES_PATH="+rolesPath,
	)

	// Pass SSH credentials from Vault via environment variables (same as processor.go)
	if d.server != nil && d.server.VaultClient != nil {
		if credentials, err := d.server.VaultClient.GetSecret("ansible/credentials"); err == nil {
			if username, ok := credentials["username"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_SSH_USER="+username.(string))
			}
			if password, ok := credentials["password"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_SSH_PASSWORD="+password.(string))
			}
			if sudoPassword, ok := credentials["sudo_password"]; ok {
				cmd.Env = append(cmd.Env, "ANSIBLE_BECOME_PASSWORD="+sudoPassword.(string))
			}
		}
	}

	_, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Error().Str("playbook", playbookPath).Err(err).Msg("Ansible remediation failed")
		return "error", time.Now().UTC().Format(time.RFC3339)
	}

	d.logger.Info().Str("playbook", playbookPath).Msg("Ansible remediation completed successfully")
	return "ok", time.Now().UTC().Format(time.RFC3339)
}

// getGitHubToken retrieves a GitHub App installation token
func (d *DriftDetector) getGitHubToken() (string, error) {
	return (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
		AppID:          d.server.GithubAppID,
		InstallationID: d.server.GithubInstallationID,
		PrivateKey:     d.server.GithubPrivateKey,
		APIBaseURL:     d.server.GithubAPIBaseURL,
	})
}

// getRemoteCommitHash gets the current commit hash from a remote repository
func (d *DriftDetector) getRemoteCommitHash(repoURL, branch string) (string, error) {
	d.logger.Debug().Str("repo", repoURL).Str("branch", branch).Msg("Getting remote commit hash")

	token, err := d.getGitHubToken()
	if err != nil {
		d.logger.Error().Err(err).Str("repo", repoURL).Msg("Failed to get GitHub token for remote commit check")
		return "", fmt.Errorf("failed to authenticate with GitHub: %w", err)
	}

	repoPath := d.extractRepoPath(repoURL)
	host := d.extractHost(repoURL)
	cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

	d.logger.Debug().Str("repo", repoURL).Str("clone_url", maskTokenInURL(cloneURL)).Msg("Executing git ls-remote")

	// Create command with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", cloneURL, branch)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			d.logger.Error().Str("repo", repoURL).Msg("Git ls-remote timed out after 30 seconds")
			return "", fmt.Errorf("git ls-remote timed out: %w", err)
		}
		d.logger.Error().Err(err).Str("repo", repoURL).Msg("Git ls-remote failed")
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		d.logger.Error().Str("repo", repoURL).Msg("No output from git ls-remote")
		return "", fmt.Errorf("no output from git ls-remote")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		d.logger.Error().Str("repo", repoURL).Str("output", string(output)).Msg("Unexpected git ls-remote output format")
		return "", fmt.Errorf("unexpected output format from git ls-remote")
	}

	commitHash := parts[0]
	d.logger.Debug().Str("repo", repoURL).Str("commit", commitHash).Msg("Successfully retrieved remote commit hash")
	return commitHash, nil
}

// extractRepoPath extracts the repository path from a URL
func (d *DriftDetector) extractRepoPath(repoURL string) string {
	// Simple extraction - assumes format like https://github.com/user/repo.git
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return repoURL
}

// extractHost extracts the host from a URL
func (d *DriftDetector) extractHost(repoURL string) string {
	// Simple extraction - assumes format like https://github.com/user/repo.git
	if strings.HasPrefix(repoURL, "https://") {
		parts := strings.Split(repoURL[8:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "github.com"
}

// fileHash calculates SHA256 hash of a file
func (d *DriftDetector) fileHash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// loadState loads the state file
func (d *DriftDetector) loadState() (StateFile, error) {
	state := make(StateFile)

	f, err := os.Open(d.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := dec.Decode(&state); err != nil {
		return nil, err
	}

	return state, nil
}

// saveState saves the state file
func (d *DriftDetector) saveState(state StateFile) error {
	f, err := os.Create(d.stateFile)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

// logAnsibleSummary logs a clean summary of Ansible output
func (d *DriftDetector) logAnsibleSummary(playbook, output string) {
	lines := strings.Split(output, "\n")
	var recapLine string
	for _, line := range lines {
		if strings.Contains(line, "PLAY RECAP") {
			recapLine = strings.TrimSpace(line)
			break
		}
	}

	changedCount := "0"
	if strings.Contains(output, "changed=") {
		changedMatch := regexp.MustCompile(`changed=(\d+)`).FindStringSubmatch(output)
		if len(changedMatch) > 1 {
			changedCount = changedMatch[1]
		}
	}

	d.logger.Info().
		Str("playbook", playbook).
		Str("changed_count", changedCount).
		Str("summary", recapLine).
		Msg("Ansible check mode completed")
}

// areChangesIgnorable checks if all changes in Ansible output are ignorable
func (d *DriftDetector) areChangesIgnorable(output string) bool {
	// Patterns that indicate ignorable changes
	ignorablePatterns := []string{
		"atime", "mtime", "ctime",
		"ansible_date_time", "generated by ansible",
		"timestamp", "last_modified", "last_updated",
		"access_time", "modification_time", "creation_time",
		"ansible date and time", "date and time",
		"iso8601", "utc", "gmt", "timezone",
		"deployed:", "deployment time", "creation date",
		"verify", "health_check", "check", "test", "ping", "curl", "wget",
		"uri", "get_url", "stat", "find", "command", "shell",
		"ansible_facts", "gather_facts", "setup",
		"accessible", "deployment results", "display", "debug",
		"create custom index page", "index page", "index.html",
		// Ansible temporary paths and command arguments
		"/tmp/repo-drift-", "included:", "command arguments",
		"-t", "nginx -t", "test", "validation",
		"item=", "ansible-local-", "tmpkd4k35jk", "tmpro_kr2vk",
	}

	// Comprehensive regex patterns for timestamp and dynamic content changes
	dynamicContentRegexes := []*regexp.Regexp{
		// ISO 8601 timestamps (most common)
		regexp.MustCompile(`^\s*[+-].*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z?`),
		// Date formats
		regexp.MustCompile(`^\s*[+-].*\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}`),
		// Unix timestamps
		regexp.MustCompile(`^\s*[+-].*\b\d{10,13}\b`),
		// Common timestamp keywords with dates
		regexp.MustCompile(`^\s*[+-].*(?:deployed|created|modified|updated|generated):\s*\d{4}-\d{2}-\d{2}`),
		// Specific "Deployed:" timestamp pattern from templates
		regexp.MustCompile(`^\s*[+-].*Deployed:\s*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`),
		// ansible_date_time variables
		regexp.MustCompile(`^\s*[+-].*ansible_date_time\.(iso8601|epoch|date|time)`),
		// Additional timestamp patterns that could appear in templates
		regexp.MustCompile(`^\s*[+-].*(?:timestamp|time|date|created_at|updated_at|last_modified):\s*\d{4}-\d{2}-\d{2}`),
		// RFC 3339 timestamps with timezone offsets
		regexp.MustCompile(`^\s*[+-].*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?[+-]\d{2}:\d{2}`),
		// Human-readable date formats
		regexp.MustCompile(`^\s*[+-].*(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+\d{1,2},\s+\d{4}`),
		// ISO date only (without time) - simplified without lookahead
		regexp.MustCompile(`^\s*[+-].*\d{4}-\d{2}-\d{2}\s*$`),
		// Time-only patterns (HH:MM:SS)
		regexp.MustCompile(`^\s*[+-].*\d{2}:\d{2}:\d{2}(?:\.\d{3})?`),
		// Epoch timestamps (10 or 13 digits)
		regexp.MustCompile(`^\s*[+-].*\b\d{10,13}\b`),
		// Template variables that might contain timestamps
		regexp.MustCompile(`^\s*[+-].*\{\{.*(?:date|time|timestamp).*\}\}`),
		// HTML class attributes with timestamps
		regexp.MustCompile(`^\s*[+-].*class=".*timestamp.*".*\d{4}-\d{2}-\d{2}`),
		// Generic dynamic IDs or session tokens
		regexp.MustCompile(`^\s*[+-].*(?:id|token|session|uuid).*[a-f0-9]{8,32}`),
		// Common dynamic content patterns
		regexp.MustCompile(`^\s*[+-].*(?:version|build|commit|hash|checksum).*[a-f0-9]{6,40}`),
		// Random strings and hashes
		regexp.MustCompile(`^\s*[+-].*[a-f0-9]{16,64}`),
		// Process IDs and temporary file paths
		regexp.MustCompile(`^\s*[+-].*(?:pid|process).*\d{1,6}`),
		// Ansible temporary paths and directories
		regexp.MustCompile(`^\s*[+-].*/tmp/repo-drift-\d+/`),
		regexp.MustCompile(`^\s*[+-].*ansible-local-\d+[a-z0-9_]+/`),
		regexp.MustCompile(`^\s*[+-].*tmp[a-z0-9_]+/`),
		// Ansible command arguments and included tasks
		regexp.MustCompile(`^\s*[+-].*included:\s*/tmp/repo-drift-\d+/`),
		regexp.MustCompile(`^\s*[+-].*\(item=/tmp/repo-drift-\d+/`),
		regexp.MustCompile(`^\s*[+-].*"-t"`),
	}

	lines := strings.Split(output, "\n")
	hasNonIgnorableChanges := false

	d.logger.Debug().Msg("Starting areChangesIgnorable analysis")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)

		// Skip headers and metadata
		if line == "" ||
			strings.HasPrefix(line, "---") ||
			strings.HasPrefix(line, "+++") ||
			strings.HasPrefix(line, "@@") ||
			strings.HasPrefix(line, "PLAY") ||
			strings.HasPrefix(line, "TASK") ||
			strings.HasPrefix(line, "PLAY RECAP") ||
			strings.HasPrefix(line, "#") {
			continue
		}

		// Check for specific task names that are inherently non-idempotent
		if strings.Contains(lineLower, "task [") {
			for _, pattern := range ignorablePatterns {
				if strings.Contains(lineLower, pattern) {
					// This entire task is ignorable, skip ahead
					goto nextLine
				}
			}
		}

		// Check if change is ignorable (diff lines)
		if strings.Contains(line, "-") || strings.Contains(line, "+") {
			d.logger.Debug().Str("diff_line", line).Msg("Processing diff line")
			isIgnorable := false

			// First check regex patterns for dynamic content matching (if enabled)
			if d.server != nil && d.server.Config != nil && d.server.Config.DriftIgnoreDynamicContent {
				for i, regex := range dynamicContentRegexes {
					if regex.MatchString(line) {
						d.logger.Debug().Str("line", line).Int("regex_index", i).Msg("Ignoring dynamic content change")
						isIgnorable = true
						break
					}
				}
			}

			// Then check general ignorable patterns
			if !isIgnorable {
				for _, pattern := range ignorablePatterns {
					if strings.Contains(lineLower, pattern) {
						d.logger.Debug().Str("line", line).Str("pattern", pattern).Msg("Ignoring change due to ignorable pattern")
						isIgnorable = true
						break
					}
				}
			}

			// Additional safety check: if line contains service-related keywords, don't ignore
			if isIgnorable {
				serviceKeywords := []string{"service", "systemctl", "start", "stop", "restart", "enable", "disable"}
				for _, keyword := range serviceKeywords {
					if strings.Contains(lineLower, keyword) {
						d.logger.Debug().Str("line", line).Str("keyword", keyword).Msg("Not ignoring service-related change")
						isIgnorable = false
						break
					}
				}
			}

			if !isIgnorable {
				d.logger.Debug().Str("non_ignorable_line", line).Msg("Found non-ignorable change")
				hasNonIgnorableChanges = true
			}
		}

	nextLine:
	}

	result := !hasNonIgnorableChanges
	d.logger.Debug().Bool("all_changes_ignorable", result).Bool("has_non_ignorable", hasNonIgnorableChanges).Msg("areChangesIgnorable result")
	return result
}

// UpdatePlaybookState updates the state for a specific playbook
func (d *DriftDetector) UpdatePlaybookState(logicalPath, fullPath, repo, status, targetHosts string) error {
	d.logger.Info().Str("logicalPath", logicalPath).Str("fullPath", fullPath).Msg("Updating playbook state")

	hash, err := d.fileHash(fullPath)
	if err != nil {
		d.logger.Error().Err(err).Msg("Failed to hash file")
		return err
	}

	commitHash, err := d.getRemoteCommitHash(repo, "main")
	if err != nil {
		d.logger.Warn().Err(err).Msg("Failed to get remote commit hash")
		commitHash = ""
	}

	state, err := d.loadState()
	if err != nil {
		return err
	}

	state[logicalPath] = PlaybookState{
		Repo:           repo,
		LastRun:        time.Now().UTC().Format(time.RFC3339),
		LastHash:       hash,
		LastStatus:     status,
		PlaybookCommit: commitHash,
		TargetHosts:    targetHosts,
	}

	if err := d.saveState(state); err != nil {
		d.logger.Error().Err(err).Msg("Failed to save state file")
		return err
	}

	d.logger.Info().Str("stateFile", d.stateFile).Msg("State file updated successfully")
	return nil
}

// RemovePlaybookState removes a playbook from the state
func (d *DriftDetector) RemovePlaybookState(playbookPath string) error {
	state, err := d.loadState()
	if err != nil {
		return err
	}

	delete(state, playbookPath)
	return d.saveState(state)
}

// Legacy functions for backward compatibility
func UpdatePlaybookState(server *Server, logicalPath, fullPath, repo, status, targetHosts string) error {
	detector := NewDriftDetector(server)
	return detector.UpdatePlaybookState(logicalPath, fullPath, repo, status, targetHosts)
}

func RemovePlaybookState(playbookPath string) error {
	detector := NewDriftDetector(nil) // Will use default state file
	return detector.RemovePlaybookState(playbookPath)
}

func StartDriftDetection(server *Server) {
	detector := NewDriftDetector(server)
	detector.Start()
}
