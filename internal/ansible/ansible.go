package ansible

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"ansible-api/internal/vault"
)

// NewClient creates a new Ansible client
func NewClient(vaultClient *vault.VaultClient) (*Client, error) {
	// Get an SSH key from Vault
	sshKey, err := vaultClient.GetSSHKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH key from Vault: %v", err)
	}

	// Create temporary file for SSH key
	tmpFile, err := os.CreateTemp("", "ansible-ssh-key-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %v", err)
	}

	// Write an SSH key to a temporary file
	if _, err := tmpFile.WriteString(sshKey); err != nil {
		err := tmpFile.Close()
		if err != nil {
			log.Error().Err(err).Msg("failed to close temporary file after write error")
			return nil, err
		}
		err = os.Remove(tmpFile.Name())
		if err != nil {
			log.Error().Err(err).Msg("failed to remove temporary file after write error")
			return nil, err
		}
		return nil, fmt.Errorf("failed to write SSH key to temporary file: %v", err)
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		if removeErr := os.Remove(tmpFile.Name()); removeErr != nil {
			log.Error().Err(removeErr).Msg("failed to remove temporary file after close error")
			return nil, fmt.Errorf("failed to close and remove temporary file: %v (remove error: %v)", err, removeErr)
		}
		return nil, fmt.Errorf("failed to close temporary file: %v", err)
	}

	// Set correct permissions
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		err := os.Remove(tmpFile.Name())
		if err != nil {
			log.Error().Err(err).Msg("failed to remove temporary file after chmod error")
			return nil, err
		}
		return nil, fmt.Errorf("failed to set permissions on temporary file: %v", err)
	}

	return &Client{
		SSHKeyPath: tmpFile.Name(),
	}, nil
}
