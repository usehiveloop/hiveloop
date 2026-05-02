// Package subagentmcp registers the `sub_agent` MCP tool on the hiveloop
// MCP server. This tool replaces the bridge-native sub_agent primitive that
// lived inside Bridge before the Wave 2 migration: now each subagent runs in
// its own dedicated bridge sandbox, and parents reach them via this MCP tool.
//
// Execution semantics: the tool is synchronous-blocking. It pushes the
// subagent definition into a child sandbox (via Pusher), creates a child
// bridge conversation, sends the prompt, and consumes the SSE stream until
// it sees `turn_completed` (or hits timeout). The aggregated assistant text
// is returned as the MCP tool result.
package subagentmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Orchestrator is the slice of internal/sandbox.Orchestrator that this tool
// needs. Defined as an interface here so tests can pass a fake without
// pulling in the whole orchestrator graph.
type Orchestrator interface {
	EnsureSubagentSandbox(ctx context.Context, orgID, parentAgentID, subagentID uuid.UUID) (*model.Sandbox, error)
	GetBridgeClient(ctx context.Context, sb *model.Sandbox) (BridgeClient, error)
}

// Pusher is the slice of internal/sandbox.Pusher this tool needs.
type Pusher interface {
	PushAgentToSandbox(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error
}

// BridgeClient is the slice of internal/bridge.BridgeClient this tool needs.
type BridgeClient interface {
	CreateConversation(ctx context.Context, agentID string) (*CreateConversationResponse, error)
	SendMessage(ctx context.Context, convID string, content string) error
	SSEStream(ctx context.Context, convID string) (io.ReadCloser, error)
}

// CreateConversationResponse mirrors the bridge response shape we care about.
type CreateConversationResponse struct {
	ConversationID string
}

const (
	defaultTimeoutSeconds = 1800
	minTimeoutSeconds     = 60
	maxTimeoutSeconds     = 7200
)

// RegisterTools returns a callback that registers the sub_agent tool on the
// MCP server. The callback signature matches what the existing handler
// pipeline expects (server, token, db) plus orchestrator and pusher closed
// over by the outer call.
func RegisterTools(orch Orchestrator, push Pusher) func(server *mcp.Server, token *model.Token, db *gorm.DB) {
	return func(server *mcp.Server, token *model.Token, db *gorm.DB) {
		registerSubAgentTool(server, token, db, orch, push)
	}
}

func registerSubAgentTool(server *mcp.Server, token *model.Token, db *gorm.DB, orch Orchestrator, push Pusher) {
	server.AddTool(
		&mcp.Tool{
			Name:        "sub_agent",
			Description: "Delegate a task to a named sub-agent. Returns the sub-agent's final response.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"subagent_name": map[string]any{
						"type":        "string",
						"description": "Name of the sub-agent to invoke (must be attached to the calling agent).",
					},
					"prompt": map[string]any{
						"type":        "string",
						"description": "The task to delegate.",
					},
					"timeout_secs": map[string]any{
						"type":        "integer",
						"description": "Hard wall-clock cap for this delegation. Default 1800.",
						"minimum":     minTimeoutSeconds,
						"maximum":     maxTimeoutSeconds,
						"default":     defaultTimeoutSeconds,
					},
					"parent_conversation_id": map[string]any{
						"type":        "string",
						"description": "Optional UUID of the calling parent's conversation. The harness fills this in so the spawned subagent conversation can be linked back to its parent for lineage tracking.",
					},
				},
				"required": []string{"subagent_name", "prompt"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return invoke(ctx, req, token, db, orch, push)
		},
	)
}

func invoke(
	ctx context.Context,
	req *mcp.CallToolRequest,
	token *model.Token,
	db *gorm.DB,
	orch Orchestrator,
	push Pusher,
) (*mcp.CallToolResult, error) {
	var params struct {
		SubagentName         string `json:"subagent_name"`
		Prompt               string `json:"prompt"`
		TimeoutSecs          int    `json:"timeout_secs,omitempty"`
		ParentConversationID string `json:"parent_conversation_id,omitempty"`
	}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &params); err != nil {
			return toolError(fmt.Sprintf("invalid parameters: %v", err)), nil
		}
	}

	name := strings.TrimSpace(params.SubagentName)
	prompt := strings.TrimSpace(params.Prompt)
	if name == "" {
		return toolError("subagent_name is required"), nil
	}
	if prompt == "" {
		return toolError("prompt is required"), nil
	}

	timeoutSecs := params.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultTimeoutSeconds
	}
	if timeoutSecs < minTimeoutSeconds {
		timeoutSecs = minTimeoutSeconds
	}
	if timeoutSecs > maxTimeoutSeconds {
		timeoutSecs = maxTimeoutSeconds
	}

	// Resolve the calling parent agent from the token meta.
	parentIDStr, _ := token.Meta["agent_id"].(string)
	parentID, err := uuid.Parse(parentIDStr)
	if err != nil || parentID == uuid.Nil {
		return toolError("token has no parent agent_id"), nil
	}

	var parent model.Agent
	if err := db.WithContext(ctx).Where("id = ?", parentID).First(&parent).Error; err != nil {
		return toolError(fmt.Sprintf("loading parent agent: %v", err)), nil
	}
	if parent.OrgID == nil {
		return toolError("parent agent has no org"), nil
	}

	// Find the attached subagent by name. We join through agent_subagents so
	// the parent can only invoke its own attached subagents.
	var sub model.Agent
	err = db.WithContext(ctx).
		Joins("JOIN agent_subagents s ON s.subagent_id = agents.id").
		Where("s.agent_id = ? AND agents.name = ? AND agents.agent_type = ?", parent.ID, name, model.AgentTypeSubagent).
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolError(fmt.Sprintf("subagent_not_found: no attached subagent named %q", name)), nil
		}
		return toolError(fmt.Sprintf("looking up subagent: %v", err)), nil
	}

	// Provision (or reuse) the dedicated child sandbox.
	childSB, err := orch.EnsureSubagentSandbox(ctx, *parent.OrgID, parent.ID, sub.ID)
	if err != nil {
		return toolError(fmt.Sprintf("ensuring subagent sandbox: %v", err)), nil
	}

	// Push the subagent definition to its sandbox (idempotent).
	if err := push.PushAgentToSandbox(ctx, &sub, childSB); err != nil {
		return toolError(fmt.Sprintf("pushing subagent: %v", err)), nil
	}

	client, err := orch.GetBridgeClient(ctx, childSB)
	if err != nil {
		return toolError(fmt.Sprintf("getting bridge client: %v", err)), nil
	}

	convResp, err := client.CreateConversation(ctx, sub.ID.String())
	if err != nil {
		return toolError(fmt.Sprintf("creating subagent conversation: %v", err)), nil
	}

	// Persist the hiveloop AgentConversation row, linking it back to the
	// parent via ParentConversationID. ParentConversationID is optional —
	// when the harness can't determine it (e.g. router-driven flows), we
	// just leave it nil rather than refusing to delegate.
	conv := model.AgentConversation{
		OrgID:                *parent.OrgID,
		AgentID:              sub.ID,
		SandboxID:            childSB.ID,
		BridgeConversationID: convResp.ConversationID,
		Status:               "active",
	}
	if pcID, perr := uuid.Parse(strings.TrimSpace(params.ParentConversationID)); perr == nil && pcID != uuid.Nil {
		conv.ParentConversationID = &pcID
	}
	if err := db.WithContext(ctx).Create(&conv).Error; err != nil {
		return toolError(fmt.Sprintf("saving conversation: %v", err)), nil
	}

	// Send the prompt and stream the response back. We hard-cap the wait at
	// timeout_secs so a stalled subagent can't hang the parent forever.
	if err := client.SendMessage(ctx, convResp.ConversationID, prompt); err != nil {
		return toolError(fmt.Sprintf("sending prompt: %v", err)), nil
	}

	streamCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	stream, err := client.SSEStream(streamCtx, convResp.ConversationID)
	if err != nil {
		return toolError(fmt.Sprintf("opening sse stream: %v", err)), nil
	}
	defer stream.Close()

	text, terr := consumeUntilCompleted(streamCtx, stream)
	if terr != nil {
		return toolError(fmt.Sprintf("subagent run failed: %v", terr)), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}, nil
}

// consumeUntilCompleted reads SSE events from the bridge stream until it
// sees a `turn_completed` event (returns the concatenated assistant text)
// or an `agent_error` event (returns an error). Each SSE event is two lines:
//
//	event: <type>
//	data: <json>
//
// followed by a blank line. We're deliberately lenient about extra fields —
// only `event` and `data` matter here.
func consumeUntilCompleted(ctx context.Context, r io.Reader) (string, error) {
	var (
		assistant strings.Builder
		curEvent  string
		curData   string
	)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	flush := func() (done bool, err error) {
		if curEvent == "" && curData == "" {
			return false, nil
		}
		defer func() {
			curEvent, curData = "", ""
		}()

		switch curEvent {
		case "response_chunk":
			var payload struct {
				Text string `json:"text"`
			}
			if uerr := json.Unmarshal([]byte(curData), &payload); uerr == nil {
				assistant.WriteString(payload.Text)
			}
		case "turn_completed":
			return true, nil
		case "agent_error":
			var payload struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal([]byte(curData), &payload)
			if payload.Message == "" {
				payload.Message = curData
			}
			return true, fmt.Errorf("agent_error: %s", payload.Message)
		}
		return false, nil
	}

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return assistant.String(), fmt.Errorf("timeout waiting for sub_agent: %w", err)
		}
		line := scanner.Text()
		switch {
		case line == "":
			done, err := flush()
			if err != nil {
				return assistant.String(), err
			}
			if done {
				return assistant.String(), nil
			}
		case strings.HasPrefix(line, "event:"):
			curEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if curData != "" {
				curData += "\n"
			}
			curData += strings.TrimPrefix(line, "data:")
			curData = strings.TrimPrefix(curData, " ")
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return assistant.String(), fmt.Errorf("timeout waiting for sub_agent")
		}
		return assistant.String(), fmt.Errorf("reading stream: %w", err)
	}
	// Stream closed before turn_completed.
	if err := ctx.Err(); err != nil {
		return assistant.String(), fmt.Errorf("timeout waiting for sub_agent: %w", err)
	}
	return assistant.String(), fmt.Errorf("stream ended without turn_completed")
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
		IsError: true,
	}
}
