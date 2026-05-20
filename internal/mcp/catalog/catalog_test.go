package catalog

import (
	"sort"
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
		provider  string
		wantCount int
		wantTypes []string
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

func mapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
