package claude

import "encoding/json"

// Stream-json event types from `claude --print --output-format stream-json --verbose`

// StreamEvent is the generic envelope for all stream events.
type StreamEvent struct {
	Type string `json:"type"`
}

// SystemEvent is the init event with session info.
type SystemEvent struct {
	Type      string `json:"type"`    // "system"
	Subtype   string `json:"subtype"` // "init"
	SessionID string `json:"session_id"`
}

// AssistantEvent contains the assistant's response message.
type AssistantEvent struct {
	Type      string           `json:"type"` // "assistant"
	Message   AssistantMessage `json:"message"`
	SessionID string           `json:"session_id"`
}

// AssistantMessage is the message payload in an assistant event.
type AssistantMessage struct {
	Content []ContentBlock `json:"content"`
}

// ContentBlock represents a content block (text or tool_use).
type ContentBlock struct {
	Type  string          `json:"type"` // "text", "tool_use"
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Result is the final result event.
type Result struct {
	Type      string  `json:"type"` // "result"
	Subtype   string  `json:"subtype"`
	SessionID string  `json:"session_id"`
	Result    string  `json:"result,omitempty"`
	IsError   bool    `json:"is_error"`
	TotalCost float64 `json:"total_cost_usd,omitempty"`
	Duration  float64 `json:"duration_ms,omitempty"`
	NumTurns  int     `json:"num_turns,omitempty"`
}

// ProgressCallback is called with progress updates during execution.
type ProgressCallback func(event ProgressEvent)

// ProgressEvent carries progress information to the caller.
type ProgressEvent struct {
	Type      ProgressType
	Text      string
	ToolName  string
	ToolID    string
	ToolInput map[string]interface{} // parsed tool input for context
	IsFinal   bool
	Result    *Result
}

type ProgressType int

const (
	ProgressText ProgressType = iota
	ProgressToolUse
	ProgressToolResult
	ProgressComplete
	ProgressError
)
