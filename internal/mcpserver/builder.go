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

// BuildServer creates an MCP server with tools registered from token scopes.
// Each scope's connection+actions are turned into MCP tools via the catalog.
func BuildServer(
	token *model.Token,
	scopes []mcppkg.TokenScope,
	cat *catalog.Catalog,
	nangoClient *nango.Client,
	db *gorm.DB,
	ctr *counter.Counter,
) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ziraloop",
		Version: "v1.0.0",
	}, nil)

	for _, scope := range scopes {
		// Load connection + integration from DB
		var conn model.Connection
		if err := db.Preload("Integration").
			Where("id = ? AND revoked_at IS NULL", scope.ConnectionID).
			First(&conn).Error; err != nil {
			return nil, fmt.Errorf("loading connection %s: %w", scope.ConnectionID, err)
		}

		provider := conn.Integration.Provider
		providerCfgKey := fmt.Sprintf("%s_%s", token.OrgID.String(), conn.Integration.UniqueKey)
		nangoConnID := conn.NangoConnectionID

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
