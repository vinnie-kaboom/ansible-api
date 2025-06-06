package vault

import (
	"fmt"
	"os"

	vault "github.com/hashicorp/vault/api"
)

type Client struct {
	client *vault.Client
}

func NewClient() (*Client, error) {
	config := vault.DefaultConfig()
	config.Address = os.Getenv("VAULT_ADDR")
	if config.Address == "" {
		config.Address = "http://127.0.0.1:8200"
	}

	client, err := vault.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Get the role ID and secret ID from environment variables
	roleID := os.Getenv("VAULT_ROLE_ID")
	secretID := os.Getenv("VAULT_SECRET_ID")

	if roleID == "" || secretID == "" {
		return nil, fmt.Errorf("VAULT_ROLE_ID and VAULT_SECRET_ID must be set")
	}

	// Login with AppRole
	secret, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to login to vault: %w", err)
	}

	client.SetToken(secret.Auth.ClientToken)

	return &Client{client: client}, nil
}

func (c *Client) GetSecret(path string) (map[string]interface{}, error) {
	secret, err := c.client.Logical().Read(fmt.Sprintf("kv/data/%s", path))
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret not found: %s", path)
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret data format")
	}

	return data, nil
}

func (c *Client) PutSecret(path string, data map[string]interface{}) error {
	_, err := c.client.Logical().Write(fmt.Sprintf("kv/data/%s", path), map[string]interface{}{
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return nil
}

func (c *Client) DeleteSecret(path string) error {
	_, err := c.client.Logical().Delete(fmt.Sprintf("kv/data/%s", path))
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

func (c *Client) ListSecrets(path string) ([]string, error) {
	secret, err := c.client.Logical().List(fmt.Sprintf("kv/metadata/%s", path))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret list format")
	}

	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = key.(string)
	}

	return result, nil
}

// GetSSHKey retrieves the SSH private key from Vault
func (c *Client) GetSSHKey() (string, error) {
	secret, err := c.client.Logical().Read("kv/data/ansible/ssh-key")
	if err != nil {
		return "", fmt.Errorf("failed to read SSH key from Vault: %v", err)
	}
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("no SSH key found in Vault")
	}

	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid data format in Vault response")
	}

	key, ok := data["key"].(string)
	if !ok {
		return "", fmt.Errorf("SSH key not found in Vault data")
	}

	return key, nil
}
