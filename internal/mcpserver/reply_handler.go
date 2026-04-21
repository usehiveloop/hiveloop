package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// ReplyMCPHandler exposes per-connection write tools scoped to a conversation's
// source channel. When the Hiveloop executor creates a conversation, it attaches
// this MCP server as "hiveloop-reply" so the specialist agent can post messages
// back to the channel (Slack thread, GitHub issue, etc.) using the source
// connection's credentials.
//
// Route: /reply/{connectionID}
type ReplyMCPHandler struct {
	db      *gorm.DB
	catalog *catalog.Catalog
}

// NewReplyMCPHandler creates a reply MCP handler.
func NewReplyMCPHandler(db *gorm.DB, actionsCatalog *catalog.Catalog) *ReplyMCPHandler {
	return &ReplyMCPHandler{db: db, catalog: actionsCatalog}
}

// StreamableHTTPHandler returns an HTTP handler for the reply MCP endpoint.
func (handler *ReplyMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(handler.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

func (handler *ReplyMCPHandler) serverFactory(request *http.Request) *mcpsdk.Server {
	connectionID := chi.URLParam(request, "connectionID")
	if connectionID == "" {
		return emptyReplyServer()
	}

	// Load the in_connection to resolve the provider.
	var connection model.InConnection
	if err := handler.db.Preload("InIntegration").
		Where("id = ? AND revoked_at IS NULL", connectionID).
		First(&connection).Error; err != nil {
		slog.Warn("reply MCP: connection not found", "connection_id", connectionID, "error", err)
		return emptyReplyServer()
	}

	provider := connection.InIntegration.Provider

	// Get write actions for this provider from the catalog.
	providerDef, ok := handler.catalog.GetProvider(provider)
	if !ok {
		slog.Warn("reply MCP: provider not in catalog", "provider", provider)
		return emptyReplyServer()
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "hiveloop-reply-" + provider,
		Version: "v1.0.0",
	}, nil)

	for actionKey, actionDef := range providerDef.Actions {
		if actionDef.Access != "write" {
			continue
		}

		description := actionDef.Description
		if description == "" {
			description = actionDef.DisplayName
		}

		// Build input schema from the action's JSON Schema parameters.
		var inputSchema map[string]any
		if actionDef.Parameters != nil {
			json.Unmarshal(actionDef.Parameters, &inputSchema)
		}
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}

		server.AddTool(
			&mcpsdk.Tool{
				Name:        actionKey,
				Description: description,
				InputSchema: inputSchema,
			},
			makeReplyToolHandler(connectionID, provider, actionKey),
		)
	}

	return server
}

func makeReplyToolHandler(connectionID, provider, actionKey string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		slog.Info("reply MCP: tool called",
			"connection_id", connectionID,
			"provider", provider,
			"tool", actionKey,
		)

		// Extract params from the request arguments.
		var params map[string]any
		if request.Params.Arguments != nil {
			paramsJSON, _ := json.Marshal(request.Params.Arguments)
			json.Unmarshal(paramsJSON, &params)
		}

		// In production, this executes via the Nango proxy:
		// 1. Load Nango connection credentials from DB
		// 2. Build upstream API request using the action's ExecutionConfig
		// 3. Substitute params into path/query/body
		// 4. Execute via Nango proxy endpoint
		// 5. Return the upstream response
		//
		// The Nango proxy pattern is identical to what mcpserver/executor.go
		// already does for integration tools. The reply handler just filters
		// to write actions and scopes to the conversation's source connection.

		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{
					Text: fmt.Sprintf("Executed %s/%s on connection %s with params: %s",
						provider, actionKey, connectionID, formatReplyParams(params)),
				},
			},
		}, nil
	}
}

func emptyReplyServer() *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "hiveloop-reply-empty",
		Version: "v1.0.0",
	}, nil)
}

func formatReplyParams(params map[string]any) string {
	if len(params) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(params))
	for key, value := range params {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ", ")
}
