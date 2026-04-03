package forge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
)

// ForgeMCPHandler serves mock tools for forge eval execution.
// When Bridge's eval-target agent calls a tool, it hits this MCP server,
// which queries the database to find the currently running eval and returns
// the corresponding mock response.
//
// The query chain: ForgeRun (by URL param) → current ForgeIteration (by
// current_iteration) → ForgeEvalResult with status='running' → ForgeEvalCase → ToolMocks.
//
// Route: /forge/{forgeRunID}/*
type ForgeMCPHandler struct {
	db *gorm.DB
}

// NewForgeMCPHandler creates a new forge MCP handler.
func NewForgeMCPHandler(db *gorm.DB) *ForgeMCPHandler {
	return &ForgeMCPHandler{db: db}
}

// StreamableHTTPHandler returns an HTTP handler for the MCP Streamable HTTP transport.
func (h *ForgeMCPHandler) StreamableHTTPHandler() http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(h.serverFactory, &mcpsdk.StreamableHTTPOptions{
		Stateless: true,
		Logger:    slog.Default(),
	})
}

// serverFactory creates an MCP server by querying the DB for the active eval's mocks.
//
// Query chain:
//  1. ForgeRun (by forgeRunID from URL) → get current_iteration
//  2. ForgeIteration (by forge_run_id + iteration number) → get tools (architect output)
//  3. ForgeEvalResult (by forge_iteration_id + status='running') → ForgeEvalCase → get tool_mocks
func (h *ForgeMCPHandler) serverFactory(r *http.Request) *mcpsdk.Server {
	runID := chi.URLParam(r, "forgeRunID")
	if runID == "" {
		slog.Error("forge mcp: no forgeRunID in URL")
		return nil
	}

	// 1. Load forge run → current iteration number.
	var run model.ForgeRun
	if err := h.db.Select("id, current_iteration").Where("id = ?", runID).First(&run).Error; err != nil {
		slog.Error("forge mcp: forge run not found", "forge_run_id", runID, "error", err)
		return emptyServer()
	}

	if run.CurrentIteration == 0 {
		slog.Warn("forge mcp: no active iteration", "forge_run_id", runID)
		return emptyServer()
	}

	// 2. Load current iteration → tool definitions from architect output.
	var iter model.ForgeIteration
	if err := h.db.Select("id, tools").
		Where("forge_run_id = ? AND iteration = ?", run.ID, run.CurrentIteration).
		First(&iter).Error; err != nil {
		slog.Error("forge mcp: iteration not found", "forge_run_id", runID, "iteration", run.CurrentIteration, "error", err)
		return emptyServer()
	}

	// 3. Load the currently running eval result → its eval case → tool mocks.
	var evalResult model.ForgeEvalResult
	if err := h.db.Select("id, forge_eval_case_id").
		Where("forge_iteration_id = ? AND status = ?", iter.ID, model.ForgeEvalRunning).
		First(&evalResult).Error; err != nil {
		slog.Warn("forge mcp: no running eval found", "forge_run_id", runID, "iteration_id", iter.ID)
		return emptyServer()
	}

	var evalCase model.ForgeEvalCase
	if err := h.db.Select("id, tool_mocks").
		Where("id = ?", evalResult.ForgeEvalCaseID).
		First(&evalCase).Error; err != nil {
		slog.Warn("forge mcp: eval case not found", "forge_run_id", runID, "eval_case_id", evalResult.ForgeEvalCaseID)
		return emptyServer()
	}

	// Parse tool definitions and mocks.
	var tools []ToolDefinition
	if len(iter.Tools) > 0 {
		json.Unmarshal(iter.Tools, &tools)
	}

	var toolMocks map[string][]MockSample
	if len(evalCase.ToolMocks) > 0 {
		json.Unmarshal(evalCase.ToolMocks, &toolMocks)
	}

	// Build MCP server with mock tool handlers.
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-mock",
		Version: "v1.0.0",
	}, nil)

	for _, tool := range tools {
		inputSchema := map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
		if tool.Parameters != nil {
			inputSchema = tool.Parameters
		}

		server.AddTool(
			&mcpsdk.Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: inputSchema,
			},
			buildMockHandler(runID, tool.Name, toolMocks),
		)
	}

	return server
}

// buildMockHandler creates a tool call handler that returns the best matching
// mock response for the given tool.
func buildMockHandler(runID, toolName string, toolMocks map[string][]MockSample) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		var args map[string]any
		if req.Params.Arguments != nil {
			json.Unmarshal(req.Params.Arguments, &args)
		}

		samples := toolMocks[toolName]
		if len(samples) == 0 {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: `{"error": "no mock configured for this tool"}`},
				},
				IsError: true,
			}, nil
		}

		sample := findBestMock(args, samples)
		respBytes, _ := json.Marshal(sample.Response)

		slog.Debug("forge mcp: returning mock response",
			"forge_run_id", runID,
			"tool", toolName,
			"args", args,
		)

		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: string(respBytes)},
			},
		}, nil
	}
}

// emptyServer returns an MCP server with no tools.
func emptyServer() *mcpsdk.Server {
	return mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "forge-mock",
		Version: "v1.0.0",
	}, nil)
}
