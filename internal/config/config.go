package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// Slack
	SlackBotToken string
	SlackAppToken string

	// Workspace
	WorkspacePath string // parent directory containing the repository

	// GitHub
	GitHubOwner   string
	GitHubRepo    string
	DefaultBranch string

	// Commit Author
	AuthorName    string
	AuthorEmail   string
	CoAuthorName  string
	CoAuthorEmail string

	// Claude
	ClaudePath    string // path to claude CLI binary
	MaxConcurrent int    // max concurrent claude runs
}

func Load() (*Config, error) {
	cfg := &Config{
		SlackBotToken: os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken: os.Getenv("SLACK_APP_TOKEN"),
		WorkspacePath: getEnvDefault("WORKSPACE_PATH", "."),
		GitHubOwner:   os.Getenv("GITHUB_OWNER"),
		GitHubRepo:    os.Getenv("GITHUB_REPO"),
		DefaultBranch: getEnvDefault("DEFAULT_BRANCH", "main"),
		AuthorName:    os.Getenv("AUTHOR_NAME"),
		AuthorEmail:   os.Getenv("AUTHOR_EMAIL"),
		CoAuthorName:  getEnvDefault("CO_AUTHOR_NAME", "Claude"),
		CoAuthorEmail: getEnvDefault("CO_AUTHOR_EMAIL", "noreply+claude@anthropic.com"),
		ClaudePath:    getEnvDefault("CLAUDE_PATH", "claude"),
		MaxConcurrent: getEnvIntDefault("MAX_CONCURRENT", 5),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	required := map[string]string{
		"SLACK_BOT_TOKEN": c.SlackBotToken,
		"SLACK_APP_TOKEN": c.SlackAppToken,
		"GITHUB_OWNER":    c.GitHubOwner,
		"GITHUB_REPO":     c.GitHubRepo,
		"AUTHOR_NAME":     c.AuthorName,
		"AUTHOR_EMAIL":    c.AuthorEmail,
	}

	for name, val := range required {
		if val == "" {
			return fmt.Errorf("required config %s is not set", name)
		}
	}
	return nil
}

func getEnvDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvIntDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
