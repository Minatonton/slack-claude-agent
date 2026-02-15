package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
)

// Parser processes stream-json output from Claude CLI.
type Parser struct {
	logger   *slog.Logger
	callback ProgressCallback
}

func NewParser(logger *slog.Logger, callback ProgressCallback) *Parser {
	return &Parser{
		logger:   logger,
		callback: callback,
	}
}

// Parse reads stream-json lines from the reader and calls the callback.
func (p *Parser) Parse(r io.Reader) (*Result, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB buffer

	var finalResult *Result

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var env StreamEvent
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			p.logger.Debug("skip non-json line", "line", truncate(line, 100))
			continue
		}

		p.logger.Debug("stream event", "type", env.Type)

		switch env.Type {
		case "system":
			var evt SystemEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}
			p.logger.Info("claude session", "session_id", evt.SessionID)

		case "assistant":
			var evt AssistantEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				p.logger.Error("failed to parse assistant event", "error", err)
				continue
			}
			p.handleAssistant(evt)

		case "result":
			var evt Result
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				continue
			}
			finalResult = &evt
			p.callback(ProgressEvent{
				Type:    ProgressComplete,
				IsFinal: true,
				Result:  &evt,
			})

		default:
			p.logger.Debug("unknown event type", "type", env.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return finalResult, err
	}

	return finalResult, nil
}

func (p *Parser) handleAssistant(evt AssistantEvent) {
	for _, block := range evt.Message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				p.callback(ProgressEvent{
					Type: ProgressText,
					Text: block.Text,
				})
			}
		case "tool_use":
			var inputMap map[string]interface{}
			if len(block.Input) > 0 {
				_ = json.Unmarshal(block.Input, &inputMap)
			}
			p.callback(ProgressEvent{
				Type:      ProgressToolUse,
				ToolName:  block.Name,
				ToolID:    block.ID,
				ToolInput: inputMap,
			})
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
