package githubapp

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-github/v55/github"
	"golang.org/x/oauth2"
)

// GetInstallationToken generates a JWT, exchanges it for an installation token, and returns the token string.
func GetInstallationToken(appID, installationID int, privateKeyPath, apiBaseURL string) (string, error) {
	jwtToken, err := generateGitHubAppJWT(appID, privateKeyPath)
	if err != nil {
		return "", err
	}
	return exchangeJWTForToken(jwtToken, installationID, apiBaseURL)
}

func generateGitHubAppJWT(appID int, privateKeyPath string) (string, error) {
	keyBytes, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key: %w", err)
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return "", errors.New("failed to parse PEM block from private key")
	}
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse RSA private key: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(time.Minute * 10).Unix(),
		"iss": appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privKey)
}

func exchangeJWTForToken(jwtToken string, installationID int, apiBaseURL string) (string, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: jwtToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client, err := github.NewEnterpriseClient(apiBaseURL, apiBaseURL, tc)
	if err != nil {
		return "", fmt.Errorf("failed to create GitHub client: %w", err)
	}
	token, _, err := client.Apps.CreateInstallationToken(ctx, int64(installationID), nil)
	if err != nil {
		return "", fmt.Errorf("failed to get installation token: %w", err)
	}
	return token.GetToken(), nil
}

// BuildCloneURL returns a git clone URL with the installation token for HTTPS cloning.
func BuildCloneURL(token, repoPath, host string) string {
	return fmt.Sprintf("https://x-access-token:%s@%s/%s", token, host, repoPath)
}
