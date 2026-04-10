package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitFetcher resolves refs and downloads tarballs from GitHub repositories.
// Other hosts can be added later by implementing the same interface.
type GitFetcher struct {
	httpClient *http.Client
	apiBase    string // default "https://api.github.com"; overridable for tests
	token      string // optional; raises the unauthenticated rate limit
}

// NewGitFetcher returns a GitFetcher that talks to github.com.
func NewGitFetcher(token string) *GitFetcher {
	return &GitFetcher{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiBase:    "https://api.github.com",
		token:      token,
	}
}

// WithAPIBase overrides the GitHub API base URL. Intended for tests.
func (g *GitFetcher) WithAPIBase(base string) *GitFetcher {
	g.apiBase = strings.TrimRight(base, "/")
	return g
}

// ResolveRef returns the commit SHA that a ref (branch, tag, or SHA) points to.
func (g *GitFetcher) ResolveRef(ctx context.Context, repoURL, ref string) (string, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/commits/%s", g.apiBase, owner, repo, url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve ref: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("resolve ref %s: github returned %d: %s", ref, resp.StatusCode, string(body))
	}

	var parsed struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode resolve ref response: %w", err)
	}
	if parsed.SHA == "" {
		return "", fmt.Errorf("resolve ref %s: empty sha in response", ref)
	}
	return parsed.SHA, nil
}

// FetchTarball downloads the .tar.gz archive of the repo at a specific commit.
// Caller is responsible for closing the returned reader.
func (g *GitFetcher) FetchTarball(ctx context.Context, repoURL, sha string) (io.ReadCloser, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", g.apiBase, owner, repo, url.PathEscape(sha))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build tarball request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tarball: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("fetch tarball %s: github returned %d: %s", sha, resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

// parseGitHubURL extracts (owner, repo) from a GitHub repository URL.
// Accepts the common shapes: https://github.com/owner/repo, with or without a
// trailing .git, with or without a trailing slash.
func parseGitHubURL(repoURL string) (string, string, error) {
	trimmed := strings.TrimSpace(repoURL)
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("parse repo url: %w", err)
	}
	host := strings.ToLower(parsed.Host)
	if host != "github.com" && host != "www.github.com" {
		return "", "", fmt.Errorf("unsupported git host %q (only github.com is supported)", parsed.Host)
	}

	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", "", fmt.Errorf("repo url %q must be of the form https://github.com/owner/repo", repoURL)
	}
	return segments[0], segments[1], nil
}
