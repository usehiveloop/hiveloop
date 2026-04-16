package catalog

import (
	"testing"
)

func TestResourceDef(t *testing.T) {
	c := Global()

	// Test GetResourceDef for configured providers
	tests := []struct {
		provider     string
		resourceType string
		wantExists   bool
		wantDef      ResourceDef
	}{
		{
			provider:     "slack",
			resourceType: "slack_channel",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Slack Channels",
				IDField:     "id",
				NameField:   "name_normalized",
				Icon:        "hash",
				ListAction:  "/conversations.list",
			},
		},
		{
			provider:     "github-app",
			resourceType: "repository",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Repositories",
				Description: "GitHub repositories the AI can access",
				IDField:     "full_name",
				NameField:   "name",
				Icon:        "repo",
				ListAction:  "/installation/repositories",
			},
		},
		{
			provider:     "notion",
			resourceType: "page",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Pages",
				Description: "Notion pages the AI can access",
				IDField:     "id",
				NameField:   "title",
				Icon:        "page",
				ListAction:  "/v1/search",
			},
		},
		{
			provider:     "notion",
			resourceType: "database",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Databases",
				Description: "Notion databases the AI can query",
				IDField:     "id",
				NameField:   "title",
				Icon:        "database",
				ListAction:  "/v1/search",
			},
		},
		{
			provider:     "unknown",
			resourceType: "channel",
			wantExists:   false,
		},
		{
			provider:     "slack",
			resourceType: "unknown",
			wantExists:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"_"+tt.resourceType, func(t *testing.T) {
			def, exists := c.GetResourceDef(tt.provider, tt.resourceType)
			if exists != tt.wantExists {
				t.Errorf("GetResourceDef() exists = %v, want %v", exists, tt.wantExists)
				return
			}
			if !tt.wantExists {
				return
			}
			if def.DisplayName != tt.wantDef.DisplayName {
				t.Errorf("DisplayName = %q, want %q", def.DisplayName, tt.wantDef.DisplayName)
			}
			// Only assert Description when the test supplies one — catalog
			// descriptions are prose and drift over time.
			if tt.wantDef.Description != "" && def.Description != tt.wantDef.Description {
				t.Errorf("Description = %q, want %q", def.Description, tt.wantDef.Description)
			}
			if def.IDField != tt.wantDef.IDField {
				t.Errorf("IDField = %q, want %q", def.IDField, tt.wantDef.IDField)
			}
			if def.NameField != tt.wantDef.NameField {
				t.Errorf("NameField = %q, want %q", def.NameField, tt.wantDef.NameField)
			}
			if def.Icon != tt.wantDef.Icon {
				t.Errorf("Icon = %q, want %q", def.Icon, tt.wantDef.Icon)
			}
			if def.ListAction != tt.wantDef.ListAction {
				t.Errorf("ListAction = %q, want %q", def.ListAction, tt.wantDef.ListAction)
			}
		})
	}
}

func TestListResourceTypes(t *testing.T) {
	c := Global()

	tests := []struct {
		provider   string
		wantCount  int
		wantTypes  []string
	}{
		{
			provider:  "slack",
			wantCount: 3,
			wantTypes: []string{"slack_channel", "slack_thread", "slack_user"},
		},
		{
			provider:  "github-app",
			wantCount: 10,
			wantTypes: []string{"repository", "issue", "pull_request"},
		},
		{
			provider:  "notion",
			wantCount: 2,
			wantTypes: []string{"page", "database"},
		},
		{
			provider:  "unknown",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			resources := c.ListResourceTypes(tt.provider)
			if len(resources) != tt.wantCount {
				t.Errorf("ListResourceTypes() count = %d, want %d", len(resources), tt.wantCount)
			}
			for _, wantType := range tt.wantTypes {
				if _, ok := resources[wantType]; !ok {
					t.Errorf("ListResourceTypes() missing type %q", wantType)
				}
			}
		})
	}
}

func TestHasConfigurableResources(t *testing.T) {
	c := Global()

	tests := []struct {
		provider string
		want     bool
	}{
		{"github-app", true},
		{"slack", false},
		{"notion", false},
		{"asana", false},
		{"jira", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := c.HasConfigurableResources(tt.provider)
			if got != tt.want {
				t.Errorf("HasConfigurableResources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateResourcesWithConnectionResources(t *testing.T) {
	c := Global()

	tests := []struct {
		name              string
		provider          string
		actions           []string
		requested         map[string][]string
		allowed           map[string][]string
		wantErr           bool
		wantErrContains   string
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
				if tt.wantErrContains != "" && !contains(err.Error(), tt.wantErrContains) {
					t.Errorf("ValidateResources() error = %q, want containing %q", err.Error(), tt.wantErrContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateResources() error = %v, want nil", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
