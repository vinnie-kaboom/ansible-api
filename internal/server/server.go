package server

import (
	"ansible-api/internal/vault"
	"os"

	"github.com/rs/zerolog/log"
)

func LoadConfiguration(vaultClient *vault.VaultClient, config *Config) {
	// Load GitHub configuration
	githubVaultPath := os.Getenv("GITHUB_VAULT_PATH")
	if githubVaultPath == "" {
		githubVaultPath = "ansible/github"
	}
	if githubConfig, err := vaultClient.GetSecret(githubVaultPath); err == nil {
		for key, value := range githubConfig {
			config.setIntValue(key, value)
			config.setStringValue(key, value)
		}
	} else {
		log.Warn().Msg("GitHub configuration not found in Vault, will use environment variables")
	}

	// Load API configuration
	apiVaultPath := os.Getenv("API_VAULT_PATH")
	if apiVaultPath == "" {
		apiVaultPath = "ansible/api"
	}
	if apiConfig, err := vaultClient.GetSecret(apiVaultPath); err == nil {
		for key, value := range apiConfig {
			config.setIntValue(key, value)
			config.setStringValue(key, value)
		}
	} else {
		log.Warn().Msg("API configuration not found in Vault, will use environment variables")
	}
}
