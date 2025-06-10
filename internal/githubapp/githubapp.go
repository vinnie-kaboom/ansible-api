package githubapp

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v55/github"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

const (
	jwtExpirationMinutes = 10
	defaultAPIBaseURL    = "https://api.github.com"
)

type AuthConfig struct {
	AppID          int
	InstallationID int
	PrivateKeyPath string
	APIBaseURL     string
}

type GithubAuthenticator interface {
	GetInstallationToken(config AuthConfig) (string, error)
}

type DefaultAuthenticator struct{}

type AuthError struct {
	Op  string
	Err error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("github authentication error during %s: %v", e.Op, e.Err)
}

// GetInstallationToken generates a GitHub App installation token
func (a *DefaultAuthenticator) GetInstallationToken(config AuthConfig) (string, error) {
	jwtToken, err := generateGitHubAppJWT(config.AppID, config.PrivateKeyPath)
	if err != nil {
		return "", err
	}
	return exchangeJWTForToken(jwtToken, config.InstallationID, config.APIBaseURL)
}

func generateGitHubAppJWT(appID int, privateKeyPath string) (string, error) {
	logger := log.With().Str("pem_path", privateKeyPath).Logger()
	logger.Info().Msg("Reading GitHub App private key PEM file")

	keyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return "", &AuthError{Op: "read_private_key", Err: err}
	}

	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return "", &AuthError{Op: "decode_pem", Err: fmt.Errorf("invalid PEM format")}
	}

	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return "", &AuthError{Op: "parse_rsa_key", Err: err}
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(time.Minute * jwtExpirationMinutes).Unix(),
		"iss": appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	logger.Info().Msg("Generated JWT for GitHub App authentication")

	return token.SignedString(privKey)
}

func exchangeJWTForToken(jwtToken string, installationID int, apiBaseURL string) (string, error) {
	logger := log.With().Int("installation_id", installationID).Logger()
	logger.Info().Msg("Requesting GitHub App installation token")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: jwtToken})
	tc := oauth2.NewClient(ctx, ts)

	var client *github.Client
	var err error

	if apiBaseURL != "" && apiBaseURL != defaultAPIBaseURL {
		// Create a new client with custom base URL
		client = github.NewClient(tc)
		client.BaseURL, err = url.Parse(apiBaseURL)
		if err != nil {
			return "", &AuthError{Op: "parse_api_url", Err: err}
		}
		// Set the upload URL to the same base URL
		client.UploadURL = client.BaseURL
	} else {
		client = github.NewClient(tc)
	}

	token, _, err := client.Apps.CreateInstallationToken(ctx, int64(installationID), nil)
	if err != nil {
		return "", &AuthError{Op: "create_installation_token", Err: err}
	}

	logger.Info().Msg("Successfully obtained GitHub App installation token")
	return token.GetToken(), nil
}

// BuildCloneURL creates a clone URL with authentication token
func BuildCloneURL(token, repoPath, host string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s", token, host, repoPath)
}
