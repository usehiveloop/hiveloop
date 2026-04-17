package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/registry"
	"github.com/ziraloop/ziraloop/internal/trigger/zira"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already clean", "Debug Safari Login", "Debug Safari Login"},
		{"wrapped in double quotes", `"Debug Safari Login"`, "Debug Safari Login"},
		{"wrapped in single quotes", `'Debug Safari Login'`, "Debug Safari Login"},
		{"smart quotes", `“Debug Safari Login”`, "Debug Safari Login"},
		{"trailing period", "Debug Safari Login.", "Debug Safari Login"},
		{"trailing exclaim", "Debug Safari Login!", "Debug Safari Login"},
		{"trailing punctuation chain", "Debug Safari Login?!", "Debug Safari Login"},
		{"leading/trailing whitespace", "  Debug Safari Login  ", "Debug Safari Login"},
		{"multi-line — keeps first line only", "Debug Safari Login\n\nmore text", "Debug Safari Login"},
		{"empty", "", ""},
		{"only whitespace", "   ", ""},
		{
			"too long gets truncated",
			strings.Repeat("a", 200),
			strings.Repeat("a", 120),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanTitle(tc.in); got != tc.want {
				t.Errorf("cleanTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPickCheapestModel_UnknownProvider(t *testing.T) {
	_, _, ok := pickCheapestModel("this-provider-does-not-exist")
	if ok {
		t.Fatal("expected ok=false for unknown provider")
	}
}

func TestPickCheapestModel_ReturnsCheapestWithCost(t *testing.T) {
	// Walk every real provider in the curated registry. For any provider
	// with at least one cost-annotated model, the picker must return a model
	// whose input cost equals the minimum across that provider's catalog
	// (excluding deprecated/retired and cost-less models).
	for _, provider := range registry.Global().AllProviders() {
		var expectedMin float64 = -1
		for _, m := range provider.Models {
			if m.Cost == nil {
				continue
			}
			if m.Status == "deprecated" || m.Status == "retired" {
				continue
			}
			if expectedMin < 0 || m.Cost.Input < expectedMin {
				expectedMin = m.Cost.Input
			}
		}

		gotID, _, ok := pickCheapestModel(provider.ID)

		if expectedMin < 0 {
			// Provider has no usable cost-annotated model — picker must bail.
			if ok {
				t.Errorf("provider %q: expected ok=false (no usable models), got model %q", provider.ID, gotID)
			}
			continue
		}

		if !ok {
			t.Errorf("provider %q: expected a model, got ok=false", provider.ID)
			continue
		}
		got, exists := provider.Models[gotID]
		if !exists {
			t.Errorf("provider %q: picker returned unknown model %q", provider.ID, gotID)
			continue
		}
		if got.Cost == nil {
			t.Errorf("provider %q: picked model %q has no cost (should have been filtered)", provider.ID, gotID)
			continue
		}
		if got.Cost.Input != expectedMin {
			t.Errorf("provider %q: picked %q with input cost %v, expected min %v",
				provider.ID, gotID, got.Cost.Input, expectedMin)
		}
	}
}

func TestGenerateConversationTitle_ToolPath(t *testing.T) {
	mock := zira.NewMockCompletionClient()
	mock.SetFallback(zira.CompletionResponse{
		Message: zira.Message{
			Role: "assistant",
			ToolCalls: []zira.ToolCall{{
				ID:        "call_1",
				Name:      "submit_title",
				Arguments: `{"title": "Debug Safari Login Regression"}`,
			}},
		},
	})

	title, err := generateConversationTitle(context.Background(), mock, "some-model", true, "Login broken on Safari 17.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Debug Safari Login Regression" {
		t.Errorf("title = %q, want %q", title, "Debug Safari Login Regression")
	}

	// Verify the request was structured for tool use.
	req := mock.LastRequest()
	if req.ToolChoice != "required" {
		t.Errorf("ToolChoice = %q, want %q", req.ToolChoice, "required")
	}
	if len(req.Tools) != 1 || req.Tools[0].Name != "submit_title" {
		t.Errorf("expected single submit_title tool, got %+v", req.Tools)
	}
	// The tool parameter schema should be valid JSON.
	var schema map[string]any
	if err := json.Unmarshal(req.Tools[0].Parameters, &schema); err != nil {
		t.Errorf("tool parameters not valid JSON: %v", err)
	}
}

func TestGenerateConversationTitle_TextPath(t *testing.T) {
	mock := zira.NewMockCompletionClient()
	mock.SetFallback(zira.CompletionResponse{
		Message: zira.Message{
			Role:    "assistant",
			Content: "Debug Safari Login",
		},
	})

	title, err := generateConversationTitle(context.Background(), mock, "some-model", false, "Login broken on Safari 17.2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Debug Safari Login" {
		t.Errorf("title = %q, want %q", title, "Debug Safari Login")
	}

	// Verify the request did NOT configure tools when the model doesn't support them.
	req := mock.LastRequest()
	if req.ToolChoice != "" {
		t.Errorf("ToolChoice = %q, want empty (no tools)", req.ToolChoice)
	}
	if len(req.Tools) != 0 {
		t.Errorf("expected no tools, got %+v", req.Tools)
	}
}

func TestGenerateConversationTitle_ToolPathInvalidArgs(t *testing.T) {
	mock := zira.NewMockCompletionClient()
	mock.SetFallback(zira.CompletionResponse{
		Message: zira.Message{
			Role: "assistant",
			ToolCalls: []zira.ToolCall{{
				ID:        "call_1",
				Name:      "submit_title",
				Arguments: `{"title": `, // malformed
			}},
		},
	})

	_, err := generateConversationTitle(context.Background(), mock, "some-model", true, "hello")
	if err == nil {
		t.Fatal("expected error on malformed tool args")
	}
}

func TestGenerateConversationTitle_ToolPathNoToolCallsFallsBackToContent(t *testing.T) {
	// Edge case: model was told to use a tool but ignored the instruction
	// and returned text anyway. Current handler falls back to Content.
	mock := zira.NewMockCompletionClient()
	mock.SetFallback(zira.CompletionResponse{
		Message: zira.Message{
			Role:    "assistant",
			Content: "Fallback Title",
		},
	})

	title, err := generateConversationTitle(context.Background(), mock, "some-model", true, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Fallback Title" {
		t.Errorf("title = %q, want %q", title, "Fallback Title")
	}
}

func TestNewConversationNameTask(t *testing.T) {
	id := uuid.New()

	task, err := NewConversationNameTask(id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TypeConversationName {
		t.Errorf("type = %q, want %q", task.Type(), TypeConversationName)
	}

	var payload ConversationNamePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if payload.ConversationID != id {
		t.Errorf("payload conversation id = %v, want %v", payload.ConversationID, id)
	}
}
