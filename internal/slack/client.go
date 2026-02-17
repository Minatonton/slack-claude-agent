package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

type Client struct {
	api *slack.Client
}

func NewClient(api *slack.Client) *Client {
	return &Client{
		api: api,
	}
}

func (c *Client) AddReaction(channel, timestamp, emoji string) error {
	ref := slack.NewRefToMessage(channel, timestamp)
	return c.api.AddReaction(emoji, ref)
}

func (c *Client) PostMessage(channel, text string) error {
	_, _, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
	)
	return err
}

func (c *Client) PostMessageReturningTS(channel, text string) (string, error) {
	_, ts, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
	)
	return ts, err
}

func (c *Client) PostThreadMessage(channel, threadTS, text string) error {
	_, _, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	return err
}

func (c *Client) PostThreadMessageReturningTS(channel, threadTS, text string) (string, error) {
	_, ts, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	return ts, err
}

func (c *Client) UpdateThreadMessage(channel, messageTS, text string) error {
	_, _, _, err := c.api.UpdateMessage(
		channel,
		messageTS,
		slack.MsgOptionText(text, false),
	)
	return err
}

func (c *Client) NotifyError(channel, threadTS string, err error) {
	c.PostThreadMessage(channel, threadTS, fmt.Sprintf("エラーが発生しました: %s", err.Error()))
}
