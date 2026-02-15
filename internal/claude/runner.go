package claude

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Runner struct {
	claudePath      string
	workspacePath   string
	githubOwner     string
	githubRepo      string
	defaultBranch   string
	authorName      string
	authorEmail     string
	coAuthorName    string
	coAuthorEmail   string
	semaphore       chan struct{}
	logger          *slog.Logger
}

type Config struct {
	ClaudePath    string
	WorkspacePath string
	GitHubOwner   string
	GitHubRepo    string
	DefaultBranch string
	AuthorName    string
	AuthorEmail   string
	CoAuthorName  string
	CoAuthorEmail string
	MaxConcurrent int
}

func NewRunner(cfg Config, logger *slog.Logger) *Runner {
	return &Runner{
		claudePath:    cfg.ClaudePath,
		workspacePath: cfg.WorkspacePath,
		githubOwner:   cfg.GitHubOwner,
		githubRepo:    cfg.GitHubRepo,
		defaultBranch: cfg.DefaultBranch,
		authorName:    cfg.AuthorName,
		authorEmail:   cfg.AuthorEmail,
		coAuthorName:  cfg.CoAuthorName,
		coAuthorEmail: cfg.CoAuthorEmail,
		semaphore:     make(chan struct{}, cfg.MaxConcurrent),
		logger:        logger,
	}
}

// Run executes claude CLI with the given prompt.
// workDir is where the repository is located.
func (r *Runner) Run(ctx context.Context, prompt string, callback ProgressCallback) (*Result, error) {
	// Acquire semaphore
	select {
	case r.semaphore <- struct{}{}:
		defer func() { <-r.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Build full prompt with instructions
	fullPrompt := r.buildPrompt(prompt)

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
		fullPrompt,
	}

	workDir := filepath.Join(r.workspacePath, r.githubRepo)
	r.logger.Info("running claude", "workdir", workDir, "args_count", len(args))

	cmd := exec.CommandContext(ctx, r.claudePath, args...)
	cmd.Dir = workDir

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	parser := NewParser(r.logger, callback)
	result, parseErr := parser.Parse(stdout)

	waitErr := cmd.Wait()

	stderrStr := stderrBuf.String()
	if stderrStr != "" {
		r.logger.Error("claude stderr", "stderr", stderrStr)
	}

	if parseErr != nil {
		return result, fmt.Errorf("parse error: %w", parseErr)
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("claude exited with error: %w (stderr: %s)", waitErr, stderrStr)
	}

	return result, nil
}

func (r *Runner) buildPrompt(instruction string) string {
	return fmt.Sprintf(`%s

Repository: %s/%s
Default branch: %s

Instructions:
1. Implement the requested changes
2. Create a new branch with a descriptive name
3. Commit your changes with a clear commit message
4. Create a pull request using 'gh pr create'

When creating commits, use this format:
git commit -m "Your commit message

Co-Authored-By: %s <%s>"

CRITICAL RULES:
- NEVER merge any branch into main/master/develop
- NEVER push directly to %s
- NEVER force push (git push -f, git push --force)
- Always create a feature branch and push to that branch, then create a PR
`,
		instruction,
		r.githubOwner,
		r.githubRepo,
		r.defaultBranch,
		r.coAuthorName,
		r.coAuthorEmail,
		r.defaultBranch,
	)
}

// RunWithTimeout wraps Run with a timeout.
func (r *Runner) RunWithTimeout(ctx context.Context, prompt string, timeout time.Duration, callback ProgressCallback) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return r.Run(ctx, prompt, callback)
}

// FormatToolSummary creates a human-readable summary of a tool invocation.
func FormatToolSummary(name string, input map[string]interface{}) string {
	switch name {
	case "Read":
		if p, ok := input["file_path"].(string); ok {
			return shortPath(p) + " を読み取り"
		}
	case "Edit":
		if p, ok := input["file_path"].(string); ok {
			return shortPath(p) + " を編集"
		}
	case "Write":
		if p, ok := input["file_path"].(string); ok {
			return shortPath(p) + " を作成"
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			short := cmd
			if len(short) > 60 {
				short = short[:60] + "..."
			}
			return "`" + short + "`"
		}
	case "Glob":
		if p, ok := input["pattern"].(string); ok {
			return p + " を検索"
		}
	case "Grep":
		if p, ok := input["pattern"].(string); ok {
			return "`" + p + "` を検索"
		}
	}
	return name
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) <= 2 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}
