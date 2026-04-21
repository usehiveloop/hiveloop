package subscriptions

import (
	"strings"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
)

// The parser tests don't depend on a DB — they operate over the loaded catalog
// plus the regex/template logic. Uses the real github-app catalog so the tests
// also validate that the embedded JSON is parseable.

func TestParseResourceID_GitHubPullRequest(t *testing.T) {
	_, def, ok := catalog.Global().GetSubscribableResource("github_pull_request")
	if !ok {
		t.Fatal("github_pull_request missing from catalog")
	}

	tests := []struct {
		name        string
		resourceID  string
		wantKey     string
		wantErr     bool
		errContains string
	}{
		{
			name:       "happy path",
			resourceID: "hiveloop/hiveloop#99",
			wantKey:    "github/hiveloop/hiveloop/pull/99",
		},
		{
			name:       "owner with dots and hyphens",
			resourceID: "my-org.dev/web-2#1",
			wantKey:    "github/my-org.dev/web-2/pull/1",
		},
		{
			name:        "missing hash",
			resourceID:  "hiveloop/hiveloop/99",
			wantErr:     true,
			errContains: "does not match the expected format",
		},
		{
			name:        "empty id",
			resourceID:  "",
			wantErr:     true,
			errContains: "resource_id is required",
		},
		{
			name:        "not a number",
			resourceID:  "hiveloop/hiveloop#abc",
			wantErr:     true,
			errContains: "does not match the expected format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseResourceID(def, tc.resourceID)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got key=%q", got.CanonicalKey)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.CanonicalKey != tc.wantKey {
				t.Errorf("canonical = %q, want %q", got.CanonicalKey, tc.wantKey)
			}
		})
	}
}

func TestParseResourceID_AcrossGitHubResources(t *testing.T) {
	// Smoke test that every GitHub resource type can roundtrip its own
	// IDExample back into a canonical key (the example must match the
	// pattern, and the substituted canonical must not contain unresolved
	// placeholders).
	c := catalog.Global()
	for _, resourceType := range c.ListSubscribableResourceTypes() {
		provider, def, _ := c.GetSubscribableResource(resourceType)
		if provider != "github-app" {
			continue
		}
		t.Run(resourceType, func(t *testing.T) {
			got, err := ParseResourceID(def, def.IDExample)
			if err != nil {
				t.Fatalf("parse(%q) with IDExample = error %v", resourceType, err)
			}
			if strings.Contains(got.CanonicalKey, "{") || strings.Contains(got.CanonicalKey, "}") {
				t.Errorf("canonical %q still contains unresolved placeholders", got.CanonicalKey)
			}
		})
	}
}

func TestSubstituteCanonical_UnknownPlaceholder(t *testing.T) {
	_, err := substituteCanonical("foo/{missing}", map[string]string{"owner": "a"})
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}
	if !strings.Contains(err.Error(), "{missing}") {
		t.Errorf("error should name the missing placeholder, got %q", err.Error())
	}
}
