package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	// Server
	Port string

	// Database
	DatabaseURL string

	// GitHub App
	GitHubAppID         string
	GitHubPrivateKey    string
	GitHubWebhookSecret string

	// GitHub OAuth (admin panel)
	GitHubClientID     string
	GitHubClientSecret string

	// Allowed orgs — users must be a member of at least one to use slash commands
	AllowedOrgs []string

	// Session
	SessionSecret string
}

func Load() (*Config, error) {
	c := &Config{
		Port:                envOrDefault("PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		GitHubAppID:         os.Getenv("GITHUB_APP_ID"),
		GitHubPrivateKey:    os.Getenv("GITHUB_PRIVATE_KEY"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitHubClientID:      os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
		AllowedOrgs:         parseOrgList(os.Getenv("ALLOWED_ORGS")),
		SessionSecret:       os.Getenv("SESSION_SECRET"),
	}

	required := map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"GITHUB_APP_ID":        c.GitHubAppID,
		"GITHUB_PRIVATE_KEY":   c.GitHubPrivateKey,
		"GITHUB_WEBHOOK_SECRET": c.GitHubWebhookSecret,
	}

	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("required environment variable %s is not set", name)
		}
	}

	if len(c.AllowedOrgs) == 0 {
		return nil, fmt.Errorf("required environment variable ALLOWED_ORGS is not set (comma-separated list of GitHub org names)")
	}

	return c, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseOrgList(s string) []string {
	if s == "" {
		return nil
	}
	var orgs []string
	for _, org := range strings.Split(s, ",") {
		org = strings.TrimSpace(org)
		if org != "" {
			orgs = append(orgs, org)
		}
	}
	return orgs
}
