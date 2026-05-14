package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	githubWebhookConfigKey   = "github_webhooks"
	githubWebhookSecretKey   = "webhook_secret"
	githubWebhookName        = "web"
	githubWebhookContentJSON = "json"
)

var githubEmployeeWebhookEvents = []string{
	"pull_request",
	"pull_request_review",
	"pull_request_review_comment",
	"pull_request_review_thread",
	"issue_comment",
	"workflow_run",
	"workflow_job",
	"commit_comment",
	"issues",
}

type gitHubWebhookMetadata struct {
	ID             string   `json:"id"`
	RepositoryID   string   `json:"repository_id"`
	RepositoryName string   `json:"repository_name"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
	CreatedAt      string   `json:"created_at"`
}

type gitHubWebhookRequest struct {
	Name   string            `json:"name,omitempty"`
	Active bool              `json:"active"`
	Events []string          `json:"events"`
	Config map[string]string `json:"config"`
}

type gitHubWebhookResponse struct {
	ID     any      `json:"id"`
	Name   string   `json:"name"`
	Active bool     `json:"active"`
	Events []string `json:"events"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
	CreatedAt string `json:"created_at"`
}

func (h *AgentProfileHandler) ensureGitHubRepositoryWebhook(ctx context.Context, conn model.InConnection, providerConfigKey string, repo gitHubRepository, webhookURL string, secret string) (gitHubWebhookMetadata, error) {
	owner, name, err := splitGitHubRepoFullName(repo.FullName)
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}

	hooks, err := h.listGitHubRepositoryHooks(ctx, conn, providerConfigKey, owner, name)
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}
	if hook, ok := findGitHubWebhook(hooks, webhookURL); ok {
		if hook.Active && eventsContainAll(hook.Events, githubEmployeeWebhookEvents) {
			return metadataFromGitHubWebhook(repo, hook, webhookURL), nil
		}
		return h.updateGitHubRepositoryWebhook(ctx, conn, providerConfigKey, owner, name, repo, hook, webhookURL, secret)
	}

	meta, err := h.createGitHubRepositoryWebhook(ctx, conn, providerConfigKey, owner, name, repo, webhookURL, secret)
	if err == nil {
		return meta, nil
	}

	hooks, listErr := h.listGitHubRepositoryHooks(ctx, conn, providerConfigKey, owner, name)
	if listErr == nil {
		if hook, ok := findGitHubWebhook(hooks, webhookURL); ok {
			if hook.Active && eventsContainAll(hook.Events, githubEmployeeWebhookEvents) {
				return metadataFromGitHubWebhook(repo, hook, webhookURL), nil
			}
			return h.updateGitHubRepositoryWebhook(ctx, conn, providerConfigKey, owner, name, repo, hook, webhookURL, secret)
		}
	}
	return gitHubWebhookMetadata{}, err
}

func (h *AgentProfileHandler) listGitHubRepositoryHooks(ctx context.Context, conn model.InConnection, providerConfigKey string, owner string, repo string) ([]gitHubWebhookResponse, error) {
	path := githubRepoHooksPath(owner, repo)
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodGet, providerConfigKey, conn.NangoConnectionID, path, "", nil, "")
	if err != nil {
		return nil, err
	}
	if raw.StatusCode >= 400 {
		return nil, fmt.Errorf("github hooks list failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	var hooks []gitHubWebhookResponse
	if err := json.Unmarshal(raw.Body, &hooks); err != nil {
		return nil, fmt.Errorf("decode github hooks list: %w", err)
	}
	return hooks, nil
}

func (h *AgentProfileHandler) createGitHubRepositoryWebhook(ctx context.Context, conn model.InConnection, providerConfigKey string, owner string, repoName string, repo gitHubRepository, webhookURL string, secret string) (gitHubWebhookMetadata, error) {
	body, err := json.Marshal(gitHubWebhookRequest{
		Name:   githubWebhookName,
		Active: true,
		Events: githubEmployeeWebhookEvents,
		Config: githubWebhookConfig(webhookURL, secret),
	})
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodPost, providerConfigKey, conn.NangoConnectionID, githubRepoHooksPath(owner, repoName), "", bytes.NewReader(body), "application/json")
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}
	if raw.StatusCode >= 400 {
		return gitHubWebhookMetadata{}, fmt.Errorf("github hook create failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	var hook gitHubWebhookResponse
	if err := json.Unmarshal(raw.Body, &hook); err != nil {
		return gitHubWebhookMetadata{}, fmt.Errorf("decode github hook create: %w", err)
	}
	return metadataFromGitHubWebhook(repo, hook, webhookURL), nil
}

func (h *AgentProfileHandler) updateGitHubRepositoryWebhook(ctx context.Context, conn model.InConnection, providerConfigKey string, owner string, repoName string, repo gitHubRepository, hook gitHubWebhookResponse, webhookURL string, secret string) (gitHubWebhookMetadata, error) {
	hookID := stringFromAny(hook.ID)
	if hookID == "" {
		return gitHubWebhookMetadata{}, fmt.Errorf("github hook is missing id")
	}
	body, err := json.Marshal(gitHubWebhookRequest{
		Active: true,
		Events: githubEmployeeWebhookEvents,
		Config: githubWebhookConfig(webhookURL, secret),
	})
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}
	path := githubRepoHooksPath(owner, repoName) + "/" + url.PathEscape(hookID)
	raw, err := h.nango.RawProxyRequest(ctx, http.MethodPatch, providerConfigKey, conn.NangoConnectionID, path, "", bytes.NewReader(body), "application/json")
	if err != nil {
		return gitHubWebhookMetadata{}, err
	}
	if raw.StatusCode >= 400 {
		return gitHubWebhookMetadata{}, fmt.Errorf("github hook update failed: %d: %s", raw.StatusCode, string(raw.Body))
	}
	var updated gitHubWebhookResponse
	if err := json.Unmarshal(raw.Body, &updated); err != nil {
		return gitHubWebhookMetadata{}, fmt.Errorf("decode github hook update: %w", err)
	}
	return metadataFromGitHubWebhook(repo, updated, webhookURL), nil
}

func githubWebhookConfig(webhookURL string, secret string) map[string]string {
	return map[string]string{
		"url":          webhookURL,
		"content_type": githubWebhookContentJSON,
		"insecure_ssl": "0",
		"secret":       secret,
	}
}

func githubRepoHooksPath(owner string, repo string) string {
	return githubRepoPath(owner, repo) + "/hooks"
}

func githubRepoPath(owner string, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func splitGitHubRepoFullName(fullName string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(fullName), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository full_name must be owner/repo")
	}
	return parts[0], parts[1], nil
}

func findGitHubWebhook(hooks []gitHubWebhookResponse, webhookURL string) (gitHubWebhookResponse, bool) {
	for _, hook := range hooks {
		if hook.Name == githubWebhookName && hook.Config.URL == webhookURL {
			return hook, true
		}
	}
	return gitHubWebhookResponse{}, false
}

func eventsContainAll(actual []string, required []string) bool {
	seen := make(map[string]bool, len(actual))
	for _, event := range actual {
		seen[event] = true
	}
	for _, event := range required {
		if !seen[event] {
			return false
		}
	}
	return true
}

func metadataFromGitHubWebhook(repo gitHubRepository, hook gitHubWebhookResponse, webhookURL string) gitHubWebhookMetadata {
	createdAt := hook.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	events := hook.Events
	if len(events) == 0 {
		events = githubEmployeeWebhookEvents
	}
	return gitHubWebhookMetadata{
		ID:             stringFromAny(hook.ID),
		RepositoryID:   repo.ID,
		RepositoryName: repo.FullName,
		URL:            webhookURL,
		Events:         append([]string(nil), events...),
		CreatedAt:      createdAt,
	}
}

func githubWebhookMetadataByRepo(cfg model.JSON) map[string]gitHubWebhookMetadata {
	raw, ok := cfg[githubWebhookConfigKey]
	if !ok || raw == nil {
		return map[string]gitHubWebhookMetadata{}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return map[string]gitHubWebhookMetadata{}
	}
	out := map[string]gitHubWebhookMetadata{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]gitHubWebhookMetadata{}
	}
	return out
}

func githubWebhookMetadataToJSON(metadata map[string]gitHubWebhookMetadata) model.JSON {
	b, err := json.Marshal(metadata)
	if err != nil {
		return model.JSON{}
	}
	out := model.JSON{}
	if err := json.Unmarshal(b, &out); err != nil {
		return model.JSON{}
	}
	return out
}

func githubWebhookMetadataMatches(meta gitHubWebhookMetadata, webhookURL string) bool {
	return meta.ID != "" && meta.URL == webhookURL && eventsContainAll(meta.Events, githubEmployeeWebhookEvents)
}
