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
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/src-d/go-git.v4"
)

type StateFile map[string]PlaybookState

var stateFile = filepath.Join(os.TempDir(), "default_system_state.json")

// getRemoteCommitHash gets the current commit hash from a remote repository using GitHub App authentication
func getRemoteCommitHash(server *Server, repoURL, branch string) (string, error) {
	// Get GitHub App token
	token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
		AppID:          server.GithubAppID,
		InstallationID: server.GithubInstallationID,
		PrivateKey:     server.GithubPrivateKey,
		APIBaseURL:     server.GithubAPIBaseURL,
	})
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with GitHub: %w", err)
	}

	// Build authenticated clone URL
	repoPath := extractRepoPath(repoURL)
	host := extractHost(repoURL)
	cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

	// Use git ls-remote with the authenticated URL
	cmd := exec.Command("git", "ls-remote", "--heads", cloneURL, branch)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no output from git ls-remote")
	}

	// Parse the output: <hash>\trefs/heads/<branch>
	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected output format from git ls-remote")
	}

	return parts[0], nil
}

func UpdatePlaybookState(server *Server, logicalPath, fullPath, repo string, status string, targetHosts string) error {
	log := log.With().Str("component", "drift").Logger()
	log.Info().Str("logicalPath", logicalPath).Str("fullPath", fullPath).Msg("UpdatePlaybookState called")
	hash, err := fileHash(fullPath)
	if err != nil {
		log.Error().Err(err).Msg("fileHash failed in UpdatePlaybookState")
		return err
	}

	// Get the current commit hash from the remote repository
	commitHash, err := getRemoteCommitHash(server, repo, "main") // You might want to make branch configurable
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get remote commit hash, will use empty string")
		commitHash = ""
	}

	state, _ := loadStateFile(stateFile)
	state[logicalPath] = PlaybookState{
		Repo:           repo,
		LastRun:        time.Now().UTC().Format(time.RFC3339),
		LastHash:       hash,
		LastStatus:     status,
		PlaybookCommit: commitHash,
		TargetHosts:    targetHosts,
	}
	err = saveStateFile(stateFile, state)
	if err != nil {
		log.Error().Err(err).Msg("saveStateFile failed in UpdatePlaybookState")
	} else {
		log.Info().Str("stateFile", stateFile).Msg("State file updated successfully")
	}
	return err
}

func RemovePlaybookState(playbookPath string) error {
	state, _ := loadStateFile(stateFile)
	delete(state, playbookPath)
	return saveStateFile(stateFile, state)
}

func DriftDetection(server *Server) {
	log := log.With().Str("component", "drift").Logger()
	log.Info().Msg("DriftDetection tick")
	state, _ := loadStateFile(stateFile)
	log.Info().Int("playbook_count", len(state)).Msg("Loaded playbooks from state file")
	changed := false
	for logicalPath, ps := range state {
		// Check if the commit hash has changed (for optimization)
		currentCommitHash, err := getRemoteCommitHash(server, ps.Repo, "main") // You might want to make branch configurable
		if err != nil {
			log.Warn().Str("repo", ps.Repo).Err(err).Msg("Failed to get remote commit hash, will still run drift check")
			currentCommitHash = ""
		}

		// Log if commit hash changed (for debugging)
		if ps.PlaybookCommit != "" && ps.PlaybookCommit != currentCommitHash {
			log.Info().Str("playbook", logicalPath).Str("old_commit", ps.PlaybookCommit).Str("new_commit", currentCommitHash).Msg("Commit hash changed")
		} else if ps.PlaybookCommit != "" && ps.PlaybookCommit == currentCommitHash {
			log.Debug().Str("playbook", logicalPath).Str("commit", currentCommitHash).Msg("Commit hash unchanged")
		}

		log.Info().Str("playbook", logicalPath).Msg("Running drift check (Ansible check mode)...")

		tmpDir, err := os.MkdirTemp("", "repo-drift-")
		if err != nil {
			log.Error().Err(err).Msg("Failed to create temp dir for drift check")
			continue
		}
		defer os.RemoveAll(tmpDir)

		// Get GitHub App token for authentication
		token, err := (&githubapp.DefaultAuthenticator{}).GetInstallationToken(githubapp.AuthConfig{
			AppID:          server.GithubAppID,
			InstallationID: server.GithubInstallationID,
			PrivateKey:     server.GithubPrivateKey,
			APIBaseURL:     server.GithubAPIBaseURL,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to authenticate with GitHub for drift check")
			os.RemoveAll(tmpDir)
			continue
		}

		// Build authenticated clone URL
		repoPath := extractRepoPath(ps.Repo)
		host := extractHost(ps.Repo)
		cloneURL := githubapp.BuildCloneURL(token, repoPath, host)

		log.Info().Str("repo", ps.Repo).Msg("Cloning repo for drift check")
		_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL: cloneURL,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to clone repo for drift check")
			os.RemoveAll(tmpDir)
			continue
		}

		playbookFullPath := filepath.Join(tmpDir, logicalPath)
		log.Info().Str("playbook", playbookFullPath).Msg("Running drift check (Ansible check mode)...")
		if _, err := os.Stat(playbookFullPath); os.IsNotExist(err) {
			log.Warn().Str("playbook", playbookFullPath).Msg("Playbook no longer exists in repo, skipping drift check")
			os.RemoveAll(tmpDir)
			continue
		}

		// Handle inventory file (similar to job processor)
		inventoryFilePath := filepath.Join(tmpDir, "inventory", "hosts.ini")
		fallbackInventoryFilePath := filepath.Join("inventory.ini")

		if _, err := os.Stat(inventoryFilePath); os.IsNotExist(err) {
			if _, err := os.Stat(fallbackInventoryFilePath); os.IsNotExist(err) {
				log.Error().Str("playbook", playbookFullPath).Msg("No inventory file found in repository or root directory, skipping drift check for safety")
				os.RemoveAll(tmpDir)
				continue
			} else {
				inventoryFilePath = fallbackInventoryFilePath
			}
		}

		hash, err := fileHash(playbookFullPath)
		if err != nil {
			log.Error().Str("playbook", playbookFullPath).Err(err).Msg("Failed to hash playbook")
			os.RemoveAll(tmpDir)
			continue
		}
		_, drift, remediationStatus, remediationTime := runAnsibleCheckWithOutput(playbookFullPath, inventoryFilePath, ps.TargetHosts, log)
		state[logicalPath] = PlaybookState{
			Repo:                  ps.Repo,
			LastRun:               time.Now().UTC().Format(time.RFC3339),
			LastHash:              hash,
			LastStatus:            remediationStatus,
			LastRemediation:       remediationTime,
			LastRemediationStatus: remediationStatus,
			DriftDetected:         drift,
			LastTargets:           []string{},
			PlaybookCommit:        currentCommitHash,
			TargetHosts:           ps.TargetHosts,
		}
		changed = true
		os.RemoveAll(tmpDir)
	}
	if changed {
		saveStateFile(stateFile, state)
	}
}

func fileHash(filename string) (string, error) {
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

func loadStateFile(filename string) (StateFile, error) {
	state := make(StateFile)
	f, err := os.Open(filename)
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

func saveStateFile(filename string, state StateFile) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

func runAnsibleCheckWithOutput(playbook string, inventory string, targetHosts string, log zerolog.Logger) (bool, bool, string, string) {
	cmd := exec.Command("ansible-playbook", playbook, "--check", "--diff", "--inventory", inventory)
	if targetHosts != "" {
		cmd.Args = append(cmd.Args, "--limit", targetHosts)
	}
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)

	// Log the full Ansible output for debugging
	log.Info().Str("playbook", playbook).Str("output", output).Msg("Full Ansible check mode output")

	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible check mode")
		return false, false, "error", ""
	}

	// Check if Ansible reports no changes
	if strings.Contains(output, "changed=0") {
		log.Info().Str("playbook", playbook).Msg("No changes detected by Ansible")
		return false, false, "ok", ""
	}

	// Check if Ansible reports changes
	if strings.Contains(output, "changed=") && !strings.Contains(output, "changed=0") {
		log.Info().Str("playbook", playbook).Msg("Ansible detected changes, checking if they are ignorable")

		// Check if all changes are ignorable
		if areChangesIgnorable(output) {
			log.Info().Str("playbook", playbook).Msg("All changes are ignorable (timestamps, metadata, etc.), treating as no drift")
			return false, false, "ok", ""
		}

		// Changes are not ignorable, this is true drift
		log.Warn().Str("playbook", playbook).Msg("True drift detected! Running Ansible remediation...")
		remediationStatus, remediationTime := remediateDriftWithStatus(playbook, inventory, targetHosts, log)
		return true, true, remediationStatus, remediationTime
	}

	log.Info().Str("playbook", playbook).Msg("No drift detected.")
	return false, false, "ok", ""
}

// areChangesIgnorable checks if all changes in the Ansible output are ignorable
func areChangesIgnorable(output string) bool {
	// Convert to lowercase for case-insensitive matching
	outputLower := strings.ToLower(output)

	// Define ignorable patterns
	ignorablePatterns := []string{
		"atime", "mtime", "ctime",
		"ansible_date_time", "generated by ansible",
		"timestamp", "last_modified", "last_updated",
		"access_time", "modification_time", "creation_time",
		"ansible date and time", "date and time",
		"iso8601", "utc", "gmt", "timezone",
		"state", "touch", "file",
	}

	// Check if the output contains any non-ignorable content changes
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lineLower := strings.ToLower(line)

		// Skip headers, comments, and metadata lines
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

		// If this line has + or - (indicating a change), check if it's ignorable
		if strings.Contains(line, "-") || strings.Contains(line, "+") {
			isIgnorable := false
			for _, pattern := range ignorablePatterns {
				if strings.Contains(lineLower, pattern) {
					isIgnorable = true
					break
				}
			}

			// If this change is not ignorable, return false
			if !isIgnorable {
				return false
			}
		}
	}

	// All changes are ignorable
	return true
}

func remediateDriftWithStatus(playbook string, inventory string, targetHosts string, log zerolog.Logger) (string, string) {
	cmd := exec.Command("ansible-playbook", playbook, "--inventory", inventory)
	if targetHosts != "" {
		cmd.Args = append(cmd.Args, "--limit", targetHosts)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible remediation")
		return "error", time.Now().UTC().Format(time.RFC3339)
	} else {
		log.Info().Str("playbook", playbook).Msgf("Ansible remediation output:\n%s", string(output))
		return "ok", time.Now().UTC().Format(time.RFC3339)
	}
}

func StartDriftDetection(server *Server) {
	go func() {
		// Run first drift detection immediately
		DriftDetection(server)

		// Then run every 3 minutes
		ticker := time.NewTicker(3 * time.Minute)
		defer ticker.Stop()
		for {
			<-ticker.C
			DriftDetection(server)
		}
	}()
}

// getIgnorableChangePatterns returns patterns that should be ignored as they don't represent true drift
func getIgnorableChangePatterns() []string {
	// These could come from configuration in the future
	return []string{
		"atime",
		"mtime",
		"state\": \"touch\"",
		"ansible_date_time",
		"Generated by Ansible on",
		"timestamp",
		"last_modified",
		"last_updated",
		"ctime",
		"access_time",
		"modification_time",
		"creation_time",
	}
}
