package github

import (
	"context"
	"fmt"
	"strings"

	gh "github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type Client struct {
	client *gh.Client
	owner  string
	repo   string
}

func NewClient(pat, owner, repo string) *Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: pat},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	return &Client{
		client: gh.NewClient(tc),
		owner:  owner,
		repo:   repo,
	}
}

func (c *Client) GetRepoTree(ctx context.Context, branch string) (string, error) {
	tree, _, err := c.client.Git.GetTree(ctx, c.owner, c.repo, branch, true)
	if err != nil {
		return "", fmt.Errorf("failed to get repo tree: %w", err)
	}

	var sb strings.Builder
	for _, entry := range tree.Entries {
		sb.WriteString(entry.GetPath())
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
