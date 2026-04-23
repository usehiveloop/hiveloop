package dispatch

import (
	"testing"
)

func TestExtractRefs_NumericValueStringification(t *testing.T) {
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
