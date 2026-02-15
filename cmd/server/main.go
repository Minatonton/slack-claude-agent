package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/toshin/slack-claude-agent/internal/agent"
	"github.com/toshin/slack-claude-agent/internal/claude"
	"github.com/toshin/slack-claude-agent/internal/config"
	ghclient "github.com/toshin/slack-claude-agent/internal/github"
	slackclient "github.com/toshin/slack-claude-agent/internal/slack"
)

func main() {
	// JSON structured logging
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize clients
	sc := slackclient.NewClient(cfg.SlackBotToken)

	cc, err := claude.NewClient(cfg.GCPProjectID, cfg.GCPLocation, cfg.ClaudeModel)
	if err != nil {
		slog.Error("failed to create claude client", "error", err)
		os.Exit(1)
	}

	gc := ghclient.NewClient(cfg.GitHubPAT, cfg.GitHubOwner, cfg.GitHubRepo)

	// Create agent
	ag := agent.New(sc, cc, gc, cfg)

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	slackHandler := slackclient.NewHandler(cfg.SlackSigningSecret, ag)
	r.Post("/slack/events", slackHandler.ServeHTTP)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
