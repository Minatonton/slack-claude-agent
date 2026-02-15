package claude

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/vertex"
)

type Client struct {
	client *anthropic.Client
	model  anthropic.Model
}

func NewClient(projectID, location, model string) (*Client, error) {
	client := anthropic.NewClient(
		vertex.WithGoogleAuth(context.Background(), projectID, location),
	)

	return &Client{
		client: &client,
		model:  anthropic.Model(model),
	}, nil
}

func (c *Client) GenerateCode(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     c.model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(userPrompt),
			),
		},
	})
	if err != nil {
		return "", fmt.Errorf("claude API error: %w", err)
	}

	for _, block := range message.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in Claude response")
}
