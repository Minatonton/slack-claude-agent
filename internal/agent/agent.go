package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
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
	user := event.User
	text := event.Text

	// Determine thread timestamp
	// If this is a reply in a thread, use the thread's root timestamp
	// Otherwise, use the current message timestamp (start a new thread)
	threadTS := event.TS
	if event.ThreadTS != "" {
		threadTS = event.ThreadTS
	}

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
		case domain.CommandSync:
			session.SetExecutionMode(domain.ExecutionSync)
			a.slackClient.PostThreadMessage(channel, threadTS,
				fmt.Sprintf(":arrow_forward: 順次実行モードに切り替えました（タスクを1つずつ順番に実行）"))
			return
		case domain.CommandAsync:
			session.SetExecutionMode(domain.ExecutionAsync)
			a.slackClient.PostThreadMessage(channel, threadTS,
				fmt.Sprintf(":fast_forward: 並列実行モードに切り替えました（複数タスクを同時実行）"))
			return
		case domain.CommandListPRs:
			a.handleListPRs(session)
			return
		case domain.CommandReviewPR:
			a.handleReviewPR(session, instruction)
			return
		case domain.CommandHelp:
			a.handleHelp(channel, threadTS)
			return
		}

		// Check if already running
		if session.Running() {
			execMode := session.GetExecutionMode()
			if execMode == domain.ExecutionSync {
				a.slackClient.PostThreadMessage(channel, threadTS,
					":hourglass: 順次実行モード：現在実行中です。完了後にもう一度メンションしてください。")
			} else {
				a.slackClient.PostThreadMessage(channel, threadTS,
					":warning: 並列実行モード：既に実行中です。新しいタスクを開始する場合は別のスレッドを使用してください。")
			}
			return
		}
	} else {
		// Handle non-session commands
		switch cmd {
		case domain.CommandRepos:
			a.handleListReposNoSession(channel, threadTS)
			return
		case domain.CommandListPRs:
			a.handleListPRsNoSession(channel, threadTS)
			return
		case domain.CommandHelp:
			a.handleHelp(channel, threadTS)
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
	execMode := session.GetExecutionMode()
	execIcon := ":fast_forward:"
	if execMode == domain.ExecutionSync {
		execIcon = ":arrow_forward:"
	}
	msgTS, _ := a.slackClient.PostThreadMessageReturningTS(channel, threadTS,
		fmt.Sprintf(":hourglass_flowing_sand: タスクを開始します... (リポジトリ: %s, モード: 実装, %s %s)",
			repo.Key(), execIcon, execMode.String()))
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

	// Post new status message (emphasize continuation)
	mode := session.GetMode()
	modeIcon := ":hammer_and_wrench:"
	if mode == domain.ModeReview {
		modeIcon = ":mag:"
	}

	execMode := session.GetExecutionMode()
	execIcon := ":fast_forward:"
	if execMode == domain.ExecutionSync {
		execIcon = ":arrow_forward:"
	}

	repo := session.GetRepository()
	msgTS, _ := a.slackClient.PostThreadMessageReturningTS(session.Channel, session.ThreadTS,
		fmt.Sprintf(":speech_balloon: 会話を継続中... (リポジトリ: %s, モード: %s %s, %s %s)",
			repo.Key(), modeIcon, mode.String(), execIcon, execMode.String()))
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
	// ツール実行時のみ新規メッセージを投稿（ログを残すため）
	if len(tools) == 0 {
		return
	}

	last := tools[len(tools)-1]
	message := fmt.Sprintf(":wrench: %s", last.Summary)

	// 新規メッセージとして投稿（更新しない）
	a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS, message)
}

func (a *Agent) updateMessage(session *domain.Session, text string) {
	// 常に新規メッセージとして投稿（ログを残すため）
	a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS, text)
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

func (a *Agent) handleListPRs(session *domain.Session) {
	a.handleListPRsNoSession(session.Channel, session.ThreadTS)
}

func (a *Agent) handleListPRsNoSession(channel, threadTS string) {
	// Get current repository
	var repo *domain.Repository
	if threadTS != "" {
		a.mu.RLock()
		session, exists := a.sessions[threadTS]
		a.mu.RUnlock()
		if exists {
			repo = session.GetRepository()
		}
	}
	if repo == nil {
		repo = a.defaultRepo
	}

	a.slackClient.PostThreadMessage(channel, threadTS,
		fmt.Sprintf(":hourglass: %s のPR一覧を取得中...", repo.Key()))

	// Get PR list using gh CLI
	prList, err := a.getPRList(repo)
	if err != nil {
		a.slackClient.PostThreadMessage(channel, threadTS,
			fmt.Sprintf(":x: PR一覧の取得に失敗しました: %s", err.Error()))
		return
	}

	if prList == "" {
		a.slackClient.PostThreadMessage(channel, threadTS,
			fmt.Sprintf(":information_source: %s には開いているPRがありません", repo.Key()))
		return
	}

	msg := fmt.Sprintf(":mag: *%s のPR一覧:*\n```\n%s\n```\n\nレビューするには: `review-pr <番号>`",
		repo.Key(), prList)
	a.slackClient.PostThreadMessage(channel, threadTS, msg)
}

func (a *Agent) handleReviewPR(session *domain.Session, instruction string) {
	prNumber := domain.ExtractPRNumber(instruction)
	if prNumber == "" {
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
			":warning: PR番号を指定してください。例: `review-pr 123`")
		return
	}

	repo := session.GetRepository()
	if repo == nil {
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
			":x: リポジトリが設定されていません")
		return
	}

	// Get PR diff
	a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
		fmt.Sprintf(":hourglass: PR #%s を取得中...", prNumber))

	prDiff, err := a.getPRDiff(repo, prNumber)
	if err != nil {
		a.slackClient.PostThreadMessage(session.Channel, session.ThreadTS,
			fmt.Sprintf(":x: PRの取得に失敗しました: %s", err.Error()))
		return
	}

	// Create review prompt
	reviewPrompt := fmt.Sprintf(`以下のPull Request (#%s)をレビューしてください。

%s

レビューポイント:
- コードの品質
- 潜在的なバグ
- パフォーマンスの問題
- セキュリティの懸念
- コーディング規約の遵守
- 改善提案

詳細なレビューコメントを提供してください。`, prNumber, prDiff)

	// Continue session with review
	session.UpdateActivity()
	go a.runClaude(session, reviewPrompt)
}

func (a *Agent) getPRList(repo *domain.Repository) (string, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--repo", repo.Key(),
		"--limit", "10")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr list failed: %w (output: %s)", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

func (a *Agent) getPRDiff(repo *domain.Repository, prNumber string) (string, error) {
	// Get PR details
	viewCmd := exec.Command("gh", "pr", "view", prNumber,
		"--repo", repo.Key())
	viewOutput, err := viewCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr view failed: %w", err)
	}

	// Get PR diff
	diffCmd := exec.Command("gh", "pr", "diff", prNumber,
		"--repo", repo.Key())
	diffOutput, err := diffCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr diff failed: %w", err)
	}

	return fmt.Sprintf("## PR詳細\n%s\n\n## Diff\n```diff\n%s\n```",
		string(viewOutput), string(diffOutput)), nil
}

func (a *Agent) handleHelp(channel, threadTS string) {
	helpText := "*📚 利用可能なコマンド*\n\n" +
		"*基本操作:*\n" +
		"• `@bot <タスク内容>` - タスクを実行\n" +
		"• `end` / `終了` / `おわり` - セッションを終了\n\n" +
		"*モード切り替え:*\n" +
		"• `review` / `レビュー` - レビューモードに切り替え\n" +
		"• `implement` / `実装` - 実装モードに切り替え（デフォルト）\n" +
		"• `sync` / `順次` - 順次実行モード\n" +
		"• `async` / `並列` - 並列実行モード（デフォルト）\n\n" +
		"*リポジトリ管理:*\n" +
		"• `repos` / `repositories` / `リポジトリ` - 利用可能なリポジトリ一覧\n" +
		"• `switch owner/repo` / `切り替え owner/repo` - リポジトリを切り替え\n\n" +
		"*PRレビュー:*\n" +
		"• `list-prs` / `prs` / `PR一覧` - 開いているPR一覧を表示\n" +
		"• `review-pr <番号>` / `PRレビュー <番号>` - 指定したPRをレビュー\n\n" +
		"*ヘルプ:*\n" +
		"• `help` / `ヘルプ` / `?` - このヘルプを表示\n\n" +
		"*使用例:*\n" +
		"```\n" +
		"@bot ユーザー認証機能を追加して\n" +
		"@bot switch myorg/frontend\n" +
		"@bot list-prs\n" +
		"@bot review-pr 123\n" +
		"```"

	a.slackClient.PostThreadMessage(channel, threadTS, helpText)
}
