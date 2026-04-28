package system

import (
	"strings"
	"testing"
)

func TestRenderUserPrompt_Substitutes(t *testing.T) {
	task := Task{
		Name:               "x",
		UserPromptTemplate: "Shape: {{.shape}}\nGoal: {{.goal}}",
	}
	got, err := RenderUserPrompt(task, map[string]any{
		"shape": "haiku",
		"goal":  "delight",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(got, "Shape: haiku") || !strings.Contains(got, "Goal: delight") {
		t.Fatalf("substitution failed: %q", got)
	}
}

func TestRenderUserPrompt_MissingKeyIsError(t *testing.T) {
	// missingkey=error means a referenced var must be present in the map.
	task := Task{Name: "x", UserPromptTemplate: "Hi {{.who}}"}
	_, err := RenderUserPrompt(task, map[string]any{})
	if err == nil {
		t.Fatalf("expected missing-key error, got nil")
	}
}

func TestBuildLLMRequest_DefaultMessages(t *testing.T) {
	task := Task{
		Name:               "x",
		SystemPrompt:       "You are concise.",
		UserPromptTemplate: "Topic: {{.topic}}",
		MaxOutputTokens:    256,
	}
	req, err := BuildLLMRequest(task, "gpt-4.1-mini", map[string]any{"topic": "go"}, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.Model != "gpt-4.1-mini" {
		t.Fatalf("model = %q", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("messages = %d, want 2 (system+user)", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
		t.Fatalf("message roles wrong: %+v", req.Messages)
	}
	if req.Messages[1].Content != "Topic: go" {
		t.Fatalf("user content = %q", req.Messages[1].Content)
	}
	if req.MaxTokens != 256 {
		t.Fatalf("MaxTokens = %d", req.MaxTokens)
	}
	if req.StreamOptions != nil {
		t.Fatalf("non-streaming request must not set StreamOptions")
	}
}

func TestBuildLLMRequest_StreamingIncludesUsage(t *testing.T) {
	task := Task{
		Name: "x", UserPromptTemplate: "x", MaxOutputTokens: 10,
	}
	req, err := BuildLLMRequest(task, "m", map[string]any{}, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !req.Stream {
		t.Fatalf("Stream not set")
	}
	if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
		t.Fatalf("StreamOptions.IncludeUsage must be true on streaming requests")
	}
}

func TestBuildLLMRequest_JSONMode(t *testing.T) {
	task := Task{
		Name: "x", UserPromptTemplate: "x",
		MaxOutputTokens: 10, ResponseFormat: ResponseJSON,
	}
	req, err := BuildLLMRequest(task, "m", map[string]any{}, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("expected response_format=json_object, got %+v", req.ResponseFormat)
	}
}

func TestBuildLLMRequest_NoSystemPrompt(t *testing.T) {
	task := Task{
		Name: "x", UserPromptTemplate: "{{.q}}", MaxOutputTokens: 10,
	}
	req, err := BuildLLMRequest(task, "m", map[string]any{"q": "hi"}, false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("expected single user message, got %+v", req.Messages)
	}
}
