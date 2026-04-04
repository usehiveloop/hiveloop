package forge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ziraloop/ziraloop/internal/bridge"
)

// BridgeReader reads forge agent responses via direct SSE connection to Bridge.
// No webhooks are used — the forge controller opens a long-lived SSE stream
// and collects content deltas until a terminal event.
type BridgeReader struct{}

// ReadFullResponse sends a message to a Bridge conversation, then opens an SSE
// stream and blocks until the agent's complete response is available.
// Returns the concatenated text content from all content delta events.
func (r *BridgeReader) ReadFullResponse(ctx context.Context, client *bridge.BridgeClient, convID, message string) (string, error) {
	// Open SSE stream BEFORE sending the message to avoid missing events.
	stream, err := client.SSEStream(ctx, convID)
	if err != nil {
		return "", fmt.Errorf("opening SSE stream: %w", err)
	}
	defer stream.Close()

	// Now send the message (async — returns 202 immediately).
	if err := client.SendMessage(ctx, convID, message); err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	// Read SSE events until a terminal event arrives.
	scanner := bufio.NewScanner(stream)
	// Increase scanner buffer for large responses.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var text strings.Builder
	for scanner.Scan() {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		line := scanner.Text()

		// SSE format: "data: {json}" or comments (": ping") or empty lines.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		// Parse the SSE event envelope.
		var event sseEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// Skip unparseable events.
			continue
		}

		switch {
		case isContentDelta(event):
			text.WriteString(extractTextDelta(event))

		case isTerminalEvent(event):
			return text.String(), nil

		case isErrorEvent(event):
			return "", fmt.Errorf("bridge agent error: %s", data)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading SSE stream: %w", err)
	}

	// Stream ended without terminal event — return what we have.
	if text.Len() > 0 {
		return text.String(), nil
	}
	return "", fmt.Errorf("SSE stream ended without response")
}

// BridgeResponse is a richer response that captures both text and tool call events.
type BridgeResponse struct {
	Text      string         `json:"text"`
	ToolCalls []ToolCallInfo `json:"tool_calls,omitempty"`
}

// ToolCallInfo captures a tool call observed during eval execution.
type ToolCallInfo struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ReadFullResponseWithTools is like ReadFullResponse but also captures tool call events
// for eval observability. Used by the forge controller when running evals via Bridge.
func (r *BridgeReader) ReadFullResponseWithTools(ctx context.Context, client *bridge.BridgeClient, convID, message string) (*BridgeResponse, error) {
	stream, err := client.SSEStream(ctx, convID)
	if err != nil {
		return nil, fmt.Errorf("opening SSE stream: %w", err)
	}
	defer stream.Close()

	if err := client.SendMessage(ctx, convID, message); err != nil {
		return nil, fmt.Errorf("sending message: %w", err)
	}

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	resp := &BridgeResponse{}
	var text strings.Builder

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var event sseEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch {
		case isContentDelta(event):
			text.WriteString(extractTextDelta(event))

		case isToolCallEvent(event):
			if tc := extractToolCall(event); tc != nil {
				resp.ToolCalls = append(resp.ToolCalls, *tc)
			}

		case isTerminalEvent(event):
			resp.Text = text.String()
			return resp, nil

		case isErrorEvent(event):
			return nil, fmt.Errorf("bridge agent error: %s", data)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading SSE stream: %w", err)
	}

	resp.Text = text.String()
	if resp.Text != "" {
		return resp, nil
	}
	return nil, fmt.Errorf("SSE stream ended without response")
}

// sseEvent is a flexible envelope for Bridge SSE events.
// Bridge event formats vary, so we try multiple field names.
type sseEvent struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Text      string          `json:"text"`
	Content   string          `json:"content"`
	Delta     *struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	} `json:"delta"`
}

func (e sseEvent) eventName() string {
	if e.Type != "" {
		return e.Type
	}
	if e.EventType != "" {
		return e.EventType
	}
	return e.Event
}

func isContentDelta(e sseEvent) bool {
	name := strings.ToLower(e.eventName())
	return name == "content_delta" ||
		name == "contentdelta" ||
		name == "content_block_delta" ||
		name == "text_delta"
}

func isTerminalEvent(e sseEvent) bool {
	name := strings.ToLower(e.eventName())
	return name == "response_completed" ||
		name == "responsecompleted" ||
		name == "turn_completed" ||
		name == "turncompleted" ||
		name == "message_stop" ||
		name == "done"
}

func isErrorEvent(e sseEvent) bool {
	name := strings.ToLower(e.eventName())
	return name == "error" ||
		name == "agent_error" ||
		name == "agenterror"
}

func isToolCallEvent(e sseEvent) bool {
	name := strings.ToLower(e.eventName())
	return name == "tool_use" ||
		name == "tooluse" ||
		name == "tool_call" ||
		name == "toolcall" ||
		name == "function_call"
}

func extractToolCall(e sseEvent) *ToolCallInfo {
	if len(e.Data) > 0 {
		var tc struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Input     any    `json:"input"`
		}
		if json.Unmarshal(e.Data, &tc) == nil && tc.Name != "" {
			args := tc.Arguments
			if args == "" && tc.Input != nil {
				b, _ := json.Marshal(tc.Input)
				args = string(b)
			}
			return &ToolCallInfo{Name: tc.Name, Arguments: args}
		}
	}
	return nil
}

func extractTextDelta(e sseEvent) string {
	// Try multiple common formats.
	if e.Text != "" {
		return e.Text
	}
	if e.Content != "" {
		return e.Content
	}
	if e.Delta != nil {
		if e.Delta.Text != "" {
			return e.Delta.Text
		}
		if e.Delta.Content != "" {
			return e.Delta.Content
		}
	}

	// Try to extract from nested data field.
	if len(e.Data) > 0 {
		var nested struct {
			Text    string `json:"text"`
			Content string `json:"content"`
			Delta   *struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal(e.Data, &nested) == nil {
			if nested.Text != "" {
				return nested.Text
			}
			if nested.Content != "" {
				return nested.Content
			}
			if nested.Delta != nil && nested.Delta.Text != "" {
				return nested.Delta.Text
			}
		}
	}

	return ""
}
