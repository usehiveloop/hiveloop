package tasks

import (
	"errors"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/system"
)

// parseUUIDs is the gate that rejects malformed IDs with a stable error
// code. Tested directly because the round-trip through resolvePromptWriterArgs
// would force a DB harness for no extra signal.
func TestParseUUIDs_InvalidIDReturnsResolveError(t *testing.T) {
	_, err := parseUUIDs([]string{"not-a-uuid"}, "skill_id", "unknown_skill")
	if err == nil {
		t.Fatal("expected error for malformed uuid")
	}
	var rerr *system.ResolveError
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *system.ResolveError, got %T", err)
	}
	if rerr.Code != "unknown_skill" {
		t.Fatalf("expected code unknown_skill, got %q", rerr.Code)
	}
}

func TestParseUUIDs_DedupesPreservingOrder(t *testing.T) {
	a := "11111111-1111-1111-1111-111111111111"
	b := "22222222-2222-2222-2222-222222222222"
	out, err := parseUUIDs([]string{a, b, a}, "skill_id", "unknown_skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 unique ids, got %d", len(out))
	}
	if out[0].String() != a || out[1].String() != b {
		t.Fatalf("order not preserved: %v", out)
	}
}

// resolveBuiltinTools joins permission keys (which the FE sends in `tools`)
// with sandbox tool slugs, looks each up in its catalog, and degrades to
// raw-name when neither catalog has a match.
func TestResolveBuiltinTools_MergesAndLooksUpDescriptions(t *testing.T) {
	toolsObj := map[string]any{
		"bash":         "allow",
		"unknown_tool": "allow",
	}
	sandbox := []string{"chrome"}

	got := resolveBuiltinTools(toolsObj, sandbox)
	if len(got) != 3 {
		t.Fatalf("expected 3 tools (bash + unknown_tool + chrome), got %d: %#v", len(got), got)
	}

	byName := make(map[string]resolvedTool, len(got))
	for _, tool := range got {
		byName[tool.Name] = tool
	}
	// bash exists in ValidBuiltInTools → has Description
	if byName["Bash"].Description == "" {
		t.Errorf("expected description for built-in 'bash', got empty: %#v", byName)
	}
	// chrome exists in ValidSandboxTools → has Description
	if byName["Chrome browser"].Description == "" {
		t.Errorf("expected description for sandbox tool 'chrome', got empty: %#v", byName)
	}
	// unknown_tool falls back to slug
	if _, ok := byName["unknown_tool"]; !ok {
		t.Errorf("expected unknown_tool to appear with raw slug, got: %#v", byName)
	}
}

func TestResolveBuiltinTools_DedupesAcrossPermissionsAndSandbox(t *testing.T) {
	// "bash" appears in both inputs — should appear once in output.
	toolsObj := map[string]any{"bash": "allow"}
	sandbox := []string{"bash"}

	got := resolveBuiltinTools(toolsObj, sandbox)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool after dedupe, got %d: %#v", len(got), got)
	}
}

// lookupActions degrades gracefully when the catalog is absent (test
// harnesses pass nil), so resolver-level tests don't need to mount a real
// catalog just to exercise the resolver shape.
func TestLookupActions_NilCatalogReturnsSlugOnly(t *testing.T) {
	got, err := lookupActions(nil, "slack", []string{"send_message", "list_channels"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(got))
	}
	for _, a := range got {
		if a.DisplayName != "" || a.Description != "" {
			t.Errorf("expected slug-only ref when catalog is nil, got %#v", a)
		}
	}
}

// objectListArg + stringSliceArg are the parsing gateways for the trigger
// loop. Worth pinning their shape here so a refactor doesn't silently
// flip them to ignore valid input.
func TestObjectListArg_HandlesMissingAndMixedTypes(t *testing.T) {
	// missing key → nil
	if got := objectListArg(map[string]any{}, "triggers"); got != nil {
		t.Errorf("expected nil for missing key, got %#v", got)
	}
	// mixed: keep objects, drop scalars
	args := map[string]any{
		"triggers": []any{
			map[string]any{"trigger_type": "webhook"},
			"not-an-object",
			map[string]any{"trigger_type": "cron"},
		},
	}
	got := objectListArg(args, "triggers")
	if len(got) != 2 {
		t.Fatalf("expected 2 valid objects, got %d", len(got))
	}
}
