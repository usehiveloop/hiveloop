package catalog

import (
	"testing"
)

// TestSubscribableResource_GitHub verifies that every github-app resource
// type declared in the catalog loads with its canonical fields intact.
// Kept tight on purpose — catalog JSON drift shouldn't churn a sprawling
// table-driven test. We check one representative field per resource.
func TestSubscribableResource_GitHub(t *testing.T) {
	c := Global()

	tests := []struct {
		resourceType      string
		wantCanonicalTmpl string
		wantIDExample     string
	}{
		{"github_issue", "github/{owner}/{repo}/issue/{number}", "hiveloop/hiveloop#42"},
		{"github_pull_request", "github/{owner}/{repo}/pull/{number}", "hiveloop/hiveloop#99"},
		{"github_discussion", "github/{owner}/{repo}/discussion/{number}", "hiveloop/hiveloop#14"},
		{"github_release", "github/{owner}/{repo}/release/{tag}", "hiveloop/hiveloop@v1.2.0"},
		{"github_commit", "github/{owner}/{repo}/commit/{sha}", "hiveloop/hiveloop@abc123d"},
		{"github_branch", "github/{owner}/{repo}/branch/{branch}", "hiveloop/hiveloop:main"},
		{"github_repository", "github/{owner}/{repo}", "hiveloop/hiveloop"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			provider, def, ok := c.GetSubscribableResource(tt.resourceType)
			if !ok {
				t.Fatalf("GetSubscribableResource(%q) missing", tt.resourceType)
			}
			if provider != "github-app" {
				t.Errorf("provider = %q, want github-app", provider)
			}
			if def.CanonicalTemplate != tt.wantCanonicalTmpl {
				t.Errorf("CanonicalTemplate = %q, want %q", def.CanonicalTemplate, tt.wantCanonicalTmpl)
			}
			if def.IDExample != tt.wantIDExample {
				t.Errorf("IDExample = %q, want %q", def.IDExample, tt.wantIDExample)
			}
			if def.IDPattern == "" {
				t.Error("IDPattern is empty")
			}
			if len(def.Events) == 0 {
				t.Error("Events list is empty")
			}
		})
	}
}

func TestSubscribableResource_UnknownType(t *testing.T) {
	c := Global()
	if _, _, ok := c.GetSubscribableResource("github_does_not_exist"); ok {
		t.Fatal("expected unknown type to return ok=false")
	}
}

func TestSubscribableResource_ListByProvider(t *testing.T) {
	c := Global()

	byProvider := c.ListSubscribableResourcesForProvider("github-app")
	if len(byProvider) != 7 {
		t.Errorf("github-app subscribable resources = %d, want 7", len(byProvider))
	}

	unknown := c.ListSubscribableResourcesForProvider("does-not-exist")
	if len(unknown) != 0 {
		t.Errorf("unknown provider returned %d resources, want 0", len(unknown))
	}
}

func TestSubscribableResource_ListAllTypesSorted(t *testing.T) {
	c := Global()
	types := c.ListSubscribableResourceTypes()
	if len(types) < 7 {
		t.Fatalf("ListSubscribableResourceTypes returned %d entries, want >= 7", len(types))
	}
	for i := 1; i < len(types); i++ {
		if types[i-1] > types[i] {
			t.Errorf("entries not sorted: %q > %q", types[i-1], types[i])
		}
	}
}
