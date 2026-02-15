package agent

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/toshin/slack-claude-agent/internal/claude"
	slackclient "github.com/toshin/slack-claude-agent/internal/slack"
)

var botMentionRe = regexp.MustCompile(`<@U[A-Z0-9]+>`)

type Agent struct {
	slackClient  *slackclient.Client
	claudeRunner *claude.Runner
	logger       *slog.Logger
}

func New(sc *slackclient.Client, runner *claude.Runner, logger *slog.Logger) *Agent {
	return &Agent{
		slackClient:  sc,
		claudeRunner: runner,
		logger:       logger,
	}
}

func (a *Agent) HandleMention(event slackclient.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	channel := event.Channel
	threadTS := event.TS

	logger := a.logger.With("channel", channel, "user", event.User, "thread_ts", threadTS)
	logger.Info("handling mention")

	// Add 👀 reaction
	if err := a.slackClient.AddReaction(channel, threadTS, "eyes"); err != nil {
		logger.Warn("failed to add reaction", "error", err)
	}

	// Extract instruction
	instruction := botMentionRe.ReplaceAllString(event.Text, "")
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		a.slackClient.PostThreadMessage(channel, threadTS, "指示が空です。ボットをメンションして実装内容を指示してください。")
		return
	}

	// Post initial message
	msgTS, err := a.slackClient.PostThreadMessageReturningTS(channel, threadTS, ":hourglass_flowing_sand: タスクを開始します...")
	if err != nil {
		logger.Error("failed to post initial message", "error", err)
		return
	}

	updateMsg := func(text string) {
		if err := a.slackClient.UpdateThreadMessage(channel, msgTS, text); err != nil {
			logger.Warn("failed to update message", "error", err)
		}
	}

	// Track progress
	var textBuf strings.Builder
	var toolHistory []toolEntry
	lastUpdate := time.Now()
	updateInterval := 3 * time.Second

	callback := func(evt claude.ProgressEvent) {
		switch evt.Type {
		case claude.ProgressText:
			textBuf.WriteString(evt.Text)
			// Batch updates to avoid rate limiting
			if time.Since(lastUpdate) > updateInterval {
				a.sendProgressUpdate(msgTS, channel, threadTS, textBuf.String(), toolHistory)
				lastUpdate = time.Now()
			}

		case claude.ProgressToolUse:
			entry := toolEntry{
				Name:    evt.ToolName,
				Summary: claude.FormatToolSummary(evt.ToolName, evt.ToolInput),
			}
			toolHistory = append(toolHistory, entry)
			a.sendProgressUpdate(msgTS, channel, threadTS, textBuf.String(), toolHistory)
			lastUpdate = time.Now()

		case claude.ProgressComplete:
			if evt.Result != nil && evt.Result.IsError {
				updateMsg(fmt.Sprintf(":warning: エラーが発生しました: %s", evt.Result.Result))
			}
		}
	}

	// Run claude
	startTime := time.Now()
	result, err := a.claudeRunner.Run(ctx, instruction, callback)
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("claude run failed", "error", err)
		updateMsg(fmt.Sprintf(":x: Claude実行エラー: %s", err))
		return
	}

	// Build final message
	finalText := textBuf.String()
	summary := buildSummary(toolHistory, result, elapsed)

	var finalMsg string
	if finalText != "" {
		finalMsg = formatForSlack(finalText) + "\n\n" + summary
	} else {
		finalMsg = summary
	}

	updateMsg(finalMsg)

	// Add completion reaction
	a.slackClient.AddReaction(channel, threadTS, "white_check_mark")
	logger.Info("task completed successfully")
}

type toolEntry struct {
	Name    string
	Summary string
}

func (a *Agent) sendProgressUpdate(msgTS, channel, threadTS, text string, tools []toolEntry) {
	if msgTS == "" {
		return
	}

	var parts []string

	// Show current activity
	if len(tools) > 0 {
		last := tools[len(tools)-1]
		parts = append(parts, fmt.Sprintf(":wrench: %s", last.Summary))
	} else {
		parts = append(parts, ":hourglass_flowing_sand: 処理中...")
	}

	// Show tool history (last 8 entries)
	if len(tools) > 1 {
		var history []string
		start := 0
		if len(tools) > 8 {
			start = len(tools) - 8
		}
		for i := start; i < len(tools)-1; i++ {
			history = append(history, fmt.Sprintf("  %s %s", ":white_check_mark:", tools[i].Summary))
		}
		// Current (last) tool with spinner
		history = append(history, fmt.Sprintf("  %s %s", ":hourglass_flowing_sand:", tools[len(tools)-1].Summary))
		parts = append(parts, strings.Join(history, "\n"))
	}

	// Show text progress (truncated to tail)
	if text != "" {
		display := text
		if len(display) > 2000 {
			display = "...\n" + display[len(display)-2000:]
		}
		parts = append(parts, formatForSlack(display))
	}

	a.slackClient.UpdateThreadMessage(channel, msgTS, strings.Join(parts, "\n\n"))
}

func buildSummary(tools []toolEntry, result *claude.Result, elapsed time.Duration) string {
	if len(tools) == 0 && result == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("───\n")

	// Tool activity log
	if len(tools) > 0 {
		sb.WriteString(":clipboard: *実行ログ:*\n")
		for i, t := range tools {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t.Summary))
		}
	}

	// Stats
	var stats []string
	stats = append(stats, fmt.Sprintf(":stopwatch: %s", formatDuration(elapsed)))
	if result != nil {
		if result.NumTurns > 0 {
			stats = append(stats, fmt.Sprintf("%d ターン", result.NumTurns))
		}
		if result.TotalCost > 0 {
			stats = append(stats, fmt.Sprintf("$%.4f", result.TotalCost))
		}
	}
	sb.WriteString(strings.Join(stats, "  |  "))

	return sb.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d分%d秒", m, s)
}

func formatForSlack(text string) string {
	// Basic conversions
	text = strings.ReplaceAll(text, "**", "*")  // bold
	text = strings.ReplaceAll(text, "###", "*") // h3 → bold
	text = strings.ReplaceAll(text, "## ", "*") // h2 → bold
	text = strings.ReplaceAll(text, "# ", "*")  // h1 → bold

	// Trim excessive whitespace
	lines := strings.Split(text, "\n")
	var result []string
	emptyCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyCount++
			if emptyCount <= 2 {
				result = append(result, "")
			}
		} else {
			emptyCount = 0
			result = append(result, line)
		}
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}
