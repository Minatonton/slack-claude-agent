package slack

import (
	"fmt"

	"github.com/slack-go/slack"
)

type Client struct {
	api *slack.Client
}

func NewClient(botToken string) *Client {
	return &Client{
		api: slack.New(botToken),
	}
}

func (c *Client) PostThreadMessage(channel, threadTS, text string) error {
	_, _, err := c.api.PostMessage(
		channel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTS),
	)
	return err
}

func (c *Client) NotifyStart(channel, threadTS string) {
	c.PostThreadMessage(channel, threadTS, "作業を開始します...")
}

func (c *Client) NotifyCodeGen(channel, threadTS string) {
	c.PostThreadMessage(channel, threadTS, "コード生成中...")
}

func (c *Client) NotifyCreatingPR(channel, threadTS string) {
	c.PostThreadMessage(channel, threadTS, "PR作成中...")
}

func (c *Client) NotifySuccess(channel, threadTS, prURL string) {
	c.PostThreadMessage(channel, threadTS, fmt.Sprintf("PR作成完了: %s", prURL))
}

func (c *Client) NotifyError(channel, threadTS string, err error) {
	c.PostThreadMessage(channel, threadTS, fmt.Sprintf("エラーが発生しました: %s", err.Error()))
}
