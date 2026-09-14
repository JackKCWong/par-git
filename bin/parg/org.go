package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	maxOrgPages    = 100
	orgPageSize    = 100
	orgHTTPTimeout = 30 * time.Second
)

// GitHub user/org names: alphanumeric and hyphens, cannot start or end with a
// hyphen, up to 39 characters.
var orgNameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

// nonGitHubHosts is a short blocklist of popular Git hosting providers that
// are not GitHub-compatible. Anything else is treated as either github.com or
// a GitHub Enterprise Server instance, since the GitHub REST API base URL is
// derived from the host in githubAPIBase.
var nonGitHubHosts = map[string]struct{}{
	"gitlab.com":       {},
	"bitbucket.org":    {},
	"codeberg.org":     {},
	"sr.ht":            {},
	"gitea.com":        {},
	"dev.azure.com":    {},
	"visualstudio.com": {},
}

type ghRepo struct {
	CloneURL string `json:"clone_url"`
	Name     string `json:"name"`
}

func parseOrgURL(raw string) (host, org string, err error) {
	if raw == "" {
		return "", "", fmt.Errorf("URL is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("unsupported URL scheme %q (expected http or https)", u.Scheme)
	}

	if u.Host == "" {
		return "", "", fmt.Errorf("URL is missing a host")
	}

	if u.User != nil {
		return "", "", fmt.Errorf("URL must not include userinfo")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if parts[0] == "" {
		return "", "", fmt.Errorf("URL is missing an org/user path")
	}
	if len(parts) > 1 {
		return "", "", fmt.Errorf("URL must point to an org root, not a repo: %s", raw)
	}
	if !orgNameRE.MatchString(parts[0]) {
		return "", "", fmt.Errorf("invalid org/user name %q (must match GitHub naming rules)", parts[0])
	}

	return u.Host, parts[0], nil
}

func isNonGitHubHost(host string) bool {
	_, blocked := nonGitHubHosts[host]
	return blocked
}

func githubAPIBase(host string) string {
	if host == "github.com" {
		return "https://api.github.com/"
	}
	return fmt.Sprintf("https://%s/api/v3/", host)
}

func listOrgRepos(ctx context.Context, host, org string) ([]string, error) {
	apiBase := githubAPIBase(host)

	client := &http.Client{Timeout: orgHTTPTimeout}

	var urls []string
	truncated := false
	for page := 1; page <= maxOrgPages; page++ {
		apiURL := fmt.Sprintf("%sorgs/%s/repos?per_page=%d&page=%d&type=all",
			apiBase, url.PathEscape(org), orgPageSize, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to list repos: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response: %w", readErr)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned %d for %s: %s",
				resp.StatusCode, apiURL, strings.TrimSpace(string(body)))
		}

		var repos []ghRepo
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		for _, r := range repos {
			urls = append(urls, r.CloneURL)
		}

		if len(repos) < orgPageSize || !hasNextPage(resp.Header.Get("Link")) {
			break
		}

		if page == maxOrgPages {
			truncated = true
		}
	}

	if truncated {
		fmt.Fprintf(os.Stderr, "⚠️  Reached maxOrgPages=%d (%d repos); org may have more.\n",
			maxOrgPages, len(urls))
	}

	return urls, nil
}

func hasNextPage(link string) bool {
	if link == "" {
		return false
	}
	for _, part := range strings.Split(link, ",") {
		if strings.Contains(part, `rel="next"`) {
			return true
		}
	}
	return false
}
