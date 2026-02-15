package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/toshin/slack-claude-agent/internal/claude"

	gh "github.com/google/go-github/v60/github"
)

type PRConfig struct {
	DefaultBranch string
	AuthorName    string
	AuthorEmail   string
	CoAuthorName  string
	CoAuthorEmail string
}

type PRResult struct {
	PRURL      string
	BranchName string
}

func (c *Client) CreatePR(ctx context.Context, cfg PRConfig, resp *claude.ClaudeResponse) (*PRResult, error) {
	// 1. Get the latest commit SHA of default branch
	ref, _, err := c.client.Git.GetRef(ctx, c.owner, c.repo, "refs/heads/"+cfg.DefaultBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to get ref for %s: %w", cfg.DefaultBranch, err)
	}
	parentSHA := ref.GetObject().GetSHA()

	// 2. Create new branch
	branchName := "claude/" + time.Now().Format("20060102-150405") + "-" + sanitize(resp.PRTitle)
	newRef := &gh.Reference{
		Ref:    gh.String("refs/heads/" + branchName),
		Object: &gh.GitObject{SHA: gh.String(parentSHA)},
	}
	_, _, err = c.client.Git.CreateRef(ctx, c.owner, c.repo, newRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	// Cleanup on error
	cleanup := func() {
		slog.Info("cleaning up branch", "branch", branchName)
		_, err := c.client.Git.DeleteRef(ctx, c.owner, c.repo, "refs/heads/"+branchName)
		if err != nil {
			slog.Error("failed to cleanup branch", "branch", branchName, "error", err)
		}
	}

	// 3. Create blobs for each file
	var treeEntries []*gh.TreeEntry
	for _, file := range resp.Files {
		encoded := base64.StdEncoding.EncodeToString([]byte(file.Content))
		blob, _, err := c.client.Git.CreateBlob(ctx, c.owner, c.repo, &gh.Blob{
			Content:  gh.String(encoded),
			Encoding: gh.String("base64"),
		})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to create blob for %s: %w", file.Path, err)
		}

		treeEntries = append(treeEntries, &gh.TreeEntry{
			Path: gh.String(file.Path),
			Mode: gh.String("100644"),
			Type: gh.String("blob"),
			SHA:  blob.SHA,
		})
	}

	// 4. Get parent commit's tree SHA
	parentCommit, _, err := c.client.Git.GetCommit(ctx, c.owner, c.repo, parentSHA)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to get parent commit: %w", err)
	}

	// 5. Create tree
	tree, _, err := c.client.Git.CreateTree(ctx, c.owner, c.repo, parentCommit.GetTree().GetSHA(), treeEntries)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create tree: %w", err)
	}

	// 6. Create commit with author info
	now := time.Now()
	commitMessage := resp.PRTitle + "\n\nCo-authored-by: " + cfg.CoAuthorName + " <" + cfg.CoAuthorEmail + ">"
	commit, _, err := c.client.Git.CreateCommit(ctx, c.owner, c.repo, &gh.Commit{
		Message: gh.String(commitMessage),
		Tree:    tree,
		Parents: []*gh.Commit{{SHA: gh.String(parentSHA)}},
		Author: &gh.CommitAuthor{
			Name:  gh.String(cfg.AuthorName),
			Email: gh.String(cfg.AuthorEmail),
			Date:  &gh.Timestamp{Time: now},
		},
		Committer: &gh.CommitAuthor{
			Name:  gh.String(cfg.AuthorName),
			Email: gh.String(cfg.AuthorEmail),
			Date:  &gh.Timestamp{Time: now},
		},
	}, nil)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	// 7. Update branch ref to new commit
	_, _, err = c.client.Git.UpdateRef(ctx, c.owner, c.repo, &gh.Reference{
		Ref:    gh.String("refs/heads/" + branchName),
		Object: &gh.GitObject{SHA: commit.SHA},
	}, false)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to update branch ref: %w", err)
	}

	// 8. Create PR
	pull, _, err := c.client.PullRequests.Create(ctx, c.owner, c.repo, &gh.NewPullRequest{
		Title: gh.String(resp.PRTitle),
		Body:  gh.String(resp.PRBody),
		Head:  gh.String(branchName),
		Base:  gh.String(cfg.DefaultBranch),
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return &PRResult{
		PRURL:      pull.GetHTMLURL(),
		BranchName: branchName,
	}, nil
}

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9-]`)

func sanitize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNum.ReplaceAllString(s, "")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
