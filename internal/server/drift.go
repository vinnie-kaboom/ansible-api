package server

import (
	"ansible-api/internal/githubapp"
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
	// Check if drift detection is disabled
	if os.Getenv("DISABLE_DRIFT_DETECTION") == "true" {
		d.logger.Info().Msg("Drift detection disabled by environment variable")
		return
	}

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

	// Log commit hash changes
	if playbookState.PlaybookCommit != "" && currentCommitHash != "" {
		if playbookState.PlaybookCommit != currentCommitHash {
			d.logger.Info().
				Str("playbook", logicalPath).
				Str("old_commit", playbookState.PlaybookCommit).
				Str("new_commit", currentCommitHash).
				Msg("Commit hash changed")
		} else {
			d.logger.Debug().
				Str("playbook", logicalPath).
				Str("commit", currentCommitHash).
				Msg("Commit hash unchanged")
		}
	}

	// Run drift check
	driftDetected, remediationStatus, remediationTime := d.runDriftCheck(logicalPath, playbookState)

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
	}

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

	// Load SSH key from Vault for authentication
	sshKeyPath := ""
	if sshKey, err := d.server.VaultClient.GetSecret("ansible/ssh-key"); err == nil {
		if privateKey, exists := sshKey["private_key"]; exists {
			if privateKeyStr, ok := privateKey.(string); ok {
				// Create temporary SSH key file
				tmpKeyFile, err := os.CreateTemp("", "ansible-ssh-key-*")
				if err == nil {
					defer os.Remove(tmpKeyFile.Name())

					if err := tmpKeyFile.Chmod(0600); err == nil {
						if _, err := tmpKeyFile.WriteString(privateKeyStr); err == nil {
							tmpKeyFile.Close()
							sshKeyPath = tmpKeyFile.Name()
						}
					}
				}
			}
		}
	}

	// Set environment variables to eliminate warnings and sudo password prompts
	envVars := []string{
		"ANSIBLE_PYTHON_INTERPRETER=" + d.getPythonInterpreter(),
		"ANSIBLE_HOST_KEY_CHECKING=False",
	}

	// Add become password from Vault if available, or empty for passwordless sudo
	if sudoPassword := d.getSudoPassword(); sudoPassword != "" {
		envVars = append(envVars, "ANSIBLE_BECOME_PASSWORD="+sudoPassword)
		d.logger.Debug().Msg("Using sudo password from Vault for privilege escalation")
	} else {
		// Set empty password for passwordless sudo
		envVars = append(envVars, "ANSIBLE_BECOME_PASSWORD=")
		d.logger.Debug().Msg("Using empty password for passwordless sudo")
	}

	cmd.Env = append(os.Environ(), envVars...)

	// Add SSH key to Ansible command if available
	if sshKeyPath != "" {
		cmd.Args = append(cmd.Args, "--private-key", sshKeyPath)
	}

	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)

	d.logAnsibleSummary(playbookPath, output)

	// Check for changes first, even if there was an error
	// Ansible often returns non-zero exit codes when changes would be made
	hasChanges := strings.Contains(output, "changed=") && !strings.Contains(output, "changed=0")

	if err != nil {
		d.logger.Warn().Str("playbook", playbookPath).Err(err).Str("output", output).Msg("Ansible check mode returned non-zero exit code")

		// If there are no changes and we have an error, it's likely a real failure
		if !hasChanges {
			d.logger.Error().Str("playbook", playbookPath).Err(err).Msg("Ansible check mode failed with no changes detected")
			return false, "error", ""
		}
		// If there are changes, continue with drift detection despite the error
		d.logger.Info().Str("playbook", playbookPath).Msg("Check mode error likely due to drift - proceeding with drift analysis")
	}

	// Enhanced logging to show exactly what's changing
	if hasChanges {
		d.logger.Info().Str("playbook", playbookPath).Str("full_output", output).Msg("Full Ansible check output for drift analysis")

		// Parse and log specific changes
		changes := d.parseAnsibleChanges(output)
		for _, change := range changes {
			d.logger.Info().
				Str("playbook", playbookPath).
				Str("task", change.Task).
				Str("host", change.Host).
				Str("change_type", change.Type).
				Str("details", change.Details).
				Msg("Detected change in drift check")
		}

		if d.areChangesIgnorable(output) {
			d.logger.Info().Str("playbook", playbookPath).Msg("No drift - ignorable changes only")
			return false, "ok", ""
		}

		d.logger.Warn().Str("playbook", playbookPath).Msg("Drift detected - running remediation")
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

	// Load SSH key from Vault for authentication
	sshKeyPath := ""
	if sshKey, err := d.server.VaultClient.GetSecret("ansible/ssh-key"); err == nil {
		if privateKey, exists := sshKey["private_key"]; exists {
			if privateKeyStr, ok := privateKey.(string); ok {
				// Create temporary SSH key file
				tmpKeyFile, err := os.CreateTemp("", "ansible-ssh-key-*")
				if err == nil {
					defer os.Remove(tmpKeyFile.Name())

					if err := tmpKeyFile.Chmod(0600); err == nil {
						if _, err := tmpKeyFile.WriteString(privateKeyStr); err == nil {
							tmpKeyFile.Close()
							sshKeyPath = tmpKeyFile.Name()
						}
					}
				}
			}
		}
	}

	// Set environment variables to eliminate warnings and sudo password prompts
	envVars := []string{
		"ANSIBLE_PYTHON_INTERPRETER=" + d.getPythonInterpreter(),
		"ANSIBLE_HOST_KEY_CHECKING=False",
	}

	// Add become password from Vault if available, or empty for passwordless sudo
	if sudoPassword := d.getSudoPassword(); sudoPassword != "" {
		envVars = append(envVars, "ANSIBLE_BECOME_PASSWORD="+sudoPassword)
		d.logger.Debug().Msg("Using sudo password from Vault for privilege escalation")
	} else {
		// Set empty password for passwordless sudo
		envVars = append(envVars, "ANSIBLE_BECOME_PASSWORD=")
		d.logger.Debug().Msg("Using empty password for passwordless sudo")
	}

	cmd.Env = append(os.Environ(), envVars...)

	// Add SSH key to Ansible command if available
	if sshKeyPath != "" {
		cmd.Args = append(cmd.Args, "--private-key", sshKeyPath)
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
	token, err := d.getGitHubToken()
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with GitHub: %w", err)
	}

	repoPath := d.extractRepoPath(repoURL)
	host := d.extractHost(repoURL)
	cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

	cmd := exec.Command("git", "ls-remote", "--heads", cloneURL, branch)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no output from git ls-remote")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected output format from git ls-remote")
	}

	return parts[0], nil
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
	d.logger.Debug().Str("output_length", fmt.Sprintf("%d", len(output))).Msg("Checking if changes are ignorable")

	ignorablePatterns := []string{
		// Time-related patterns
		"atime", "mtime", "ctime",
		"ansible_date_time", "generated by ansible",
		"timestamp", "last_modified", "last_updated",
		"access_time", "modification_time", "creation_time",
		"ansible date and time", "date and time",
		"iso8601", "utc", "gmt", "timezone",

		// File system patterns
		"state", "touch", "file",
		"permission", "mode", "owner", "group",
		"selinux", "context", "attributes",

		// Service patterns
		"service", "systemd", "enabled", "disabled",
		"started", "stopped", "restarted",

		// Package metadata patterns (but not actual install/removal)
		"package cache", "metadata", "repository updated",

		// Network patterns
		"interface", "ip", "address", "network",
		"hostname", "fqdn", "domain",

		// Common false positives
		"changed_when", "failed_when",
		"register", "set_fact", "include_vars",
		"debug", "msg", "var",

		// Ansible internal patterns
		"ansible_", "gather_facts", "setup",
		"inventory", "hostvars", "groupvars",

		// File content patterns that are often safe
		"content", "src", "dest", "path",
		"backup", "force", "validate",
	}

	lines := strings.Split(output, "\n")
	ignorableCount := 0
	nonIgnorableCount := 0

	// First check for significant changes that should not be ignored
	hasPackageChanges := strings.Contains(output, "TASK [Install") ||
		strings.Contains(output, "TASK [Remove") ||
		strings.Contains(output, "TASK [Uninstall")
	hasServiceChanges := strings.Contains(output, "TASK [Start") ||
		strings.Contains(output, "TASK [Stop") ||
		strings.Contains(output, "TASK [Enable") ||
		strings.Contains(output, "TASK [Disable")

	// If we have package or service changes with actual "changed" status, this is real drift
	if (hasPackageChanges || hasServiceChanges) && strings.Contains(output, "changed: [") {
		d.logger.Info().Msg("Detected significant package or service changes - not ignorable")
		return false
	}

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
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "ok:") ||
			strings.HasPrefix(line, "changed:") ||
			strings.HasPrefix(line, "skipping:") {
			continue
		}

		// Check if change is ignorable
		if strings.Contains(line, "-") || strings.Contains(line, "+") {
			// Skip warning messages and other non-diff content
			if strings.HasPrefix(line, "[WARNING]:") ||
				strings.HasPrefix(line, "[ERROR]:") ||
				strings.HasPrefix(line, "[INFO]:") ||
				strings.Contains(line, "https://") ||
				strings.Contains(line, "http://") ||
				strings.Contains(line, "See") ||
				strings.Contains(line, "for more information") {
				continue
			}

			isIgnorable := false

			// Special handling for timestamp changes
			if strings.Contains(line, "atime") || strings.Contains(line, "mtime") {
				isIgnorable = true
				d.logger.Debug().
					Str("ignorable_timestamp_change", line).
					Msg("Ignoring timestamp change")
			} else if strings.Contains(line, "state") && (strings.Contains(line, "file") || strings.Contains(line, "touch")) {
				isIgnorable = true
				d.logger.Debug().
					Str("ignorable_state_change", line).
					Msg("Ignoring file state change")
			} else if strings.Contains(line, "175052") && (strings.Contains(line, "atime") || strings.Contains(line, "mtime")) {
				// Specific check for the timestamp format we're seeing
				isIgnorable = true
				d.logger.Debug().
					Str("ignorable_timestamp_format", line).
					Msg("Ignoring specific timestamp format change")
			} else {
				// Check against other ignorable patterns
				for _, pattern := range ignorablePatterns {
					if strings.Contains(lineLower, pattern) {
						isIgnorable = true
						break
					}
				}
			}

			if isIgnorable {
				ignorableCount++
				d.logger.Debug().
					Str("ignorable_line", line).
					Msg("Line marked as ignorable")
			} else {
				nonIgnorableCount++
				d.logger.Debug().
					Str("non_ignorable_line", line).
					Msg("Found non-ignorable change")
				return false
			}
		}
	}

	d.logger.Debug().
		Int("ignorable_count", ignorableCount).
		Int("non_ignorable_count", nonIgnorableCount).
		Msg("Finished checking ignorable changes")

	return true
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

// AnsibleChange represents a specific change detected by Ansible
type AnsibleChange struct {
	Task    string
	Host    string
	Type    string
	Details string
}

// parseAnsibleChanges extracts specific changes from Ansible output
func (d *DriftDetector) parseAnsibleChanges(output string) []AnsibleChange {
	var changes []AnsibleChange
	lines := strings.Split(output, "\n")

	var currentTask string
	var currentHost string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Track current task
		if strings.Contains(line, "TASK [") {
			currentTask = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(line, "]"), "TASK ["))
		}

		// Track current host
		if strings.Contains(line, "ok:") || strings.Contains(line, "changed:") || strings.Contains(line, "failed:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				currentHost = strings.TrimSpace(parts[0])
			}
		}

		// Look for diff lines (lines starting with + or -)
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			changeType := "added"
			if strings.HasPrefix(line, "-") {
				changeType = "removed"
			}

			changes = append(changes, AnsibleChange{
				Task:    currentTask,
				Host:    currentHost,
				Type:    changeType,
				Details: line,
			})
		}

		// Look for "changed" lines that indicate what was changed
		if strings.Contains(line, "changed:") && strings.Contains(line, "=>") {
			// Extract the change details
			if strings.Contains(line, "msg:") {
				msgIndex := strings.Index(line, "msg:")
				if msgIndex != -1 {
					details := strings.TrimSpace(line[msgIndex+4:])
					changes = append(changes, AnsibleChange{
						Task:    currentTask,
						Host:    currentHost,
						Type:    "modified",
						Details: details,
					})
				}
			}
		}
	}

	return changes
}

// getPythonInterpreter gets the Python interpreter, using config override if available, otherwise auto-detecting
func (d *DriftDetector) getPythonInterpreter() string {
	// Check if explicitly configured
	if d.server.Config != nil && d.server.Config.PythonInterpreter != "" {
		return d.server.Config.PythonInterpreter
	}

	// Check environment variable override
	if envInterpreter := os.Getenv("ANSIBLE_PYTHON_INTERPRETER_OVERRIDE"); envInterpreter != "" {
		return envInterpreter
	}

	// Auto-detect
	return detectPythonInterpreter()
}

// getSudoPassword retrieves sudo/become password from Vault if available
func (d *DriftDetector) getSudoPassword() string {
	if sudoSecret, err := d.server.VaultClient.GetSecret("ansible/sudo"); err == nil {
		if password, exists := sudoSecret["password"]; exists {
			if passwordStr, ok := password.(string); ok {
				return passwordStr
			}
		}
	}
	return ""
}
