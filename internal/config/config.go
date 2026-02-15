package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/toshin/slack-claude-agent/internal/domain"
)

type Config struct {
	// Slack
	SlackBotToken string
	SlackAppToken string

	// Workspace
	WorkspacePath string // parent directory containing the repository

	// GitHub (legacy single repository support)
	GitHubOwner   string
	GitHubRepo    string
	DefaultBranch string

	// GitHub (multi-repository support)
	Repositories      []*domain.Repository
	DefaultRepository *domain.Repository

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

	if err := cfg.loadRepositories(); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) loadRepositories() error {
	reposEnv := os.Getenv("GITHUB_REPOS")

	// If GITHUB_REPOS is set, use multi-repository mode
	if reposEnv != "" {
		repos, err := domain.ParseRepositories(reposEnv, c.DefaultBranch)
		if err != nil {
			return fmt.Errorf("failed to parse GITHUB_REPOS: %w", err)
		}
		c.Repositories = repos

		// Determine default repository
		defaultRepoKey := os.Getenv("DEFAULT_GITHUB_REPO")
		if defaultRepoKey != "" {
			c.DefaultRepository = domain.FindRepository(repos, defaultRepoKey)
			if c.DefaultRepository == nil {
				return fmt.Errorf("DEFAULT_GITHUB_REPO '%s' not found in GITHUB_REPOS", defaultRepoKey)
			}
		} else if len(repos) > 0 {
			// Use first repository as default
			c.DefaultRepository = repos[0]
		}
	} else if c.GitHubOwner != "" && c.GitHubRepo != "" {
		// Legacy mode: single repository from GITHUB_OWNER/GITHUB_REPO
		repo := &domain.Repository{
			Owner:         c.GitHubOwner,
			Name:          c.GitHubRepo,
			DefaultBranch: c.DefaultBranch,
		}
		c.Repositories = []*domain.Repository{repo}
		c.DefaultRepository = repo
	}

	return nil
}

func (c *Config) validate() error {
	required := map[string]string{
		"SLACK_BOT_TOKEN": c.SlackBotToken,
		"SLACK_APP_TOKEN": c.SlackAppToken,
		"AUTHOR_NAME":     c.AuthorName,
		"AUTHOR_EMAIL":    c.AuthorEmail,
	}

	for name, val := range required {
		if val == "" {
			return fmt.Errorf("required config %s is not set", name)
		}
	}

	// Validate that at least one repository is configured
	if len(c.Repositories) == 0 {
		return fmt.Errorf("no repositories configured: set either GITHUB_REPOS or GITHUB_OWNER/GITHUB_REPO")
	}

	if c.DefaultRepository == nil {
		return fmt.Errorf("no default repository set")
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
