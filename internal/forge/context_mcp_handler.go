package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// ForgeContextMCPHandler serves the start_forge tool for forge context-gathering
// conversations. When the context-gatherer agent decides it has enough information,
// it calls start_forge with structured requirements — this handler stores them
// in the ForgeRun record.
//
// Route: /forge-context/{forgeRunID}/*
type ForgeContextMCPHandler struct {
	db *gorm.DB
}

// NewForgeContextMCPHandler creates a new forge context MCP handler.
func NewForgeContextMCPHandler(db *gorm.DB) *ForgeContextMCPHandler {
	return &ForgeContextMCPHandler{db: db}
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *ForgeContextMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

// serverFactory creates an MCP server with the start_forge tool if the forge
// run is in gathering_context status.
func (h *ForgeContextMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		slog.Error("forge context mcp: no forgeRunID in URL")
		return emptyContextServer()
	}

	var run model.ForgeRun
	if err := h.db.Select("id, status").Where("id = ?", runID).First(&run).Error; err != nil {
		slog.Error("forge context mcp: forge run not found", "forge_run_id", runID, "error", err)
		return emptyContextServer()
	}

	if run.Status != model.ForgeStatusGatheringContext {
		slog.Warn("forge context mcp: run not in gathering_context status", "forge_run_id", runID, "status", run.Status)
		return emptyContextServer()
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-context",
		Version: "v1.0.0",
	}, nil)

	server.AddTool(
		&mcpsdk.Tool{
			Name:        "start_forge",
			Description: "Submit the gathered requirements and start the Forge optimization process. The user will be asked to approve before Forge begins.",
			InputSchema: StartForgeToolSchema(),
		},
		h.buildStartForgeHandler(run.ID.String()),
	)

	return server
}

// buildStartForgeHandler creates the tool handler for start_forge.
// It validates the arguments, stores them as context on the ForgeRun,
// and returns a confirmation message.
func (h *ForgeContextMCPHandler) buildStartForgeHandler(runID string) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args ForgeContext
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcpsdk.CallToolResult{
					Content: []mcpsdk.Content{
						&mcpsdk.TextContent{Text: fmt.Sprintf(`{"error": "invalid arguments: %s"}`, err.Error())},
					},
					IsError: true,
				}, nil
			}
		}

		if args.RequirementsSummary == "" {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: `{"error": "requirements_summary is required"}`},
				},
				IsError: true,
			}, nil
		}
		if len(args.SuccessCriteria) == 0 {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: `{"error": "success_criteria must contain at least one criterion"}`},
				},
				IsError: true,
			}, nil
		}

		contextJSON, err := json.Marshal(args)
		if err != nil {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: `{"error": "failed to serialize context"}`},
				},
				IsError: true,
			}, nil
		}

		if err := h.db.Model(&model.ForgeRun{}).
			Where("id = ? AND status = ?", runID, model.ForgeStatusGatheringContext).
			Update("context", contextJSON).Error; err != nil {
			slog.Error("forge context mcp: failed to store context", "forge_run_id", runID, "error", err)
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: `{"error": "failed to save requirements"}`},
				},
				IsError: true,
			}, nil
		}

		slog.Info("forge context mcp: requirements captured",
			"forge_run_id", runID,
			"criteria_count", len(args.SuccessCriteria),
		)

		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: `{"status": "requirements_captured", "message": "Requirements have been recorded. Waiting for user approval to start Forge."}`},
			},
		}, nil
	}
}

// emptyContextServer returns an MCP server with no tools.
func emptyContextServer() *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-context",
		Version: "v1.0.0",
	}, nil)
}
