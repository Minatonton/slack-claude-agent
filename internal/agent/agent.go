package agent

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/toshin/slack-claude-agent/internal/claude"
	"github.com/toshin/slack-claude-agent/internal/domain"
	slackclient "github.com/toshin/slack-claude-agent/internal/slack"
)

var botMentionRe = regexp.MustCompile(`<@U[A-Z0-9]+>`)

type Agent struct {
	mu            sync.RWMutex
	sessions      map[string]*domain.Session    // key: threadTS
	slackClient   *slackclient.Client
	claudeRunner  *claude.Runner                // deprecated: for backward compatibility
	runners       map[string]*claude.Runner     // key: repository.Key()
	repositories  []*domain.Repository
	defaultRepo   *domain.Repository
	logger        *slog.Logger
}

func New(sc *slackclient.Client, runners map[string]*claude.Runner, repos []*domain.Repository, defaultRepo *domain.Repository, logger *slog.Logger) *Agent {
	return &Agent{
		sessions:     make(map[string]*domain.Session),
		slackClient:  sc,
		runners:      runners,
		repositories: repos,
		defaultRepo:  defaultRepo,
		logger:       logger,
	}
}

func (a *Agent) HandleMention(event slackclient.Event) {
	channel := event.Channel
	threadTS := event.TS
	user := event.User
	text := event.Text

	// Extract instruction (remove bot mention)
	instruction := botMentionRe.ReplaceAllString(text, "")
	instruction = strings.TrimSpace(instruction)

	// Detect commands
	cmd := domain.DetectCommand(instruction)

	a.mu.RLock()
	session, exists := a.sessions[threadTS]
	a.mu.RUnlock()

	// Handle commands
	if exists {
		if !session.Active() {
			return
		}

		session.UpdateActivity()

		switch cmd {
		case domain.CommandEnd:
			a.endSession(session, user)
			return
		case domain.CommandReview:
			session.SetMode(domain.ModeReview)
			a.slackClient.PostThreadMessage(channel, threadTS,
				fmt.Sprintf(":mag: レビューモードに切り替えました"))
			return
		case domain.CommandImplement:
			session.SetMode(domain.ModeImplementation)
			a.slackClient.PostThreadMessage(channel, threadTS,
				fmt.Sprintf(":hammer_and_wrench: 実装モードに切り替えました"))
			return
		case domain.CommandSwitch:
			a.handleSwitchRepo(session, instruction)
			return
		case domain.CommandRepos:
			a.handleListRepos(session)
			return
		}

		// Check if already running
		if session.Running() {
			a.slackClient.PostThreadMessage(channel, threadTS,
				":hourglass: 現在実行中です。完了後にメッセージを処理します。しばらくお待ちください。")
			return
		}
	} else {
		// Handle non-session commands
		switch cmd {
		case domain.CommandRepos:
			a.handleListReposNoSession(channel, threadTS)
			return
		}
	}

	// Create new session if not exists
	if !exists {
		a.startNewSession(channel, threadTS, user, instruction)
		return
	}

	// Continue existing session
	a.continueSession(session, instruction)
}

func (a *Agent) startNewSession(channel, threadTS, user, instruction string) {
	if instruction == "" {
		a.slackClient.PostThreadMessage(channel, threadTS, "指示が空です。ボットをメンションして実装内容を指示してください。")
		return
	}

	session := domain.NewSession(channel, threadTS, a.defaultRepo)

	a.mu.Lock()
	a.sessions[threadTS] = session
	a.mu.Unlock()

	repo := session.GetRepository()
	a.logger.Info("new session", "thread", threadTS, "channel", channel, "user", user, "repository", repo.Key())

	// Add reaction
	a.slackClient.AddReaction(channel, threadTS, "eyes")

	// Post initial message
	msgTS, _ := a.slackClient.PostThreadMessageReturningTS(channel, threadTS,
		fmt.Sprintf(":hourglass_flowing_sand: タスクを開始します... (リポジトリ: %s, モード: 実装)", repo.Key()))
	session.Mu.Lock()
	session.StatusMsgTS = msgTS
	session.Mu.Unlock()

	// Run in goroutine
	go a.runClaude(session, instruction)
}

func (a *Agent) continueSession(session *domain.Session, instruction string) {
	if instruction == "" {
		return
	}

	session.UpdateActivity()

	// Post new status message
	mode := session.GetMode()
	modeIcon := ":hammer_and_wrench:"
	if mode == domain.ModeReview {
		modeIcon = ":mag:"
	}

	msgTS, _ := a.slackClient.PostThreadMessageReturningTS(session.Channel, session.ThreadTS,
		fmt.Sprintf(":hourglass_flowing_sand: 処理中... (モード: %s %s)", modeIcon, mode.String()))
	session.Mu.Lock()
	session.StatusMsgTS = msgTS
	session.Mu.Unlock()

	go a.runClaude(session, instruction)
}

func (a *Agent) runClaude(session *domain.Session, prompt string) {
	session.SetRunning(true)
	defer session.SetRunning(false)

	startTime := time.Now()

	// Get repository-specific runner
	repo := session.GetRepository()
	if repo == nil {
		a.updateMessage(session, ":x: エラー: リポジトリが設定されていません")
		return
	}

	runner, exists := a.runners[repo.Key()]
	if !exists {
		a.updateMessage(session, fmt.Sprintf(":x: エラー: リポジトリ %s のRunnerが見つかりません", repo.Key()))
		return
	}

	// Create cancellable context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	session.Mu.Lock()
	session.CancelFunc = cancel
	session.Mu.Unlock()

	logger := a.logger.With("thread", session.ThreadTS, "channel", session.Channel, "repository", repo.Key())

	// Track progress
	var textBuf strings.Builder
	var toolHistory []toolEntry
	lastUpdate := time.Now()
	updateInterval := 3 * time.Second

	callback := func(evt claude.ProgressEvent) {
		switch evt.Type {
		case claude.ProgressText:
			textBuf.WriteString(evt.Text)
			if time.Since(lastUpdate) > updateInterval {
				a.sendProgressUpdate(session, textBuf.String(), toolHistory)
				lastUpdate = time.Now()
			}

		case claude.ProgressToolUse:
			entry := toolEntry{
				Name:    evt.ToolName,
				Summary: claude.FormatToolSummary(evt.ToolName, evt.ToolInput),
			}
			toolHistory = append(toolHistory, entry)
			a.sendProgressUpdate(session, textBuf.String(), toolHistory)
			lastUpdate = time.Now()

		case claude.ProgressComplete:
			if evt.Result != nil && evt.Result.IsError {
				a.updateMessage(session, fmt.Sprintf(":warning: エラーが発生しました: %s", evt.Result.Result))
			}
		}
	}

	// Get session info
	mode := session.GetMode()
	session.Mu.Lock()
	sessionID := session.SessionID
	session.Mu.Unlock()

	// Run claude
	result, err := runner.Run(ctx, prompt, mode, sessionID, callback)
	elapsed := time.Since(startTime)

	if err != nil {
		logger.Error("claude run failed", "error", err)
		a.updateMessage(session, fmt.Sprintf(":x: Claude実行エラー: %s", err))
		return
	}

	// Store session ID for resume
	if result != nil && result.SessionID != "" {
		session.Mu.Lock()
		session.SessionID = result.SessionID
		session.Mu.Unlock()
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

	a.updateMessage(session, finalMsg)

	// Add completion reaction
	a.slackClient.AddReaction(session.Channel, session.ThreadTS, "white_check_mark")
	logger.Info("task completed successfully", "mode", mode.String())
}

func (a *Agent) endSession(session *domain.Session, user string) {
	session.Deactivate()

	a.logger.Info("ending session", "thread", session.ThreadTS, "user", user)

	a.mu.Lock()
	delete(a.sessions, session.ThreadTS)
	a.mu.Unlock()

	a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
		":wave: セッションを終了しました。")
}

type toolEntry struct {
	Name    string
	Summary string
}

func (a *Agent) sendProgressUpdate(session *domain.Session, text string, tools []toolEntry) {
	session.Mu.Lock()
	msgTS := session.StatusMsgTS
	session.Mu.Unlock()

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
		history = append(history, fmt.Sprintf("  %s %s", ":hourglass_flowing_sand:", tools[len(tools)-1].Summary))
		parts = append(parts, strings.Join(history, "\n"))
	}

	// Show text progress (truncated)
	if text != "" {
		display := text
		if len(display) > 2000 {
			display = "...\n" + display[len(display)-2000:]
		}
		parts = append(parts, formatForSlack(display))
	}

	a.slackClient.UpdateThreadMessage(session.Channel, msgTS, strings.Join(parts, "\n\n"))
}

func (a *Agent) updateMessage(session *domain.Session, text string) {
	session.Mu.Lock()
	msgTS := session.StatusMsgTS
	session.Mu.Unlock()

	if msgTS != "" {
		a.slackClient.UpdateThreadMessage(session.Channel, msgTS, text)
	} else {
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS, text)
	}
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
	text = strings.ReplaceAll(text, "**", "*")
	text = strings.ReplaceAll(text, "###", "*")
	text = strings.ReplaceAll(text, "## ", "*")
	text = strings.ReplaceAll(text, "# ", "*")

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

func (a *Agent) handleSwitchRepo(session *domain.Session, text string) {
	target := domain.ExtractSwitchTarget(text)
	if target == "" {
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
			":warning: リポジトリ名を指定してください (例: `switch owner/repo`)")
		return
	}

	// Find repository
	repo := domain.FindRepository(a.repositories, target)
	if repo == nil {
		// Repository not found, show available repositories
		var repoList []string
		for _, r := range a.repositories {
			repoList = append(repoList, fmt.Sprintf("• %s", r.Key()))
		}
		msg := fmt.Sprintf(":x: リポジトリ `%s` が見つかりません。利用可能なリポジトリ:\n%s",
			target, strings.Join(repoList, "\n"))
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS, msg)
		return
	}

	// Switch repository
	session.SetRepository(repo)
	a.logger.Info("switched repository", "thread", session.ThreadTS, "repository", repo.Key())
	a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
		fmt.Sprintf(":arrows_counterclockwise: リポジトリを %s に切り替えました", repo.Key()))
}

func (a *Agent) handleListRepos(session *domain.Session) {
	a.handleListReposNoSession(session.Channel, session.ThreadTS)
}

func (a *Agent) handleListReposNoSession(channel, threadTS string) {
	currentRepo := ""
	if threadTS != "" {
		a.mu.RLock()
		session, exists := a.sessions[threadTS]
		a.mu.RUnlock()
		if exists {
			repo := session.GetRepository()
			if repo != nil {
				currentRepo = repo.Key()
			}
		}
	}

	var repoList []string
	for _, r := range a.repositories {
		marker := ""
		if r.Key() == currentRepo {
			marker = " :point_left: *現在のリポジトリ*"
		} else if r.Key() == a.defaultRepo.Key() && currentRepo == "" {
			marker = " _(デフォルト)_"
		}
		repoList = append(repoList, fmt.Sprintf("• %s (ブランチ: %s)%s", r.Key(), r.DefaultBranch, marker))
	}

	msg := fmt.Sprintf(":books: *利用可能なリポジトリ:*\n%s\n\nリポジトリを切り替えるには: `switch owner/repo`",
		strings.Join(repoList, "\n"))
	a.slackClient.PostThreadMessage(channel, threadTS, msg)
}
