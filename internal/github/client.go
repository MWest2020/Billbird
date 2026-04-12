package github

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Client struct {
	appID      string
	privateKey any
	httpClient *http.Client

	mu               sync.Mutex
	installTokens    map[int64]tokenEntry
}

type tokenEntry struct {
	token     string
	expiresAt time.Time
}

func NewClient(appID, privateKeyPEM string) (*Client, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	return &Client{
		appID:         appID,
		privateKey:    key,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		installTokens: make(map[int64]tokenEntry),
	}, nil
}

func (c *Client) generateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    c.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(c.privateKey)
}

func (c *Client) getInstallationToken(installationID int64) (string, error) {
	c.mu.Lock()
	if entry, ok := c.installTokens[installationID]; ok && time.Now().Before(entry.expiresAt.Add(-60*time.Second)) {
		c.mu.Unlock()
		return entry.token, nil
	}
	c.mu.Unlock()

	jwtToken, err := c.generateJWT()
	if err != nil {
		return "", fmt.Errorf("generating JWT: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("installation token request failed (%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	c.mu.Lock()
	c.installTokens[installationID] = tokenEntry{
		token:     result.Token,
		expiresAt: result.ExpiresAt,
	}
	c.mu.Unlock()

	return result.Token, nil
}

// PostComment creates a comment on a GitHub issue.
func (c *Client) PostComment(installationID int64, repo string, issueNumber int, body string) (int64, error) {
	token, err := c.getInstallationToken(installationID)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments", repo, issueNumber)
	payload, _ := json.Marshal(map[string]string{"body": body})

	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("posting comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("post comment failed (%d): %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding comment response: %w", err)
	}

	return result.ID, nil
}

// GetIssueLabels returns the labels on a GitHub issue.
func (c *Client) GetIssueLabels(installationID int64, repo string, issueNumber int) ([]string, error) {
	token, err := c.getInstallationToken(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/labels", repo, issueNumber)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching labels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get labels failed (%d): %s", resp.StatusCode, respBody)
	}

	var labels []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&labels); err != nil {
		return nil, fmt.Errorf("decoding labels response: %w", err)
	}

	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names, nil
}

// IsOrgMember checks if a GitHub user is a member of the given organization.
// Uses the installation token to call the org membership API.
func (c *Client) IsOrgMember(installationID int64, org, username string) (bool, error) {
	token, err := c.getInstallationToken(installationID)
	if err != nil {
		return false, err
	}

	url := fmt.Sprintf("https://api.github.com/orgs/%s/members/%s", org, username)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("checking org membership: %w", err)
	}
	defer resp.Body.Close()

	// 204 = member, 404 = not a member, 302 = requester is not an org member
	return resp.StatusCode == http.StatusNoContent, nil
}

// InstallationIDFromEvent extracts the installation ID from a webhook payload.
func InstallationIDFromEvent(payload map[string]any) (int64, error) {
	inst, ok := payload["installation"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("no installation field in webhook payload")
	}

	// JSON numbers decode as float64
	switch v := inst["id"].(type) {
	case float64:
		return int64(v), nil
	case json.Number:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected installation id type: %T", v)
	}
}
