package config

import (
	"context"
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

type Config struct {
	// Server
	Port string

	// Slack
	SlackSigningSecret string
	SlackBotToken      string

	// GitHub
	GitHubPAT     string
	GitHubOwner   string
	GitHubRepo    string
	DefaultBranch string

	// Commit Author
	AuthorName    string
	AuthorEmail   string
	CoAuthorName  string
	CoAuthorEmail string

	// GCP
	GCPProjectID string
	GCPLocation  string
	ClaudeModel  string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnvOrDefault("PORT", "8080"),
		GitHubOwner:   os.Getenv("GITHUB_OWNER"),
		GitHubRepo:    os.Getenv("GITHUB_REPO"),
		DefaultBranch: getEnvOrDefault("DEFAULT_BRANCH", "main"),
		AuthorName:    os.Getenv("AUTHOR_NAME"),
		AuthorEmail:   os.Getenv("AUTHOR_EMAIL"),
		CoAuthorName:  getEnvOrDefault("CO_AUTHOR_NAME", "Claude"),
		CoAuthorEmail: getEnvOrDefault("CO_AUTHOR_EMAIL", "noreply+claude@anthropic.com"),
		GCPProjectID:  os.Getenv("GCP_PROJECT_ID"),
		GCPLocation:   getEnvOrDefault("GCP_LOCATION", "us-east5"),
		ClaudeModel:   getEnvOrDefault("CLAUDE_MODEL", "claude-sonnet-4-20250514"),
	}

	// Try Secret Manager first, fall back to env vars
	ctx := context.Background()
	cfg.SlackSigningSecret = loadSecret(ctx, cfg.GCPProjectID, "slack-signing-secret")
	if cfg.SlackSigningSecret == "" {
		cfg.SlackSigningSecret = os.Getenv("SLACK_SIGNING_SECRET")
	}

	cfg.SlackBotToken = loadSecret(ctx, cfg.GCPProjectID, "slack-bot-token")
	if cfg.SlackBotToken == "" {
		cfg.SlackBotToken = os.Getenv("SLACK_BOT_TOKEN")
	}

	cfg.GitHubPAT = loadSecret(ctx, cfg.GCPProjectID, "github-pat")
	if cfg.GitHubPAT == "" {
		cfg.GitHubPAT = os.Getenv("GITHUB_PAT")
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	required := map[string]string{
		"SLACK_SIGNING_SECRET": c.SlackSigningSecret,
		"SLACK_BOT_TOKEN":     c.SlackBotToken,
		"GITHUB_PAT":          c.GitHubPAT,
		"GITHUB_OWNER":        c.GitHubOwner,
		"GITHUB_REPO":         c.GitHubRepo,
		"GCP_PROJECT_ID":      c.GCPProjectID,
		"AUTHOR_NAME":         c.AuthorName,
		"AUTHOR_EMAIL":        c.AuthorEmail,
	}
	for name, val := range required {
		if val == "" {
			return fmt.Errorf("required config %s is not set", name)
		}
	}
	return nil
}

func loadSecret(ctx context.Context, projectID, secretName string) string {
	if projectID == "" {
		return ""
	}
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return ""
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return ""
	}
	return string(result.Payload.Data)
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
