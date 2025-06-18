package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PlaybookState struct {
	Repo       string `json:"repo"`
	LastRun    string `json:"last_run"`
	LastHash   string `json:"last_hash"`
	LastStatus string `json:"last_status"`
}

type StateFile map[string]PlaybookState

const (
	stateFile = "playbook_state.json"
)

// UpdatePlaybookState updates the state file when a playbook is run.
func UpdatePlaybookState(playbookPath, repo string, status string) error {
	hash, err := fileHash(playbookPath)
	if err != nil {
		return err
	}
	state, _ := loadStateFile(stateFile)
	state[playbookPath] = PlaybookState{
		Repo:       repo,
		LastRun:    time.Now().UTC().Format(time.RFC3339),
		LastHash:   hash,
		LastStatus: status,
	}
	return saveStateFile(stateFile, state)
}

// RemovePlaybookState removes a playbook entry from the state file.
func RemovePlaybookState(playbookPath string) error {
	state, _ := loadStateFile(stateFile)
	delete(state, playbookPath)
	return saveStateFile(stateFile, state)
}

// DriftDetection checks for drift only for playbooks present in the state file.
func DriftDetection() {
	log := log.With().Str("component", "drift").Logger()
	state, _ := loadStateFile(stateFile)
	changed := false
	for pb := range state {
		if _, err := os.Stat(pb); os.IsNotExist(err) {
			log.Warn().Str("playbook", pb).Msg("Playbook no longer exists, removing from state file")
			RemovePlaybookState(pb)
			changed = true
			continue
		}
		hash, err := fileHash(pb)
		if err != nil {
			log.Error().Str("playbook", pb).Err(err).Msg("Failed to hash playbook")
			continue
		}
		ps := state[pb]
		if ps.LastHash != hash {
			log.Info().Str("playbook", pb).Msg("Playbook changed, running drift check...")
			status := runAnsibleCheck(pb, log)
			state[pb] = PlaybookState{
				Repo:       ps.Repo,
				LastRun:    time.Now().UTC().Format(time.RFC3339),
				LastHash:   hash,
				LastStatus: status,
			}
			changed = true
		}
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
			return state, nil // empty state
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

func runAnsibleCheck(playbook string, log zerolog.Logger) string {
	cmd := exec.Command("ansible-playbook", playbook, "--check", "--diff")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible check mode")
		return "error"
	}
	if bytes.Contains(output, []byte("changed=")) {
		log.Warn().Str("playbook", playbook).Msg("Drift detected! Running Ansible remediation...")
		remediateDrift(playbook, log)
		return "drift"
	}
	log.Info().Str("playbook", playbook).Msg("No drift detected.")
	return "ok"
}

func remediateDrift(playbook string, log zerolog.Logger) {
	cmd := exec.Command("ansible-playbook", playbook)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible remediation")
	} else {
		log.Info().Str("playbook", playbook).Msgf("Ansible remediation output:\n%s", string(output))
	}
}

// StartDriftDetection launches a goroutine that checks for drift every hour and triggers remediation if needed.
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
