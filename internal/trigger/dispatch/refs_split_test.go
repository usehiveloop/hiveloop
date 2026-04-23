package dispatch

import (
	"reflect"
	"testing"
)

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
