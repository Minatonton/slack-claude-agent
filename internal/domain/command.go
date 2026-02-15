package domain

import "strings"

type Command int

const (
	CommandNone Command = iota
	CommandEnd
	CommandReview
	CommandImplement
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

	return CommandNone
}
