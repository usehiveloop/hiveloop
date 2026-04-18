package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/counter"
	mcppkg "github.com/ziraloop/ziraloop/internal/mcp"
	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// MemoryToolsFunc is a callback that registers memory tools on a server.
// Used to avoid an import cycle between mcpserver and hindsight.
type MemoryToolsFunc func(server *mcp.Server, agentID string, db *gorm.DB)

// SubscriptionToolsFunc is a callback that registers subscribe_to_events and
// list_my_subscriptions on a server. Used to avoid an import cycle between
// mcpserver and the subscriptions package.
type SubscriptionToolsFunc func(server *mcp.Server, token *model.Token, db *gorm.DB)

// BuildServer creates an MCP server with tools registered from token scopes.
// Each scope's connection+actions are turned into MCP tools via the catalog.
// If addMemoryTools is non-nil, it is called to register memory tools on the
// same server after integration tools are registered.
// If addSubscriptionTools is non-nil, it is called to register subscribe_to_events
// on the same server after memory tools are registered.
func BuildServer(
	token *model.Token,
	scopes []mcppkg.TokenScope,
	cat *catalog.Catalog,
	nangoClient *nango.Client,
	db *gorm.DB,
	ctr *counter.Counter,
	addMemoryTools MemoryToolsFunc,
	addSubscriptionTools SubscriptionToolsFunc,
) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ziraloop",
		Version: "v1.0.0",
	}, nil)

	for _, scope := range scopes {
		// Load connection from in_connections
		var provider, providerCfgKey, nangoConnID string

		var conn model.InConnection
		if err := db.Preload("InIntegration").
			Where("id = ? AND revoked_at IS NULL", scope.ConnectionID).
			First(&conn).Error; err != nil {
			return nil, fmt.Errorf("loading connection %s: %w", scope.ConnectionID, err)
		}
		provider = conn.InIntegration.Provider
		providerCfgKey = fmt.Sprintf("in_%s", conn.InIntegration.UniqueKey)
		nangoConnID = conn.NangoConnectionID

		// Skip providers that are accessed via proxy instead of MCP.
		if providerDef, ok := cat.GetProvider(provider); ok && !providerDef.ShouldPushToMCP() {
			slog.Debug("skipping provider (push_to_mcp=false)", "provider", provider)
			continue
		}

		for _, actionKey := range scope.Actions {
			action, ok := cat.GetAction(provider, actionKey)
			if !ok {
				slog.Warn("skipping unknown action", "provider", provider, "action", actionKey)
				continue
			}

			if action.Execution == nil {
				slog.Warn("skipping action without execution config", "provider", provider, "action", actionKey)
				continue
			}

			toolName := provider + "_" + actionKey
			inputSchema := buildInputSchema(action.Parameters)

			// Capture loop variables for closure
			capturedAction := action
			capturedProvider := provider
			capturedCfgKey := providerCfgKey
			capturedConnID := nangoConnID
			capturedResources := scope.Resources
			capturedJTI := token.JTI

			server.AddTool(
				&mcp.Tool{
					Name:        toolName,
					Description: action.Description,
					InputSchema: inputSchema,
				},
				func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					// Decrement request counter before execution
					if ctr != nil {
						result, err := ctr.Decrement(ctx, counter.TokKey(capturedJTI))
						if err != nil {
							slog.Error("counter decrement failed", "error", err, "jti", capturedJTI)
						} else if result == counter.DecrExhausted {
							return &mcp.CallToolResult{
								Content: []mcp.Content{
									&mcp.TextContent{Text: "Request limit exhausted for this token"},
								},
								IsError: true,
							}, nil
						}
					}

					// Parse params from request
					var params map[string]any
					if req.Params.Arguments != nil {
						if err := json.Unmarshal(req.Params.Arguments, &params); err != nil {
							return &mcp.CallToolResult{
								Content: []mcp.Content{
									&mcp.TextContent{Text: "Invalid parameters: " + err.Error()},
								},
								IsError: true,
							}, nil
						}
					}

					// Execute the action
					result, err := ExecuteAction(
						ctx,
						nangoClient,
						capturedProvider,
						capturedCfgKey,
						capturedConnID,
						capturedAction,
						params,
						capturedResources,
					)
					if err != nil {
						return &mcp.CallToolResult{
							Content: []mcp.Content{
								&mcp.TextContent{Text: "Error: " + err.Error()},
							},
							IsError: true,
						}, nil
					}

					// Return raw JSON response as text content
					jsonBytes, err := json.Marshal(result)
					if err != nil {
						return &mcp.CallToolResult{
							Content: []mcp.Content{
								&mcp.TextContent{Text: "Failed to serialize response"},
							},
							IsError: true,
						}, nil
					}

					return &mcp.CallToolResult{
						Content: []mcp.Content{
							&mcp.TextContent{Text: string(jsonBytes)},
						},
					}, nil
				},
			)

			slog.Debug("registered MCP tool", "tool", toolName, "provider", provider, "action", actionKey)
		}
	}

	// Register memory tools if callback provided
	if addMemoryTools != nil {
		agentID, _ := token.Meta["agent_id"].(string)
		if agentID != "" {
			addMemoryTools(server, agentID, db)
		}
	}

	// Register subscription tools if callback provided
	if addSubscriptionTools != nil {
		addSubscriptionTools(server, token, db)
	}

	return server, nil
}

// buildInputSchema converts the JSON Schema from the catalog into a format
// accepted by the MCP SDK. The SDK expects an any that marshals to JSON Schema.
func buildInputSchema(params json.RawMessage) any {
	if len(params) == 0 {
		return map[string]any{"type": "object"}
	}
	var schema any
	if err := json.Unmarshal(params, &schema); err != nil {
		return map[string]any{"type": "object"}
	}
	return schema
}
