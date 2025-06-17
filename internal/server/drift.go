package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type PlaybookState struct {
	LastRun    string `json:"last_run"`
	LastHash   string `json:"last_hash"`
	LastStatus string `json:"last_status"`
}

type StateFile map[string]PlaybookState

const (
	playbookListFile = "playbooks.list"
	stateFile        = "playbook_state.json"
	inventoryFile    = "hosts.ini"
)

func StartDriftDetection() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			checkAndRemediateDrift()
			<-ticker.C
		}
	}()
}

func checkAndRemediateDrift() {
	log := log.With().Str("component", "drift").Logger()
	playbooks, err := getPlaybookList(playbookListFile)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read playbook list")
		return
	}
	state, _ := loadStateFile(stateFile)
	changed := false
	for _, pb := range playbooks {
		hash, err := fileHash(pb)
		if err != nil {
			log.Error().Str("playbook", pb).Err(err).Msg("Failed to hash playbook")
			continue
		}
		ps, exists := state[pb]
		if !exists || ps.LastHash != hash {
			log.Info().Str("playbook", pb).Msg("New or changed playbook detected, running drift check...")
			status := runAnsibleCheck(pb, log)
			state[pb] = PlaybookState{
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

func getPlaybookList(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var playbooks []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			playbooks = append(playbooks, line)
		}
	}
	return playbooks, scanner.Err()
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
	cmd := exec.Command("ansible-playbook", "-i", inventoryFile, playbook, "--check", "--diff")
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
	cmd := exec.Command("ansible-playbook", "-i", inventoryFile, playbook)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Str("playbook", playbook).Err(err).Msg("Error running Ansible remediation")
	} else {
		log.Info().Str("playbook", playbook).Msgf("Ansible remediation output:\n%s", string(output))
	}
}
