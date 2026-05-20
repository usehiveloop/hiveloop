package catalog

import (
	"strings"
	"testing"
)

func TestValidateResourcesWithConnectionResources(t *testing.T) {
	c := Global()

	tests := []struct {
		name            string
		provider        string
		actions         []string
		requested       map[string][]string
		allowed         map[string][]string
		wantErr         bool
		wantErrContains string
	}{
		{
			// conversations_history + chat_post_message are slack_thread actions.
			name:      "valid resources",
			provider:  "slack",
			actions:   []string{"conversations_history", "chat_post_message"},
			requested: map[string][]string{"slack_thread": {"ts1", "ts2"}},
			allowed:   map[string][]string{"slack_thread": {"ts1", "ts2", "ts3"}},
			wantErr:   false,
		},
		{
			name:            "resource not in allowed list",
			provider:        "slack",
			actions:         []string{"conversations_history"},
			requested:       map[string][]string{"slack_thread": {"ts1", "ts999"}},
			allowed:         map[string][]string{"slack_thread": {"ts1", "ts2"}},
			wantErr:         true,
			wantErrContains: "resource \"ts999\" of type \"slack_thread\" not configured",
		},
		{
			// api_test has no resource_type in the catalog, so any requested
			// resource type must be rejected.
			name:            "resource type not valid for actions",
			provider:        "slack",
			actions:         []string{"api_test"},
			requested:       map[string][]string{"slack_thread": {"ts1"}},
			allowed:         map[string][]string{"slack_thread": {"ts1"}},
			wantErr:         true,
			wantErrContains: "resource type \"slack_thread\" does not match any listed action",
		},
		{
			name:      "empty resources",
			provider:  "slack",
			actions:   []string{"conversations_history"},
			requested: map[string][]string{},
			allowed:   map[string][]string{"slack_thread": {"ts1"}},
			wantErr:   false,
		},
		{
			name:      "nil allowed resources means no restrictions",
			provider:  "slack",
			actions:   []string{"conversations_history"},
			requested: map[string][]string{"slack_thread": {"ts1"}},
			allowed:   nil,
			wantErr:   false,
		},
		{
			// repos_get has resource_type "repository" in the curated catalog.
			name:      "github repos",
			provider:  "github-app",
			actions:   []string{"repos_get"},
			requested: map[string][]string{"repository": {"owner/repo1"}},
			allowed:   map[string][]string{"repository": {"owner/repo1", "owner/repo2"}},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.ValidateResources(tt.provider, tt.actions, tt.requested, tt.allowed)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateResources() error = nil, want error containing %q", tt.wantErrContains)
					return
				}
				if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("ValidateResources() error = %q, want containing %q", err.Error(), tt.wantErrContains)
				}
			} else if err != nil {
				t.Errorf("ValidateResources() error = %v, want nil", err)
			}
		})
	}
}

func TestRequestConfig(t *testing.T) {
	c := Global()

	// Test Notion page resource has RequestConfig with Notion-Version header
	notionPage, ok := c.GetResourceDef("notion", "page")
	if !ok {
		t.Fatal("notion page resource not found")
	}
	if notionPage.RequestConfig == nil {
		t.Fatal("notion page RequestConfig is nil")
	}
	if notionPage.RequestConfig.Method != "POST" {
		t.Errorf("notion page method = %q, want POST", notionPage.RequestConfig.Method)
	}
	if notionPage.RequestConfig.Headers == nil {
		t.Fatal("notion page headers are nil")
	}
	if notionPage.RequestConfig.Headers["Notion-Version"] != "2022-06-28" {
		t.Errorf("notion page Notion-Version header = %q, want 2022-06-28", notionPage.RequestConfig.Headers["Notion-Version"])
	}
	if notionPage.RequestConfig.BodyTemplate == nil {
		t.Fatal("notion page body template is nil")
	}

	// Test Slack channel (now has RequestConfig for generic discovery)
	slackChannel, ok := c.GetResourceDef("slack", "slack_channel")
	if !ok {
		t.Fatal("slack_channel resource not found")
	}
	if slackChannel.RequestConfig == nil {
		t.Fatal("slack_channel RequestConfig is nil")
	}
	// Slack channel discovery goes through POST now (body-templated).
	if slackChannel.RequestConfig.Method != "POST" {
		t.Errorf("slack_channel method = %q, want POST", slackChannel.RequestConfig.Method)
	}
	if slackChannel.ListAction != "/conversations.list" {
		t.Errorf("slack_channel list_action = %q, want /conversations.list", slackChannel.ListAction)
	}

	// Test GitHub repo (has RequestConfig)
	githubRepo, ok := c.GetResourceDef("github-app", "repository")
	if !ok {
		t.Fatal("github-app repository resource not found")
	}
	if githubRepo.RequestConfig == nil {
		t.Fatal("github-app repository RequestConfig is nil")
	}
	if githubRepo.ListAction != "/installation/repositories" {
		t.Errorf("github-app repository list_action = %q, want /installation/repositories", githubRepo.ListAction)
	}
}
