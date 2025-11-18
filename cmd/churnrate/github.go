package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// repo represents a GitHub repository from the API.
type repo struct {
	PushedAt time.Time `json:"pushed_at"`
	Name     string    `json:"name"`
	CloneURL string    `json:"clone_url"`
	Fork     bool      `json:"fork"`
}

// fetchOrgRepos retrieves non-fork repositories for a GitHub organization.
func fetchOrgRepos(org string) ([]repo, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/repos?per_page=100", org)
	resp, err := http.Get(url) //nolint:gosec,noctx // User-controlled org name is safe for GitHub API
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // Deferred close

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var repos []repo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, err
	}

	// Filter out forks in-place
	n := 0
	for _, r := range repos {
		if !r.Fork {
			repos[n] = r
			n++
		}
	}

	return repos[:n], nil
}
