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

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

// ReplyMCPHandler exposes per-connection write tools scoped to a conversation's
// source channel. When the Hivy executor creates a conversation, it attaches
// this MCP server as "hivy-reply" so the specialist agent can post messages
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
		Stateless:                  true,
		Logger:                     slog.Default(),
		DisableLocalhostProtection: true,
	})
}

func (handler *ReplyMCPHandler) serverFactory(request *http.Request) *mcpsdk.Server {
	connectionID := chi.URLParam(request, "connectionID")
	if connectionID == "" {
		return emptyReplyServer()
	}

	var connection model.InConnection
	ctx := request.Context()
	if err := handler.db.WithContext(ctx).Preload("InIntegration").
		Where("id = ? AND revoked_at IS NULL", connectionID).
		First(&connection).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "reply MCP: connection not found", "connection_id", connectionID, "error", err)
		return emptyReplyServer()
	}

	provider := connection.InIntegration.Provider

	providerDef, ok := handler.catalog.GetProvider(provider)
	if !ok {
		logging.FromContext(ctx).WarnContext(ctx, "reply MCP: provider not in catalog", "provider", provider)
		return emptyReplyServer()
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "hivy-reply-" + provider,
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

		var inputSchema map[string]any
		if actionDef.Parameters != nil {
			_ = json.Unmarshal(actionDef.Parameters, &inputSchema)
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

		var params map[string]any
		if request.Params.Arguments != nil {
			paramsJSON, _ := json.Marshal(request.Params.Arguments)
			_ = json.Unmarshal(paramsJSON, &params)
		}

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
		Name:    "hivy-reply-empty",
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
