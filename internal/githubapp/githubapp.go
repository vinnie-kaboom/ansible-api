package githubapp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwtExpirationMinutes = 10
	defaultAPIBaseURL    = "https://api.github.com"
)

// // AuthConfig holds the configuration for GitHub App authentication
// type AuthConfig struct {
// 	AppID          int    `json:"app_id"`
// 	InstallationID int    `json:"installation_id"`
// 	PrivateKey     string `json:"private_key"`
// 	APIBaseURL     string `json:"api_base_url"`
// }

type GithubAuthenticator interface {
	GetInstallationToken(config AuthConfig) (string, error)
}

type DefaultAuthenticator struct{}

func (e *AuthError) Error() string {
	return fmt.Sprintf("github authentication error during %s: %v", e.Op, e.Err)
}

// GetInstallationToken generates a GitHub App installation token
func (a *DefaultAuthenticator) GetInstallationToken(config AuthConfig) (string, error) {
	// Use the private key content directly from config
	privateKey := []byte(config.PrivateKey)

	// Generate JWT
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": config.AppID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	key, err := jwt.ParseRSAPrivateKeyFromPEM(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	jwtToken, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	// Request installation token
	client := &http.Client{}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/app/installations/%d/access_tokens", config.APIBaseURL, config.InstallationID), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get installation token: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Token, nil
}

// BuildCloneURL creates a clone URL with authentication token
func BuildCloneURL(token, repoPath, host string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s", token, host, repoPath)
}
