package agent

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/toshin/slack-claude-agent/internal/claude"
	"github.com/toshin/slack-claude-agent/internal/config"
	ghclient "github.com/toshin/slack-claude-agent/internal/github"
	slackclient "github.com/toshin/slack-claude-agent/internal/slack"
)

var botMentionRe = regexp.MustCompile(`<@U[A-Z0-9]+>`)

type Agent struct {
	slackClient  *slackclient.Client
	claudeClient *claude.Client
	ghClient     *ghclient.Client
	config       *config.Config
}

func New(sc *slackclient.Client, cc *claude.Client, gc *ghclient.Client, cfg *config.Config) *Agent {
	return &Agent{
		slackClient:  sc,
		claudeClient: cc,
		ghClient:     gc,
		config:       cfg,
	}
}

func (a *Agent) HandleMention(event slackclient.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	channel := event.Channel
	threadTS := event.TS

	logger := slog.With("channel", channel, "user", event.User, "thread_ts", threadTS)
	logger.Info("handling mention")

	// Notify start
	a.slackClient.NotifyStart(channel, threadTS)

	// Extract instruction
	instruction := botMentionRe.ReplaceAllString(event.Text, "")
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		a.slackClient.PostThreadMessage(channel, threadTS, "指示が空です。ボットをメンションして実装内容を指示してください。")
		return
	}

	// Get repo tree
	repoTree, err := a.ghClient.GetRepoTree(ctx, a.config.DefaultBranch)
	if err != nil {
		logger.Error("failed to get repo tree", "error", err)
		a.slackClient.NotifyError(channel, threadTS, err)
		return
	}

	// Generate code
	a.slackClient.NotifyCodeGen(channel, threadTS)
	systemPrompt := claude.SystemPrompt()
	userPrompt := claude.BuildUserPrompt(instruction, repoTree)

	var claudeResp *claude.ClaudeResponse
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, err := a.claudeClient.GenerateCode(ctx, systemPrompt, userPrompt)
		if err != nil {
			logger.Error("claude API error", "error", err, "attempt", attempt+1)
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		claudeResp, err = claude.ParseResponse(raw)
		if err != nil {
			logger.Warn("failed to parse claude response", "error", err, "attempt", attempt+1)
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}
	if claudeResp == nil {
		logger.Error("all retries failed", "error", lastErr)
		a.slackClient.NotifyError(channel, threadTS, lastErr)
		return
	}

	// Create PR
	a.slackClient.NotifyCreatingPR(channel, threadTS)
	prCfg := ghclient.PRConfig{
		DefaultBranch: a.config.DefaultBranch,
		AuthorName:    a.config.AuthorName,
		AuthorEmail:   a.config.AuthorEmail,
		CoAuthorName:  a.config.CoAuthorName,
		CoAuthorEmail: a.config.CoAuthorEmail,
	}

	result, err := a.ghClient.CreatePR(ctx, prCfg, claudeResp)
	if err != nil {
		logger.Error("failed to create PR", "error", err)
		a.slackClient.NotifyError(channel, threadTS, err)
		return
	}

	logger.Info("PR created successfully", "pr_url", result.PRURL, "branch", result.BranchName)
	a.slackClient.NotifySuccess(channel, threadTS, result.PRURL)
}
