package githubapp

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v55/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

func GetInstallationToken(appID, installationID int, privateKeyPath, apiBaseURL string) (string, error) {
	jwtToken, err := generateGitHubAppJWT(appID, privateKeyPath)
	if err != nil {
		return "", err
	}
	return exchangeJWTForToken(jwtToken, installationID, apiBaseURL)
}

func generateGitHubAppJWT(appID int, privateKeyPath string) (string, error) {
	log.Info().Str("pem_path", privateKeyPath).Msg("Reading GitHub App private key PEM file")
	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Error().Err(err).Str("pem_path", privateKeyPath).Msg("Failed to read private key PEM file")
		return "", fmt.Errorf("failed to read private key: %w", err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		log.Error().Str("pem_path", privateKeyPath).Msg("Failed to parse PEM block from private key")
		return "", errors.New("failed to parse PEM block from private key")
	}
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		log.Error().Err(err).Str("pem_path", privateKeyPath).Msg("Failed to parse RSA private key")
		return "", fmt.Errorf("failed to parse RSA private key: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(time.Minute * 10).Unix(),
		"iss": appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	log.Info().Msg("Generated JWT for GitHub App authentication")
	return token.SignedString(privKey)
}

func exchangeJWTForToken(jwtToken string, installationID int, apiBaseURL string) (string, error) {
	log.Info().Int("installation_id", installationID).Msg("Requesting GitHub App installation token from GitHub API")
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: jwtToken},
	)

	tc := oauth2.NewClient(ctx, ts)
	var client *github.Client
	var err error
	if apiBaseURL != "" && apiBaseURL != "https://api.github.com" {
		client, err = github.NewEnterpriseClient(apiBaseURL, apiBaseURL, tc)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create GitHub Enterprise client")
			return "", fmt.Errorf("failed to create GitHub Enterprise client: %w", err)
		}
	} else {
		client = github.NewClient(tc)
	}
	token, _, err := client.Apps.CreateInstallationToken(ctx, int64(installationID), nil)
	if err != nil {
		log.Error().Err(err).Int("installation_id", installationID).Msg("Failed to get installation token from GitHub API")
		return "", fmt.Errorf("failed to get installation token: %w", err)
	}
	log.Info().Int("installation_id", installationID).Msg("Successfully obtained GitHub App installation token")
	return token.GetToken(), nil
}

func BuildCloneURL(token, repoPath, host string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s", token, host, repoPath)
}
