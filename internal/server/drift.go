package server

import (
	"bytes"
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

var stateFile = filepath.Join(os.TempDir(), "playbook_state.json")

// getRemoteCommitHash gets the current commit hash from a remote repository
func getRemoteCommitHash(repoURL, branch string) (string, error) {
	cmd := exec.Command("git", "ls-remote", "--heads", repoURL, branch)
	output, err := cmd.Output()
	if err != nil {
		return "", err
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

func UpdatePlaybookState(logicalPath, fullPath, repo string, status string) error {
	log := log.With().Str("component", "drift").Logger()
	log.Info().Str("logicalPath", logicalPath).Str("fullPath", fullPath).Msg("UpdatePlaybookState called")
	hash, err := fileHash(fullPath)
	if err != nil {
		log.Error().Err(err).Msg("fileHash failed in UpdatePlaybookState")
		return err
	}

	// Get the current commit hash from the remote repository
	commitHash, err := getRemoteCommitHash(repo, "main") // You might want to make branch configurable
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

func DriftDetection() {
	log := log.With().Str("component", "drift").Logger()
	log.Info().Msg("DriftDetection tick")
	state, _ := loadStateFile(stateFile)
	log.Info().Int("playbook_count", len(state)).Msg("Loaded playbooks from state file")
	changed := false
	for logicalPath, ps := range state {
		// Check if the commit hash has changed
		currentCommitHash, err := getRemoteCommitHash(ps.Repo, "main") // You might want to make branch configurable
		if err != nil {
			log.Warn().Str("repo", ps.Repo).Err(err).Msg("Failed to get remote commit hash, skipping drift check")
			continue
		}

		// Skip drift check if commit hash hasn't changed
		if ps.PlaybookCommit != "" && ps.PlaybookCommit == currentCommitHash {
			log.Debug().Str("playbook", logicalPath).Str("commit", currentCommitHash).Msg("Commit hash unchanged, skipping drift check")
			continue
		}

		log.Info().Str("playbook", logicalPath).Str("old_commit", ps.PlaybookCommit).Str("new_commit", currentCommitHash).Msg("Commit hash changed, running drift check")

		tmpDir, err := os.MkdirTemp("", "repo-drift-")
		if err != nil {
			log.Error().Err(err).Msg("Failed to create temp dir for drift check")
			continue
		}
		defer os.RemoveAll(tmpDir)

		log.Info().Str("repo", ps.Repo).Msg("Cloning repo for drift check")
		_, err = git.PlainClone(tmpDir, false, &git.CloneOptions{
			URL: ps.Repo,
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
		hash, err := fileHash(playbookFullPath)
		if err != nil {
			log.Error().Str("playbook", playbookFullPath).Err(err).Msg("Failed to hash playbook")
			os.RemoveAll(tmpDir)
			continue
		}
		output, drift, remediationStatus, remediationTime := runAnsibleCheckWithOutput(playbookFullPath, log)
		state[logicalPath] = PlaybookState{
			Repo:                  ps.Repo,
			LastRun:               time.Now().UTC().Format(time.RFC3339),
			LastHash:              hash,
			LastStatus:            remediationStatus,
			LastCheckOutput:       output,
			LastRemediation:       remediationTime,
			LastRemediationStatus: remediationStatus,
			DriftDetected:         drift,
			LastTargets:           []string{},
			PlaybookCommit:        currentCommitHash,
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

func runAnsibleCheckWithOutput(playbook string, log zerolog.Logger) (string, bool, string, string) {
	cmd := exec.Command("ansible-playbook", playbook, "--check", "--diff")
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible check mode")
		return output, false, "error", ""
	}
	if bytes.Contains(outputBytes, []byte("changed=")) {
		log.Warn().Str("playbook", playbook).Msg("Drift detected! Running Ansible remediation...")
		remediationStatus, remediationTime := remediateDriftWithStatus(playbook, log)
		return output, true, remediationStatus, remediationTime
	}
	log.Info().Str("playbook", playbook).Msg("No drift detected.")
	return output, false, "ok", ""
}

func remediateDriftWithStatus(playbook string, log zerolog.Logger) (string, string) {
	cmd := exec.Command("ansible-playbook", playbook)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible remediation")
		return "error", time.Now().UTC().Format(time.RFC3339)
	} else {
		log.Info().Str("playbook", playbook).Msgf("Ansible remediation output:\n%s", string(output))
		return "ok", time.Now().UTC().Format(time.RFC3339)
	}
}

func StartDriftDetection() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			DriftDetection()
			<-ticker.C
		}
	}()
}
