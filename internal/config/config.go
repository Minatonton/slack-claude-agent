package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	} else {
		// Auto-detect mode: scan workspace directory for Git repositories
		repos, err := c.autoDetectRepositories()
		if err != nil {
			return fmt.Errorf("failed to auto-detect repositories: %w", err)
		}
		c.Repositories = repos

		// Use first repository as default
		if len(repos) > 0 {
			c.DefaultRepository = repos[0]
		}
	}

	return nil
}

// autoDetectRepositories scans the workspace directory and detects all Git repositories
func (c *Config) autoDetectRepositories() ([]*domain.Repository, error) {
	var repos []*domain.Repository

	fmt.Fprintf(os.Stderr, "Auto-detecting repositories in: %s\n", c.WorkspacePath)

	entries, err := os.ReadDir(c.WorkspacePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspace directory %s: %w", c.WorkspacePath, err)
	}

	fmt.Fprintf(os.Stderr, "Found %d entries in workspace\n", len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(c.WorkspacePath, entry.Name())
		gitDir := filepath.Join(repoPath, ".git")

		// Check if it's a Git repository
		if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Skipping %s: not a Git repository\n", entry.Name())
			continue
		}

		fmt.Fprintf(os.Stderr, "Detecting repository: %s\n", entry.Name())

		// Extract owner/repo from .git/config
		owner, name, err := c.extractRepoInfo(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to extract repo info from %s: %v\n", repoPath, err)
			continue
		}

		// Get default branch
		defaultBranch, err := c.getDefaultBranch(repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get default branch for %s: %v, using 'main'\n", repoPath, err)
			defaultBranch = "main"
		}

		repo := &domain.Repository{
			Owner:         owner,
			Name:          name,
			DefaultBranch: defaultBranch,
		}
		repos = append(repos, repo)
		fmt.Fprintf(os.Stderr, "Detected: %s/%s (branch: %s)\n", owner, name, defaultBranch)
	}

	fmt.Fprintf(os.Stderr, "Auto-detected %d repositories\n", len(repos))

	return repos, nil
}

// extractRepoInfo extracts owner and repo name from .git/config
func (c *Config) extractRepoInfo(repoPath string) (owner, name string, err error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	remoteURL := strings.TrimSpace(string(output))

	// Parse GitHub URL (supports both HTTPS and SSH formats)
	// HTTPS: https://github.com/owner/repo.git
	// SSH: git@github.com:owner/repo.git
	re := regexp.MustCompile(`github\.com[:/]([^/]+)/(.+?)(\.git)?$`)
	matches := re.FindStringSubmatch(remoteURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("invalid GitHub URL format: %s", remoteURL)
	}

	owner = matches[1]
	name = strings.TrimSuffix(matches[2], ".git")

	return owner, name, nil
}

// getDefaultBranch gets the default branch for a repository
func (c *Config) getDefaultBranch(repoPath string) (string, error) {
	// Try to get default branch from remote HEAD
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err == nil {
		branch := strings.TrimPrefix(strings.TrimSpace(string(output)), "origin/")
		if branch != "" {
			return branch, nil
		}
	}

	// Fallback: try to get current branch
	cmd = exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(output))
		if branch != "" && branch != "HEAD" {
			return branch, nil
		}
	}

	return "", fmt.Errorf("could not determine default branch")
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
