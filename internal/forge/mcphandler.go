package forge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
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

	// Register integration tools (from architect output) with eval-case mocks.
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

	// Register all Bridge built-in tools as mocks so the eval-target agent
	// can call them without side effects (no real web requests, no real
	// memory writes, no real shell commands).
	for _, builtinTool := range builtinToolMocks {
		// Skip if an integration tool already has this name.
		alreadyRegistered := false
		for _, tool := range tools {
			if tool.Name == builtinTool.Name {
				alreadyRegistered = true
				break
			}
		}
		if alreadyRegistered {
			continue
		}

		server.AddTool(
			&mcpsdk.Tool{
				Name:        builtinTool.Name,
				Description: builtinTool.Description,
				InputSchema: builtinTool.Schema,
			},
			buildBuiltinMockHandler(runID, builtinTool.Name, builtinTool.DefaultResponse, toolMocks),
		)
	}

	return server
}

// builtinToolMock defines a Bridge built-in tool with its default mock response.
type builtinToolMock struct {
	Name            string
	Description     string
	Schema          map[string]any
	DefaultResponse string
}

// builtinToolMocks lists all Bridge built-in tools that get mocked during forge evals.
// The eval-target agent has all built-in tools disabled — these MCP mocks replace them.
var builtinToolMocks = []builtinToolMock{
	// ── Web ──
	{Name: "web_fetch", Description: "Fetch a URL and extract readable content.", Schema: objSchema("url"), DefaultResponse: `{"content": "Mock webpage content.", "title": "Mock Page", "url": "https://example.com"}`},
	{Name: "web_search", Description: "Search the web.", Schema: objSchema("query"), DefaultResponse: `{"results": [{"title": "Mock Result", "url": "https://example.com", "description": "A mock search result."}]}`},
	{Name: "web_crawl", Description: "Crawl a website following links.", Schema: objSchema("url"), DefaultResponse: `{"pages": [{"url": "https://example.com", "content": "Mock crawled content."}]}`},
	{Name: "web_get_links", Description: "Extract all links from a webpage.", Schema: objSchema("url"), DefaultResponse: `{"links": ["https://example.com/page1", "https://example.com/page2"]}`},
	{Name: "web_screenshot", Description: "Take a screenshot of a webpage.", Schema: objSchema("url"), DefaultResponse: `{"screenshot": "base64_mock_data", "format": "png"}`},
	{Name: "web_transform", Description: "Convert HTML to markdown.", Schema: objSchema("html"), DefaultResponse: `{"markdown": "# Mock\n\nConverted content."}`},

	// ── Agent Orchestration ──
	{Name: "agent", Description: "Launch a subagent for a focused task.", Schema: objSchema("prompt"), DefaultResponse: `{"task_id": "mock-task-001", "status": "completed", "result": "Mock subagent completed the task."}`},
	{Name: "sub_agent", Description: "Launch a named subagent.", Schema: objSchema("subagent", "prompt"), DefaultResponse: `{"task_id": "mock-task-002", "status": "completed", "result": "Mock subagent completed."}`},
	{Name: "parallel_agent", Description: "Run multiple subagents in parallel.", Schema: objSchema("tasks"), DefaultResponse: `{"results": [{"task_id": "mock-001", "status": "completed", "result": "Done."}]}`},
	{Name: "batch", Description: "Execute multiple tools concurrently.", Schema: objSchema("calls"), DefaultResponse: `{"results": [{"status": "ok"}]}`},
	{Name: "join", Description: "Wait for background tasks to complete.", Schema: objSchema("task_ids"), DefaultResponse: `{"results": [{"task_id": "mock-task-001", "status": "completed", "result": "Done."}]}`},

	// ── Task Management ──
	{Name: "todowrite", Description: "Create or update the task list.", Schema: objSchema("todos"), DefaultResponse: `{"status": "ok", "count": 3}`},
	{Name: "todoread", Description: "Read the current task list.", Schema: objSchema(), DefaultResponse: `{"todos": [{"content": "Mock task", "status": "pending", "priority": "medium"}]}`},

	// ── Journal ──
	{Name: "journal_write", Description: "Write a journal entry.", Schema: objSchema("content"), DefaultResponse: `{"status": "ok", "entry_id": "journal-mock-001"}`},
	{Name: "journal_read", Description: "Read all journal entries.", Schema: objSchema(), DefaultResponse: `{"entries": [{"content": "Mock journal entry.", "category": "progress", "timestamp": "2026-01-01T00:00:00Z"}]}`},

	// ── Skills ──
	{Name: "skill", Description: "Invoke a reusable skill.", Schema: objSchema("name"), DefaultResponse: `{"status": "ok", "result": "Skill executed."}`},

	// ── Memory ──
	{Name: "memory_recall", Description: "Search long-term memory for relevant context.", Schema: objSchema("query"), DefaultResponse: `{"memories": [{"content": "Mock recalled memory.", "relevance": 0.95}]}`},
	{Name: "memory_retain", Description: "Store information to long-term memory.", Schema: objSchema("content"), DefaultResponse: `{"status": "ok", "memory_id": "mem-mock-001"}`},
	{Name: "memory_reflect", Description: "Analyze memory for patterns and synthesis.", Schema: objSchema("query"), DefaultResponse: `{"reflection": "Based on past interactions, the pattern suggests..."}`},
}

// objSchema returns a minimal JSON Schema object with the given property names.
func objSchema(properties ...string) map[string]any {
	props := map[string]any{}
	for _, prop := range properties {
		props[prop] = map[string]any{"type": "string"}
	}
	return map[string]any{
		"type":       "object",
		"properties": props,
	}
}

// buildBuiltinMockHandler returns a mock handler for a built-in tool.
// If the eval case has a custom mock for this tool name, use that instead of the default.
func buildBuiltinMockHandler(runID, toolName, defaultResponse string, toolMocks map[string][]MockSample) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		// Check if the eval case has a custom mock for this built-in tool.
		samples := toolMocks[toolName]
		if len(samples) > 0 {
			var args map[string]any
			if req.Params.Arguments != nil {
				json.Unmarshal(req.Params.Arguments, &args)
			}
			sample := findBestMock(args, samples)
			respBytes, _ := json.Marshal(sample.Response)

			slog.Debug("forge mcp: returning custom mock for builtin tool",
				"forge_run_id", runID,
				"tool", toolName,
			)

			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: string(respBytes)},
				},
			}, nil
		}

		// Return default mock response.
		slog.Debug("forge mcp: returning default mock for builtin tool",
			"forge_run_id", runID,
			"tool", toolName,
		)

		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: defaultResponse},
			},
		}, nil
	}
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
