package dispatch

import (
	"reflect"
	"testing"
)

// Unit tests for refs extraction, including the coalescing fallback syntax
// used by catalog authors to handle sibling fields that exist only in some
// event variants (e.g. Slack's event.thread_ts vs event.ts).
//
// These tests are standalone — no dispatcher harness, no fixtures, just
// direct function calls with hand-rolled payload maps. The goal is to pin
// down the semantics of extractRefs, resolveRefPath, and splitFallbackPaths
// so future changes don't silently alter behavior.

func TestExtractRefs_SinglePath(t *testing.T) {
	payload := map[string]any{
		"event": map[string]any{
			"channel": "C111",
			"user":    "W222",
			"ts":      "1595926230.009600",
		},
	}
	defs := map[string]string{
		"channel_id": "event.channel",
		"user":       "event.user",
		"ts":         "event.ts",
	}

	refs, missing := extractRefs(payload, defs)

	want := map[string]string{
		"channel_id": "C111",
		"user":       "W222",
		"ts":         "1595926230.009600",
	}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("refs = %v, want %v", refs, want)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing, got %v", missing)
	}
}

func TestExtractRefs_MissingPath(t *testing.T) {
	payload := map[string]any{
		"event": map[string]any{
			"channel": "C111",
		},
	}
	defs := map[string]string{
		"channel_id": "event.channel",
		"user":       "event.user", // missing in payload
	}

	refs, missing := extractRefs(payload, defs)

	if refs["channel_id"] != "C111" {
		t.Errorf("channel_id = %q, want C111", refs["channel_id"])
	}
	if _, present := refs["user"]; present {
		t.Error("user should not be in refs when missing from payload")
	}
	if len(missing) != 1 || missing[0] != "user=event.user" {
		t.Errorf("missing = %v, want [user=event.user]", missing)
	}
}

// --- Coalescing fallback tests -------------------------------------------

func TestExtractRefs_Coalescing_FirstPathPresent(t *testing.T) {
	// When the first path in a fallback list resolves, the second is never
	// consulted — mirrors Slack's thread reply case where event.thread_ts is
	// present and should win over event.ts.
	payload := map[string]any{
		"event": map[string]any{
			"thread_ts": "1595926230.009600",
			"ts":        "1595926540.012400",
		},
	}
	defs := map[string]string{
		"thread_id": "event.thread_ts || event.ts",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["thread_id"] != "1595926230.009600" {
		t.Errorf("thread_id = %q, want 1595926230.009600 (first path should win)", refs["thread_id"])
	}
}

func TestExtractRefs_Coalescing_FirstPathMissing(t *testing.T) {
	// When the first path isn't in the payload, fall through to the second.
	// This is Slack's top-level mention case: no thread_ts, so use ts.
	payload := map[string]any{
		"event": map[string]any{
			"ts": "1595926230.009600",
		},
	}
	defs := map[string]string{
		"thread_id": "event.thread_ts || event.ts",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["thread_id"] != "1595926230.009600" {
		t.Errorf("thread_id = %q, want 1595926230.009600 (should fall through to event.ts)", refs["thread_id"])
	}
}

func TestExtractRefs_Coalescing_AllPathsMissing(t *testing.T) {
	// Neither path resolves — ref should be absent and reported as missing.
	payload := map[string]any{
		"event": map[string]any{
			"channel": "C111",
		},
	}
	defs := map[string]string{
		"thread_id": "event.thread_ts || event.ts",
	}

	refs, missing := extractRefs(payload, defs)

	if _, present := refs["thread_id"]; present {
		t.Error("thread_id should not be set when all fallback paths fail")
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing ref, got %v", missing)
	}
	if missing[0] != "thread_id=event.thread_ts || event.ts" {
		t.Errorf("missing entry = %q", missing[0])
	}
}

func TestExtractRefs_Coalescing_EmptyStringFallsThrough(t *testing.T) {
	// When the first path exists but is an empty string, treat it as "not
	// present" and fall through. This catches the case where a field is
	// always in the envelope but blank for certain event variants.
	payload := map[string]any{
		"event": map[string]any{
			"thread_ts": "",
			"ts":        "1595926230.009600",
		},
	}
	defs := map[string]string{
		"thread_id": "event.thread_ts || event.ts",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["thread_id"] != "1595926230.009600" {
		t.Errorf("thread_id = %q, want fallback to event.ts (empty string should not count as present)", refs["thread_id"])
	}
}

func TestExtractRefs_Coalescing_NilFallsThrough(t *testing.T) {
	// Explicit nil (JSON null) should also fall through.
	payload := map[string]any{
		"event": map[string]any{
			"thread_ts": nil,
			"ts":        "1595926230.009600",
		},
	}
	defs := map[string]string{
		"thread_id": "event.thread_ts || event.ts",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["thread_id"] != "1595926230.009600" {
		t.Errorf("thread_id = %q, want fallback to event.ts (nil should not count as present)", refs["thread_id"])
	}
}

func TestExtractRefs_Coalescing_ZeroIsNotEmpty(t *testing.T) {
	// A zero numeric value IS present and should resolve — coalescing is
	// about presence, not truthiness. "Is the field here?" not "is it truthy?"
	payload := map[string]any{
		"event": map[string]any{
			"count":    float64(0),
			"fallback": "unused",
		},
	}
	defs := map[string]string{
		"result": "event.count || event.fallback",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["result"] != "0" {
		t.Errorf("result = %q, want 0 (zero should resolve, not fall through)", refs["result"])
	}
}

func TestExtractRefs_Coalescing_FalseIsNotEmpty(t *testing.T) {
	// Similarly, false is a valid value that should resolve.
	payload := map[string]any{
		"event": map[string]any{
			"flag":     false,
			"fallback": "unused",
		},
	}
	defs := map[string]string{
		"result": "event.flag || event.fallback",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["result"] != "false" {
		t.Errorf("result = %q, want 'false' (false should resolve, not fall through)", refs["result"])
	}
}

func TestExtractRefs_Coalescing_ThreePaths(t *testing.T) {
	// Fallback lists with more than two options should walk all of them.
	payload := map[string]any{
		"event": map[string]any{
			"third": "found-me",
		},
	}
	defs := map[string]string{
		"result": "event.first || event.second || event.third",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["result"] != "found-me" {
		t.Errorf("result = %q, want found-me", refs["result"])
	}
}

func TestExtractRefs_Coalescing_WhitespaceVariations(t *testing.T) {
	// Different amounts of whitespace around `||` should all produce the
	// same result — this is purely a parsing concern.
	payload := map[string]any{
		"event": map[string]any{
			"ts": "1595926230.009600",
		},
	}
	variations := []string{
		"event.thread_ts||event.ts",
		"event.thread_ts || event.ts",
		"event.thread_ts  ||  event.ts",
		"  event.thread_ts || event.ts  ",
		"event.thread_ts|| event.ts",
		"event.thread_ts ||event.ts",
	}
	for _, variant := range variations {
		refs, _ := extractRefs(payload, map[string]string{"thread_id": variant})
		if refs["thread_id"] != "1595926230.009600" {
			t.Errorf("variant %q: thread_id = %q, want 1595926230.009600", variant, refs["thread_id"])
		}
	}
}

// --- splitFallbackPaths unit tests ---------------------------------------

func TestSplitFallbackPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single path", "event.channel", []string{"event.channel"}},
		{"single path with whitespace", "  event.channel  ", []string{"event.channel"}},
		{"two paths", "event.thread_ts || event.ts", []string{"event.thread_ts", "event.ts"}},
		{"two paths no spaces", "event.thread_ts||event.ts", []string{"event.thread_ts", "event.ts"}},
		{"three paths", "a || b || c", []string{"a", "b", "c"}},
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"trailing empty segment", "event.ts || ", []string{"event.ts"}},
		{"leading empty segment", " || event.ts", []string{"event.ts"}},
		{"double pipe separator only", "||", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitFallbackPaths(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("splitFallbackPaths(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// --- resolveRefPath unit tests -------------------------------------------

func TestResolveRefPath_SinglePath(t *testing.T) {
	payload := map[string]any{
		"event": map[string]any{
			"channel": "C111",
		},
	}
	value, ok := resolveRefPath(payload, "event.channel")
	if !ok {
		t.Fatal("expected to resolve")
	}
	if value != "C111" {
		t.Errorf("value = %v, want C111", value)
	}
}

func TestResolveRefPath_DeepPath(t *testing.T) {
	payload := map[string]any{
		"event": map[string]any{
			"user": map[string]any{
				"profile": map[string]any{
					"email": "alice@example.com",
				},
			},
		},
	}
	value, ok := resolveRefPath(payload, "event.user.profile.email")
	if !ok {
		t.Fatal("expected to resolve")
	}
	if value != "alice@example.com" {
		t.Errorf("value = %v", value)
	}
}

func TestResolveRefPath_TraverseNonMap(t *testing.T) {
	// Walking through a non-map intermediate segment returns not-found.
	payload := map[string]any{
		"event": map[string]any{
			"channel": "C111",
		},
	}
	_, ok := resolveRefPath(payload, "event.channel.deeper")
	if ok {
		t.Error("should not resolve through a non-map value")
	}
}

// --- Regression: existing behavior must not change ------------------------

func TestExtractRefs_NumericValueStringification(t *testing.T) {
	// Regression test: JSON-decoded numbers come back as float64, and we
	// render integers cleanly without a trailing decimal. Breaking this
	// would produce paths like /issues/1347.000000 which fail at Nango.
	payload := map[string]any{
		"issue": map[string]any{
			"number": float64(1347),
		},
		"score": map[string]any{
			"value": float64(3.14),
		},
	}
	defs := map[string]string{
		"issue_number": "issue.number",
		"score":        "score.value",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["issue_number"] != "1347" {
		t.Errorf("issue_number = %q, want 1347", refs["issue_number"])
	}
	if refs["score"] != "3.14" {
		t.Errorf("score = %q, want 3.14", refs["score"])
	}
}

func TestExtractRefs_BooleanStringification(t *testing.T) {
	payload := map[string]any{
		"pull_request": map[string]any{
			"draft":  true,
			"merged": false,
		},
	}
	defs := map[string]string{
		"draft":  "pull_request.draft",
		"merged": "pull_request.merged",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["draft"] != "true" {
		t.Errorf("draft = %q, want true", refs["draft"])
	}
	if refs["merged"] != "false" {
		t.Errorf("merged = %q, want false", refs["merged"])
	}
}

func TestExtractRefs_ArrayIndex_FirstPullRequest(t *testing.T) {
	// check_run / check_suite / workflow_run payloads expose pull_requests as
	// an array. The canonical ref for the first PR's number uses a numeric
	// segment to reach into slot 0.
	payload := map[string]any{
		"pull_requests": []any{
			map[string]any{"number": float64(42), "head": map[string]any{"ref": "feat/x"}},
			map[string]any{"number": float64(99)},
		},
	}
	defs := map[string]string{
		"pr_number":   "pull_requests.0.number",
		"pr_head_ref": "pull_requests.0.head.ref",
		"second_pr":   "pull_requests.1.number",
	}

	refs, missing := extractRefs(payload, defs)

	if refs["pr_number"] != "42" {
		t.Errorf("pr_number = %q, want 42", refs["pr_number"])
	}
	if refs["pr_head_ref"] != "feat/x" {
		t.Errorf("pr_head_ref = %q, want feat/x", refs["pr_head_ref"])
	}
	if refs["second_pr"] != "99" {
		t.Errorf("second_pr = %q, want 99", refs["second_pr"])
	}
	if len(missing) != 0 {
		t.Errorf("missing = %v, want none", missing)
	}
}

func TestExtractRefs_ArrayIndex_OutOfRangeIsMissing(t *testing.T) {
	// Events without a linked PR have pull_requests = []. Out-of-range access
	// must be treated as missing (not an error), so the ref silently drops out
	// and the dispatcher can fall back to commit-SHA-based affinity.
	payload := map[string]any{
		"pull_requests": []any{},
	}
	defs := map[string]string{
		"pr_number": "pull_requests.0.number",
	}

	refs, missing := extractRefs(payload, defs)

	if _, exists := refs["pr_number"]; exists {
		t.Errorf("pr_number should be absent when pull_requests is empty, got %q", refs["pr_number"])
	}
	if len(missing) == 0 {
		t.Error("expected pr_number in missing list")
	}
}

func TestExtractRefs_ArrayIndex_EmptyArrayCoalesces(t *testing.T) {
	// When the primary path falls into an empty array, the fallback path
	// should resolve. This is the core of the check_run affinity strategy:
	// try pull_requests[0].number first, fall back to head_sha.
	payload := map[string]any{
		"pull_requests": []any{},
		"check_run":     map[string]any{"head_sha": "abc123"},
	}
	defs := map[string]string{
		"resource": "pull_requests.0.number || check_run.head_sha",
	}

	refs, _ := extractRefs(payload, defs)

	if refs["resource"] != "abc123" {
		t.Errorf("resource = %q, want abc123 (fallback)", refs["resource"])
	}
}
