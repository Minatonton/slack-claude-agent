package domain

import (
	"fmt"
	"strings"
)

// Repository represents a GitHub repository configuration.
type Repository struct {
	Owner         string
	Name          string
	DefaultBranch string
}

// Key returns a unique identifier for the repository (owner/name).
func (r *Repository) Key() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Name)
}

// ParseRepositories parses a comma-separated list of repositories.
// Format: "owner1/repo1:branch1,owner2/repo2:branch2,owner3/repo3"
// Branch is optional; if omitted, defaultBranch is used.
func ParseRepositories(reposEnv, defaultBranch string) ([]*Repository, error) {
	if reposEnv == "" {
		return nil, nil
	}

	parts := strings.Split(reposEnv, ",")
	repos := make([]*Repository, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Split by ':' to separate repo and branch
		repoBranch := strings.Split(part, ":")
		repoPath := strings.TrimSpace(repoBranch[0])
		branch := defaultBranch
		if len(repoBranch) > 1 {
			branch = strings.TrimSpace(repoBranch[1])
		}

		// Split owner/name
		ownerName := strings.Split(repoPath, "/")
		if len(ownerName) != 2 {
			return nil, fmt.Errorf("invalid repository format: %s (expected owner/name)", part)
		}

		owner := strings.TrimSpace(ownerName[0])
		name := strings.TrimSpace(ownerName[1])

		if owner == "" || name == "" {
			return nil, fmt.Errorf("invalid repository format: %s (owner and name cannot be empty)", part)
		}

		repos = append(repos, &Repository{
			Owner:         owner,
			Name:          name,
			DefaultBranch: branch,
		})
	}

	return repos, nil
}

// FindRepository finds a repository by its key (owner/name).
func FindRepository(repos []*Repository, key string) *Repository {
	for _, repo := range repos {
		if repo.Key() == key {
			return repo
		}
	}
	return nil
}
