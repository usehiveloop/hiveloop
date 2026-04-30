package system

import (
	"bytes"
	"fmt"
	"text/template"
)

// RenderUserPrompt produces the rendered user message from the task's
// UserPromptTemplate and the validated args. Uses text/template (not
// html/template) — output is a prompt to a model, not HTML, so no escape.
func RenderUserPrompt(t Task, args map[string]any) (string, error) {
	tmpl, err := template.New(t.Name).Option("missingkey=error").Parse(t.UserPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse user prompt template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, args); err != nil {
		return "", fmt.Errorf("execute user prompt template: %w", err)
	}
	return buf.String(), nil
}

// BuildLLMRequest constructs the upstream wire request for a task. Default
// implementation that most tasks use; tasks can also build their own.
func BuildLLMRequest(t Task, model string, args map[string]any, stream bool) (*LLMRequest, error) {
	rendered, err := RenderUserPrompt(t, args)
	if err != nil {
		return nil, err
	}
	msgs := []LLMMessage{}
	if t.SystemPrompt != "" {
		msgs = append(msgs, LLMMessage{Role: "system", Content: t.SystemPrompt})
	}
	msgs = append(msgs, LLMMessage{Role: "user", Content: rendered})
	req := &LLMRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   t.MaxOutputTokens,
		Temperature: t.Temperature,
		Stream:      stream,
	}
	if t.ResponseFormat == ResponseJSON {
		req.ResponseFormat = &responseSpec{Type: "json_object"}
	}
	if t.ReasoningEffort != "" {
		req.Reasoning = &reasoningSpec{Effort: t.ReasoningEffort}
	}
	if stream {
		// OpenAI requires this opt-in to include usage in the final SSE chunk.
		req.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	return req, nil
}
