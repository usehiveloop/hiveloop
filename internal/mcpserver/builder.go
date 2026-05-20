package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/counter"
	"github.com/usehivy/hivy/internal/logging"
	mcppkg "github.com/usehivy/hivy/internal/mcp"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

// MemoryToolsFunc is a callback that registers memory tools on a server.
// Used to avoid an import cycle between mcpserver and hindsight.
type MemoryToolsFunc func(server *mcp.Server, agentID string, db *gorm.DB)

// WebToolsFunc is a callback that registers web_fetch and web_search on a
// server. Used to avoid an import cycle between mcpserver and spider.
type WebToolsFunc func(server *mcp.Server, token *model.Token)

// KnowledgeToolsFunc registers org-scoped knowledge-base search tools.
type KnowledgeToolsFunc func(server *mcp.Server, token *model.Token)

// BuildServer creates an MCP server with tools registered from token scopes.
// Each scope's connection+actions are turned into MCP tools via the catalog.
// If addMemoryTools is non-nil, it is called to register memory tools on the
// same server after integration tools are registered.
// If addWebTools is non-nil, it is called to register web_fetch and web_search
// on the same server after memory tools are registered.
func BuildServer(
	ctx context.Context,
	token *model.Token,
	scopes []mcppkg.TokenScope,
	cat *catalog.Catalog,
	nangoClient *nango.Client,
	db *gorm.DB,
	ctr *counter.Counter,
	addMemoryTools MemoryToolsFunc,
	addWebTools WebToolsFunc,
	addKnowledgeTools KnowledgeToolsFunc,
) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "hivy",
		Version: "v1.0.0",
	}, nil)

	for _, scope := range scopes {

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

		if providerDef, ok := cat.GetProvider(provider); ok && !providerDef.ShouldPushToMCP() {
			continue
		}

		for _, actionKey := range scope.Actions {
			action, ok := cat.GetAction(provider, actionKey)
			if !ok {
				logging.FromContext(ctx).WarnContext(ctx, "skipping unknown action", "provider", provider, "action", actionKey)
				continue
			}

			if action.Execution == nil {
				logging.FromContext(ctx).WarnContext(ctx, "skipping action without execution config", "provider", provider, "action", actionKey)
				continue
			}

			toolName := provider + "_" + actionKey
			inputSchema := buildInputSchema(action.Parameters)

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

					if ctr != nil {
						result, err := ctr.Decrement(ctx, counter.TokKey(capturedJTI))
						if err != nil {
							logging.FromContext(ctx).ErrorContext(ctx, "counter decrement failed", "error", err, "jti", capturedJTI)
						} else if result == counter.DecrExhausted {
							return &mcp.CallToolResult{
								Content: []mcp.Content{
									&mcp.TextContent{Text: "Request limit exhausted for this token"},
								},
								IsError: true,
							}, nil
						}
					}

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
		}
	}

	if addMemoryTools != nil {
		agentID, _ := token.Meta["agent_id"].(string)
		if agentID != "" {
			addMemoryTools(server, agentID, db)
		}
	}

	if addWebTools != nil {
		addWebTools(server, token)
	}

	if addKnowledgeTools != nil {
		addKnowledgeTools(server, token)
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
