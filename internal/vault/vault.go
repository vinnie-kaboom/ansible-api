package vault

import (
	"fmt"
	"os"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var logger zerolog.Logger

func init() {
	// Set up logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger = log.With().Str("component", "vault").Logger()
}

// VaultClient represents a Vault client
type VaultClient struct {
	client *vault.Client
}

func NewClient() (*VaultClient, error) {
	logger.Info().Msg("Initializing Vault client")

	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		vaultAddr = "http://127.0.0.1:8200"
		logger.Debug().Str("vault_addr", vaultAddr).Msg("Using default Vault address")
	} else {
		logger.Debug().Str("vault_addr", vaultAddr).Msg("Using configured Vault address")
	}

	config := vault.DefaultConfig()
	config.Address = vaultAddr

	client, err := vault.NewClient(config)
	if err != nil {
		logger.Error().Err(err).Str("vault_addr", vaultAddr).Msg("Failed to create Vault client")
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Get the role ID and secret ID from environment variables
	roleID := os.Getenv("VAULT_ROLE_ID")
	secretID := os.Getenv("VAULT_SECRET_ID")

	if roleID == "" || secretID == "" {
		logger.Error().
			Bool("role_id_set", roleID != "").
			Bool("secret_id_set", secretID != "").
			Msg("Required Vault credentials not set")
		return nil, fmt.Errorf("VAULT_ROLE_ID and VAULT_SECRET_ID must be set")
	}

	logger.Debug().
		Str("role_id", maskString(roleID)).
		Str("secret_id", maskString(secretID)).
		Msg("Vault credentials found, attempting authentication")

	// Login with the provided role_id and secret_id
	loginSecret, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		logger.Error().
			Err(err).
			Str("role_id", maskString(roleID)).
			Str("vault_addr", vaultAddr).
			Msg("Failed to authenticate with Vault")
		return nil, fmt.Errorf("failed to login to vault: %w", err)
	}

	client.SetToken(loginSecret.Auth.ClientToken)
	logger.Info().
		Str("vault_addr", vaultAddr).
		Time("token_expiry", time.Unix(int64(loginSecret.Auth.LeaseDuration), 0)).
		Msg("Vault client initialized successfully")
	return &VaultClient{client: client}, nil
}

func (c *VaultClient) GetSecret(path string) (map[string]interface{}, error) {
	fullPath := fmt.Sprintf("kv/data/%s", path)

	logger.Debug().
		Str("path", path).
		Str("full_path", fullPath).
		Msg("Retrieving secret from Vault")

	secret, err := c.client.Logical().Read(fullPath)
	if err != nil {
		logger.Error().
			Err(err).
			Str("path", path).
			Str("full_path", fullPath).
			Msg("Failed to read secret from Vault")
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	if secret == nil || secret.Data == nil {
		logger.Warn().
			Str("path", path).
			Str("full_path", fullPath).
			Msg("Secret not found in Vault")
		return nil, fmt.Errorf("secret not found: %s", path)
	}

	// For KV v2, the data is nested under the "data" key
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		logger.Error().
			Str("path", path).
			Str("full_path", fullPath).
			Msg("Invalid secret data format")
		return nil, fmt.Errorf("invalid secret data format")
	}

	logger.Debug().
		Str("path", path).
		Int("data_keys", len(data)).
		Msg("Secret retrieved successfully")

	return data, nil
}

func (c *VaultClient) PutSecret(path string, data map[string]interface{}) error {
	_, err := c.client.Logical().Write(fmt.Sprintf("kv/data/%s", path), map[string]interface{}{
		"data": data,
	})
	if err != nil {
		logger.Error().Msg("Failed to write secret")
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return nil
}

func (c *VaultClient) DeleteSecret(path string) error {
	_, err := c.client.Logical().Delete(fmt.Sprintf("kv/data/%s", path))
	if err != nil {
		logger.Error().Msg("Failed to delete secret")
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

func (c *VaultClient) ListSecrets(path string) ([]string, error) {
	secret, err := c.client.Logical().List(fmt.Sprintf("kv/metadata/%s", path))
	if err != nil {
		logger.Error().Msg("Failed to list secrets")
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		logger.Error().Msg("Invalid secret list format")
		return nil, fmt.Errorf("invalid secret list format")
	}

	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = key.(string)
	}

	return result, nil
}

// GetSSHKey retrieves the SSH private key from Vault
func (c *VaultClient) GetSSHKey() (string, error) {
	secret, err := c.client.Logical().Read("kv/data/ansible/ssh-key")
	if err != nil {
		logger.Error().Msg("Failed to read SSH key")
		return "", fmt.Errorf("failed to read SSH key from Vault: %w", err)
	}

	if secret == nil || secret.Data == nil {
		logger.Warn().Msg("SSH key not found")
		return "", fmt.Errorf("SSH key not found in Vault")
	}

	// For KV v2, the data is nested under the "data" key
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		logger.Error().Msg("Invalid SSH key data format")
		return "", fmt.Errorf("invalid SSH key data format")
	}

	privateKey, ok := data["private_key"].(string)
	if !ok {
		logger.Error().Msg("Invalid SSH key format")
		return "", fmt.Errorf("invalid SSH key format")
	}

	return privateKey, nil
}

// maskString returns a masked version of a string for logging
func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
