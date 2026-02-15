package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/toshin/slack-claude-agent/internal/agent"
	"github.com/toshin/slack-claude-agent/internal/claude"
	"github.com/toshin/slack-claude-agent/internal/config"
	slackclient "github.com/toshin/slack-claude-agent/internal/slack"
)

func main() {
	// JSON structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Create Socket Mode handler (also creates the Slack API client)
	handler := slackclient.NewHandler(cfg.SlackAppToken, cfg.SlackBotToken, nil)
	sc := slackclient.NewClient(handler.APIClient())

	// Create Claude runner
	runnerCfg := claude.Config{
		ClaudePath:    cfg.ClaudePath,
		WorkspacePath: cfg.WorkspacePath,
		GitHubOwner:   cfg.GitHubOwner,
		GitHubRepo:    cfg.GitHubRepo,
		DefaultBranch: cfg.DefaultBranch,
		AuthorName:    cfg.AuthorName,
		AuthorEmail:   cfg.AuthorEmail,
		CoAuthorName:  cfg.CoAuthorName,
		CoAuthorEmail: cfg.CoAuthorEmail,
		MaxConcurrent: cfg.MaxConcurrent,
	}
	runner := claude.NewRunner(runnerCfg, logger)

	// Create agent and wire it into the handler
	ag := agent.New(sc, runner, logger)
	handler.SetMentionHandler(ag)

	// Run Socket Mode (blocks until context is cancelled)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting slack-claude-agent")
	if err := handler.Run(ctx); err != nil {
		logger.Error("handler exited", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
