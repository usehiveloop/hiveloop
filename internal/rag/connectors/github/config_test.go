package github

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConfig_ValidatesRepoOwnerRequired(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"missing", `{}`},
		{"blank", `{"repo_owner":"   "}`},
		{"null", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.raw)
			}
		})
	}
}

func TestConfig_ValidatesStateFilterEnum(t *testing.T) {
	_, err := LoadConfig(json.RawMessage(`{"repo_owner":"acme","state_filter":"merged"}`))
	if err == nil {
		t.Fatal("expected state_filter=merged to be rejected; got nil")
	}

	for _, ok := range []string{"open", "closed", "all", "Open", "  ALL  "} {
		raw := []byte(`{"repo_owner":"acme","state_filter":"` + ok + `"}`)
		if _, err := LoadConfig(raw); err != nil {
			t.Fatalf("unexpected error for state %q: %v", ok, err)
		}
	}

	cfg, err := LoadConfig(json.RawMessage(`{"repo_owner":"acme"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.StateFilter != "all" {
		t.Fatalf("StateFilter default = %q, want %q", cfg.StateFilter, "all")
	}
}

func TestConfig_RepositoriesParsedAsList(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{"repo_owner":"acme","repositories":["a,b,c"]}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(cfg.Repositories, want) {
		t.Fatalf("Repositories = %v, want %v", cfg.Repositories, want)
	}

	cfg, err = LoadConfig(json.RawMessage(`{"repo_owner":"acme","repositories":["widget","gadget"]}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.Repositories, []string{"widget", "gadget"}) {
		t.Fatalf("Repositories = %v", cfg.Repositories)
	}

	cfg, err = LoadConfig(json.RawMessage(`{"repo_owner":"acme","repositories":["widget,"]}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.Repositories, []string{"widget"}) {
		t.Fatalf("Repositories = %v", cfg.Repositories)
	}

	cfg, err = LoadConfig(json.RawMessage(`{"repo_owner":"acme","repositories":[]}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Repositories != nil {
		t.Fatalf("expected nil Repositories, got %v", cfg.Repositories)
	}
}

func TestConfig_DefaultsIncludePRsAndIssuesToTrue(t *testing.T) {
	cfg, err := LoadConfig(json.RawMessage(`{"repo_owner":"acme"}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.IncludePRs || !cfg.IncludeIssues {
		t.Fatalf("expected both toggles to default to true; got PRs=%v Issues=%v",
			cfg.IncludePRs, cfg.IncludeIssues)
	}

	cfg, err = LoadConfig(json.RawMessage(`{"repo_owner":"acme","include_prs":false,"include_issues":false}`))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.IncludePRs || cfg.IncludeIssues {
		t.Fatalf("explicit false toggles not honoured: PRs=%v Issues=%v",
			cfg.IncludePRs, cfg.IncludeIssues)
	}
}
