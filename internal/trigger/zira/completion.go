// Package zira implements Zira's routing brain — the LLM-powered triage and
// enrichment layer that decides which specialist agent handles each inbound
// event and what cross-connection context to gather before dispatch.
//
// The package is provider-agnostic: it defines a CompletionClient interface
// with adapters for OpenAI-compatible providers and Anthropic. Tests use a
// mock that scripts deterministic tool-call sequences.
package zira

import (
	"context"
	"encoding/json"
)

// CompletionClient is the provider-agnostic LLM interface used by Zira's
// routing agent. Production callers use NewCompletionClient to get an
// OpenAI or Anthropic adapter based on the credential. Tests inject
// MockCompletionClient with scripted responses.
type CompletionClient interface {
	ChatCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// CompletionRequest is a provider-agnostic chat completion request with
// tool calling support.
type CompletionRequest struct {
	Model      string    `json:"model"`
	Messages   []Message `json:"messages"`
	Tools      []ToolDef `json:"tools,omitempty"`
	ToolChoice string    `json:"tool_choice,omitempty"` // "required" forces tool calls, "auto" allows text
	MaxTokens  int       `json:"max_tokens,omitempty"`
}

// Message represents a chat message. For assistant messages with tool calls,
// Content is empty and ToolCalls is populated. For tool results, ToolCallID
// identifies which call this result answers.
type Message struct {
	Role       string     `json:"role"`                  // system, user, assistant, tool
	Content    string     `json:"content,omitempty"`     // text content
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // assistant's tool invocations
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role=tool: which call this answers
	Name       string     `json:"name,omitempty"`        // for role=tool: tool name
}

// ToolDef describes a tool the LLM can call. Parameters is a JSON Schema
// object describing the function's arguments.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolCall is an LLM-generated tool invocation. ID is provider-assigned
// and must be echoed back in the tool result message.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string
}

// CompletionResponse wraps the LLM's response. The adapter normalizes
// provider-specific formats into this single shape.
type CompletionResponse struct {
	Message Message `json:"message"`
}
