package subscriptions

import (
	"testing"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
)

// The resolver tests exercise the webhook-payload → resource_key path that
// powers subscription dispatch. They use the real embedded github.triggers.json
// so a mistake in that file (e.g. a template placeholder that doesn't match a
// ref name) is caught by the tests, not at runtime in production.

func TestResolveEventResourceKey_IssueOpened(t *testing.T) {
	cat := catalog.Global()

	payload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"name":      "ziraloop",
			"full_name": "ziraloop/ziraloop",
			"owner": map[string]any{
				"login": "ziraloop",
			},
		},
		"issue": map[string]any{
			"number": float64(42),
			"title":  "Something broke",
		},
	}

	key, ok := ResolveEventResourceKey(nil, cat,"github", "issues", "opened", payload)
	if !ok {
		t.Fatal("expected resolution to succeed")
	}
	want := "github/ziraloop/ziraloop/issue/42"
	if key != want {
		t.Errorf("resource_key = %q, want %q", key, want)
	}
}

func TestResolveEventResourceKey_GitHubAppFallsBackToGitHubVariant(t *testing.T) {
	// github-app has no triggers of its own; it falls back to the github
	// variant registered via HasProviderTriggersForVariant.
	cat := catalog.Global()

	payload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"name":      "ziraloop",
			"full_name": "ziraloop/ziraloop",
			"owner":     map[string]any{"login": "ziraloop"},
		},
		"pull_request": map[string]any{
			"number": float64(99),
		},
	}

	key, ok := ResolveEventResourceKey(nil, cat,"github-app", "pull_request", "opened", payload)
	if !ok {
		t.Fatal("expected github-app → github variant fallback to resolve")
	}
	want := "github/ziraloop/ziraloop/pull/99"
	if key != want {
		t.Errorf("resource_key = %q, want %q", key, want)
	}
}

func TestResolveEventResourceKey_MissingRefDropsEvent(t *testing.T) {
	cat := catalog.Global()

	// No issue.number in payload — template can't be satisfied.
	payload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"name":      "ziraloop",
			"full_name": "ziraloop/ziraloop",
			"owner":     map[string]any{"login": "ziraloop"},
		},
		"issue": map[string]any{
			"title": "No number field",
		},
	}

	_, ok := ResolveEventResourceKey(nil, cat,"github", "issues", "opened", payload)
	if ok {
		t.Fatal("expected resolution to fail when issue_number is missing")
	}
}

func TestResolveEventResourceKey_UnknownTriggerDrops(t *testing.T) {
	cat := catalog.Global()

	_, ok := ResolveEventResourceKey(nil, cat,"github", "not_a_real_event", "", map[string]any{})
	if ok {
		t.Fatal("expected unknown trigger to drop")
	}
}

func TestSubstituteTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		refs     map[string]string
		want     string
		wantOK   bool
	}{
		{
			name:     "happy path",
			template: "github/{owner}/{repo}/issue/{num}",
			refs:     map[string]string{"owner": "a", "repo": "b", "num": "1"},
			want:     "github/a/b/issue/1",
			wantOK:   true,
		},
		{
			name:     "no placeholders",
			template: "literal/key",
			refs:     map[string]string{},
			want:     "literal/key",
			wantOK:   true,
		},
		{
			name:     "missing ref fails",
			template: "github/{owner}/{repo}",
			refs:     map[string]string{"owner": "a"},
			wantOK:   false,
		},
		{
			name:     "empty ref value fails",
			template: "github/{owner}/{repo}",
			refs:     map[string]string{"owner": "a", "repo": ""},
			wantOK:   false,
		},
		{
			name:     "unclosed brace is kept literally",
			template: "github/{owner}/{repo",
			refs:     map[string]string{"owner": "a"},
			want:     "github/a/{repo",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := substituteTemplate(tc.template, tc.refs)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got value %q)", ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Errorf("substituted = %q, want %q", got, tc.want)
			}
		})
	}
}
