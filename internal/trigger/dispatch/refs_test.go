package dispatch

import (
	"reflect"
	"testing"
)

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
		"user":       "event.user",
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

func TestExtractRefs_Coalescing_FirstPathPresent(t *testing.T) {
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
