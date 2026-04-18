package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// RegisterTools is a callback matching mcpserver.SubscriptionToolsFunc that
// registers subscribe_to_events on the MCP server. It's a package-level
// function returning a closure so the MCP server can call it without
// importing this package at construction time (avoids a circular dep with
// the builder).
//
// The closure resolves agent + conversation from the token metadata:
//   - token.Meta["agent_id"] is the worker agent ID (populated at token mint)
//   - conversation_id is looked up via agent_conversations.token_id = token.ID
//
// If any of those lookups fail, the tool registers but every call returns a
// helpful error — the agent sees the failure, not a panic.
func RegisterTools(svc *Service) func(server *mcp.Server, token *model.Token, db *gorm.DB) {
	return func(server *mcp.Server, token *model.Token, db *gorm.DB) {
		registerSubscribeTool(server, svc, token, db)
	}
}

func registerSubscribeTool(server *mcp.Server, svc *Service, token *model.Token, db *gorm.DB) {
	server.AddTool(
		&mcp.Tool{
			Name: "subscribe_to_events",
			Description: `Subscribe this conversation to future webhook events for a specific external resource (a GitHub PR, issue, Linear ticket, Slack thread, etc.). Call this immediately after creating any resource whose follow-up events you need to receive.

Call as many times as needed for different resources. Re-subscribing to the same resource in the same conversation is a no-op.

Available resource types and their id formats appear in the system reminder at the top of each message.

resource_id format is per-type — see the reminder for the expected shape (e.g. "owner/repo#99" for github_pull_request). If the id format is wrong, the tool returns an error naming the expected shape.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"resource_type": map[string]any{
						"type":        "string",
						"description": "One of the resource types available to this agent (e.g. github_pull_request). See the system reminder for the full list.",
					},
					"resource_id": map[string]any{
						"type":        "string",
						"description": "Resource identifier matching the format for this resource_type (e.g. 'ziraloop/ziraloop#99' for github_pull_request). See the system reminder for examples.",
					},
				},
				"required": []string{"resource_type", "resource_id"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				ResourceType string `json:"resource_type"`
				ResourceID   string `json:"resource_id"`
			}
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &params)
			}
			resourceType := strings.ToLower(strings.TrimSpace(params.ResourceType))
			resourceID := strings.TrimSpace(params.ResourceID)

			if resourceType == "" {
				return toolError("resource_type is required"), nil
			}
			if resourceID == "" {
				return toolError("resource_id is required"), nil
			}

			agentID, conversationID, orgID, err := resolveCallerContext(ctx, token, db)
			if err != nil {
				return toolError(err.Error()), nil
			}

			result, err := svc.Subscribe(ctx, SubscribeRequest{
				OrgID:          orgID,
				AgentID:        agentID,
				ConversationID: conversationID,
				ResourceType:   resourceType,
				ResourceID:     resourceID,
			})
			if err != nil {
				return toolError(err.Error()), nil
			}

			return toolJSON(map[string]any{
				"ok":              true,
				"subscription_id": result.SubscriptionID.String(),
				"resource_type":   result.ResourceType,
				"resource_id":     result.ResourceID,
				"resource_key":    result.ResourceKey,
				"provider":        result.Provider,
				"idempotent":      result.Idempotent,
				"listening_for":   result.Events,
			})
		},
	)
}

// resolveCallerContext extracts (agent_id, conversation_id, org_id) from the
// token that authenticated this MCP session. The token's Meta map carries
// agent_id (populated at mint time); the conversation is found via the
// reverse FK on agent_conversations.
func resolveCallerContext(ctx context.Context, token *model.Token, db *gorm.DB) (agentID, conversationID, orgID uuid.UUID, err error) {
	if token == nil {
		err = errors.New("no active auth token on this session; subscribe_to_events requires an agent context")
		return
	}

	orgID = token.OrgID

	agentIDStr, _ := token.Meta["agent_id"].(string)
	if agentIDStr == "" {
		err = errors.New("token is not bound to an agent; subscribe_to_events can only be called from an agent conversation")
		return
	}
	agentID, err = uuid.Parse(agentIDStr)
	if err != nil {
		err = fmt.Errorf("token.agent_id is not a valid UUID: %w", err)
		return
	}

	var conv model.AgentConversation
	lookup := db.WithContext(ctx).
		Where("token_id = ? AND status = ?", token.ID, "active").
		Order("created_at DESC").
		Limit(1).
		First(&conv)
	if lookup.Error != nil {
		if errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			err = errors.New("no active conversation is bound to this token; subscribe_to_events must be called from inside a live conversation")
			return
		}
		err = fmt.Errorf("looking up conversation for token: %w", lookup.Error)
		return
	}
	conversationID = conv.ID
	return
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func toolJSON(value any) (*mcp.CallToolResult, error) {
	buf, err := json.Marshal(value)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
	}, nil
}

