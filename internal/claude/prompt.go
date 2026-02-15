package claude

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ClaudeResponse struct {
	PRTitle string       `json:"pr_title"`
	PRBody  string       `json:"pr_body"`
	Files   []FileChange `json:"files"`
}

type FileChange struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Action  string `json:"action"` // "create" | "update"
}

func SystemPrompt() string {
	return `あなたはソフトウェアエンジニアです。ユーザーの指示に従い、コードの変更を行います。

レスポンスは必ず以下のJSON形式で返してください:

` + "```json" + `
{
  "pr_title": "PRのタイトル",
  "pr_body": "変更内容の説明...",
  "files": [
    {
      "path": "path/to/file.go",
      "content": "package ...\n完全なファイル内容",
      "action": "create"
    }
  ]
}
` + "```" + `

actionは "create"（新規作成）または "update"（既存ファイル更新）を指定してください。
ファイルのcontentは完全な内容を含めてください（差分ではなく全体）。`
}

func BuildUserPrompt(instruction string, repoTree string) string {
	var sb strings.Builder
	sb.WriteString("## リポジトリのファイル構成\n")
	sb.WriteString("```\n")
	sb.WriteString(repoTree)
	sb.WriteString("\n```\n\n")
	sb.WriteString("## 指示\n")
	sb.WriteString(instruction)
	return sb.String()
}

var jsonBlockRe = regexp.MustCompile("(?s)```json\\s*\n(.*?)\n\\s*```")

func ParseResponse(raw string) (*ClaudeResponse, error) {
	matches := jsonBlockRe.FindStringSubmatch(raw)
	var jsonStr string
	if len(matches) >= 2 {
		jsonStr = matches[1]
	} else {
		// Try parsing the entire response as JSON
		jsonStr = strings.TrimSpace(raw)
	}

	var resp ClaudeResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response as JSON: %w\nraw response: %s", err, raw)
	}

	if resp.PRTitle == "" {
		return nil, fmt.Errorf("pr_title is empty")
	}
	if len(resp.Files) == 0 {
		return nil, fmt.Errorf("no files in response")
	}
	for i, f := range resp.Files {
		if f.Path == "" {
			return nil, fmt.Errorf("file[%d] has empty path", i)
		}
		if f.Content == "" {
			return nil, fmt.Errorf("file[%d] (%s) has empty content", i, f.Path)
		}
		if f.Action != "create" && f.Action != "update" {
			return nil, fmt.Errorf("file[%d] (%s) has invalid action: %s", i, f.Path, f.Action)
		}
	}

	return &resp, nil
}
