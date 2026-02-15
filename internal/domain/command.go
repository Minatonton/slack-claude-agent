package domain

import "strings"

type Command int

const (
	CommandNone Command = iota
	CommandEnd
	CommandReview
	CommandImplement
	CommandSwitch
	CommandRepos
	CommandSync  // 順次実行モード
	CommandAsync // 並列実行モード
)

// DetectCommand detects special commands in the message text.
func DetectCommand(text string) Command {
	lower := strings.ToLower(strings.TrimSpace(text))

	// End session
	if lower == "おわり" || lower == "end" || lower == "終了" {
		return CommandEnd
	}

	// Switch to review mode
	if strings.HasPrefix(lower, "review") || strings.HasPrefix(lower, "レビュー") {
		return CommandReview
	}

	// Switch to implementation mode
	if strings.HasPrefix(lower, "implement") || strings.HasPrefix(lower, "実装") {
		return CommandImplement
	}

	// Switch repository
	if strings.HasPrefix(lower, "switch ") || strings.HasPrefix(lower, "切り替え ") {
		return CommandSwitch
	}

	// List repositories
	if lower == "repos" || lower == "repositories" || lower == "リポジトリ" {
		return CommandRepos
	}

	// Set execution mode to sync
	if lower == "sync" || lower == "順次" {
		return CommandSync
	}

	// Set execution mode to async
	if lower == "async" || lower == "並列" {
		return CommandAsync
	}

	return CommandNone
}

// ExtractSwitchTarget extracts the target repository from a switch command.
// Returns the repository key (e.g., "owner/repo") or empty string if not found.
func ExtractSwitchTarget(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Try "switch owner/repo"
	if strings.HasPrefix(lower, "switch ") {
		parts := strings.SplitN(text, " ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	// Try "切り替え owner/repo"
	if strings.HasPrefix(lower, "切り替え ") {
		parts := strings.SplitN(text, " ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	return ""
}
