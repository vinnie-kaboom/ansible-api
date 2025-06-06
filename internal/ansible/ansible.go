package ansible

import (
	"fmt"
	"os"

	"ansible-api/internal/vault"
)

// AnsibleClient represents an Ansible client
type AnsibleClient struct {
	SSHKeyPath string
}

// NewAnsibleClient creates a new Ansible client
func NewAnsibleClient(vaultClient *vault.Client) (*AnsibleClient, error) {
	// Get SSH key from Vault
	sshKey, err := vaultClient.GetSSHKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH key from Vault: %v", err)
	}

	// Create temporary file for SSH key
	tmpFile, err := os.CreateTemp("", "ansible-ssh-key-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %v", err)
	}

	// Write SSH key to temporary file
	if _, err := tmpFile.WriteString(sshKey); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to write SSH key to temporary file: %v", err)
	}

	// Close the file
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to close temporary file: %v", err)
	}

	// Set correct permissions
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to set permissions on temporary file: %v", err)
	}

	return &AnsibleClient{
		SSHKeyPath: tmpFile.Name(),
	}, nil
}
