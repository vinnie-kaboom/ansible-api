package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// DefaultAuthenticator implements the Authenticator interface
type DefaultAuthenticator struct{}

// GetInstallationToken retrieves a GitHub App installation token
func (a *DefaultAuthenticator) GetInstallationToken(config AuthConfig) (string, error) {
	// Implementation for getting installation token
	// This would use the GitHub App credentials to get an installation token
	return "dummy-token", nil
}

// BuildCloneURL builds a clone URL with authentication
func BuildCloneURL(token, repoPath, host string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s.git", token, host, repoPath)
}

// GenerateSSHKey generates an SSH key pair for Ansible authentication
func (a *DefaultAuthenticator) GenerateSSHKey() (string, string, error) {
	// Generate SSH key pair
	privateKey, publicKey, err := generateSSHKeyPair()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate SSH key pair: %w", err)
	}

	// Add public key to GitHub (optional - for remote hosts)
	// This would require additional GitHub API calls

	return privateKey, publicKey, nil
}

// generateSSHKeyPair creates a new SSH key pair
func generateSSHKeyPair() (string, string, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", "", err
	}

	// Encode private key
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyBytes := pem.EncodeToMemory(privateKeyPEM)

	// Generate public key
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	return string(privateKeyBytes), string(publicKeyBytes), nil
}

// AuthConfig holds GitHub App authentication configuration
type AuthConfig struct {
	AppID          int
	InstallationID int
	PrivateKey     string
	APIBaseURL     string
}
