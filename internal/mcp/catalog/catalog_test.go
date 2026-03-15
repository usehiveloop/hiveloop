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
			resourceType: "channel",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Channels",
				Description: "Slack channels the AI can access",
				IDField:     "id",
				NameField:   "name_normalized",
				Icon:        "hash",
				ListAction:  "/conversations.list",
			},
		},
		{
			provider:     "github-app",
			resourceType: "repo",
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
			provider:     "google_drive",
			resourceType: "folder",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Folders",
				Description: "Folders the AI can access files within",
				IDField:     "id",
				NameField:   "name",
				Icon:        "folder",
				ListAction:  "/drive/v3/files",
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
			provider:     "linear",
			resourceType: "team",
			wantExists:   true,
			wantDef: ResourceDef{
				DisplayName: "Teams",
				Description: "Linear teams the AI can create issues in",
				IDField:     "id",
				NameField:   "name",
				Icon:        "team",
				ListAction:  "/graphql",
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
			if def.Description != tt.wantDef.Description {
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
			wantCount: 1,
			wantTypes: []string{"channel"},
		},
		{
			provider:  "github-app",
			wantCount: 1,
			wantTypes: []string{"repo"},
		},
		{
			provider:  "google_drive",
			wantCount: 1,
			wantTypes: []string{"folder"},
		},
		{
			provider:  "notion",
			wantCount: 2,
			wantTypes: []string{"page", "database"},
		},
		{
			provider:  "linear",
			wantCount: 1,
			wantTypes: []string{"team"},
		},
		{
			provider:  "asana",
			wantCount: 0,
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
		{"slack", true},
		{"github-app", true},
		{"google_drive", true},
		{"notion", true},
		{"linear", true},
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
			name:      "valid resources",
			provider:  "slack",
			actions:   []string{"read_messages", "send_message"},
			requested: map[string][]string{"channel": {"C123", "C456"}},
			allowed:   map[string][]string{"channel": {"C123", "C456", "C789"}},
			wantErr:   false,
		},
		{
			name:            "resource not in allowed list",
			provider:        "slack",
			actions:         []string{"read_messages"},
			requested:       map[string][]string{"channel": {"C123", "C999"}},
			allowed:         map[string][]string{"channel": {"C123", "C456"}},
			wantErr:         true,
			wantErrContains: "resource \"C999\" of type \"channel\" not configured",
		},
		{
			name:            "resource type not valid for actions",
			provider:        "slack",
			actions:         []string{"list_channels"},
			requested:       map[string][]string{"channel": {"C123"}},
			allowed:         map[string][]string{"channel": {"C123"}},
			wantErr:         true,
			wantErrContains: "resource type \"channel\" does not match any listed action",
		},
		{
			name:      "empty resources",
			provider:  "slack",
			actions:   []string{"read_messages"},
			requested: map[string][]string{},
			allowed:   map[string][]string{"channel": {"C123"}},
			wantErr:   false,
		},
		{
			name:      "nil allowed resources means no restrictions",
			provider:  "slack",
			actions:   []string{"read_messages"},
			requested: map[string][]string{"channel": {"C123"}},
			allowed:   nil,
			wantErr:   false,
		},
		{
			name:      "github repos",
			provider:  "github-app",
			actions:   []string{"list_issues", "create_issue"},
			requested: map[string][]string{"repo": {"owner/repo1"}},
			allowed:   map[string][]string{"repo": {"owner/repo1", "owner/repo2"}},
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

	// Test Linear team resource has RequestConfig with GraphQL query
	linearTeam, ok := c.GetResourceDef("linear", "team")
	if !ok {
		t.Fatal("linear team resource not found")
	}
	if linearTeam.RequestConfig == nil {
		t.Fatal("linear team RequestConfig is nil")
	}
	if linearTeam.RequestConfig.Method != "POST" {
		t.Errorf("linear team method = %q, want POST", linearTeam.RequestConfig.Method)
	}
	if linearTeam.RequestConfig.ResponsePath != "data.teams.nodes" {
		t.Errorf("linear team response_path = %q, want data.teams.nodes", linearTeam.RequestConfig.ResponsePath)
	}
	if linearTeam.RequestConfig.BodyTemplate == nil {
		t.Fatal("linear team body template is nil")
	}

	// Test Slack channel (now has RequestConfig for generic discovery)
	slackChannel, ok := c.GetResourceDef("slack", "channel")
	if !ok {
		t.Fatal("slack channel resource not found")
	}
	if slackChannel.RequestConfig == nil {
		t.Fatal("slack channel RequestConfig is nil")
	}
	if slackChannel.RequestConfig.Method != "GET" {
		t.Errorf("slack channel method = %q, want GET", slackChannel.RequestConfig.Method)
	}
	if slackChannel.RequestConfig.QueryParams == nil {
		t.Fatal("slack channel query params are nil")
	}
	if slackChannel.ListAction != "/conversations.list" {
		t.Errorf("slack channel list_action = %q, want /conversations.list", slackChannel.ListAction)
	}

	// Test GitHub repo (has RequestConfig)
	githubRepo, ok := c.GetResourceDef("github-app", "repo")
	if !ok {
		t.Fatal("github repo resource not found")
	}
	if githubRepo.RequestConfig == nil {
		t.Fatal("github repo RequestConfig is nil")
	}
	if githubRepo.ListAction != "/installation/repositories" {
		t.Errorf("github repo list_action = %q, want /installation/repositories", githubRepo.ListAction)
	}
}
