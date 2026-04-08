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
// Descriptions are copied from Bridge source: crates/tools/src/instructions/*.txt
var builtinToolMocks = []builtinToolMock{
	// ── Web ──
	{
		Name:            "web_fetch",
		Description:     "Fetches content from a specified URL. Takes a URL and optional format as input. Fetches the URL content, converts to requested format (markdown by default). Returns the content in the specified format. The URL must be a fully-formed valid URL. HTTP URLs will be automatically upgraded to HTTPS. Format options: \"markdown\" (default), \"text\", or \"html\". Uses article extraction (Mozilla Readability algorithm) when possible. Follows redirects automatically (up to 10 hops). Content is truncated at max_length characters (default 50,000). Requests time out after 30 seconds. This tool will fail for authenticated or private URLs.",
		Schema:          objSchema("url", "format", "max_length"),
		DefaultResponse: `{"content": "Mock webpage content extracted from the URL.", "title": "Mock Page Title", "url": "https://example.com"}`,
	},
	{
		Name:            "web_search",
		Description:     "Search the web for information. Returns structured results with title, URL, and snippet. Provides up-to-date information for current events and recent data. Results include knowledge graph data (when available) and organic search results. Keep queries concise and specific for best results. Use web_fetch to retrieve full content from URLs found in search results.",
		Schema:          objSchema("query", "fetch_page_content"),
		DefaultResponse: `{"results": [{"title": "Mock Search Result", "url": "https://example.com/result", "description": "A relevant search result matching the query."}]}`,
	},
	{
		Name:            "web_crawl",
		Description:     "Crawl a website starting from a URL, following links to discover and return page content. Control scope with limit (max pages), depth (max link depth), and request mode (http/chrome/smart). Set readability: true for clean content extraction. Always set a limit to avoid crawling thousands of pages accidentally. Use request: \"smart\" when unsure whether a site uses JavaScript rendering.",
		Schema:          objSchema("url", "limit", "depth", "return_format", "request", "readability"),
		DefaultResponse: `{"pages": [{"url": "https://example.com", "content": "Mock crawled page content.", "title": "Page 1"}]}`,
	},
	{
		Name:            "web_get_links",
		Description:     "Extract all links from a webpage. Returns a list of URLs found on the page. Use to discover pages on a website before deciding what to crawl, find sitemaps, documentation indexes, or navigation structure. Start with limit: 1 to see links on a single page before crawling further.",
		Schema:          objSchema("url", "limit", "request"),
		DefaultResponse: `{"links": ["https://example.com/page1", "https://example.com/page2", "https://example.com/docs"]}`,
	},
	{
		Name:            "web_screenshot",
		Description:     "Take a screenshot of a webpage. Returns a base64-encoded PNG image. Use request: \"chrome\" for accurate visual screenshots. Use wait_for_selector for SPAs that load content after the initial page load.",
		Schema:          objSchema("url", "request"),
		DefaultResponse: `{"screenshot": "base64_mock_png_data", "format": "png"}`,
	},
	{
		Name:            "web_transform",
		Description:     "Convert HTML content to markdown or plain text without making any HTTP requests. Processes HTML you already have. Include the source url when HTML contains relative links. Use \"markdown\" format for content you need to read or summarize.",
		Schema:          objSchema("data", "return_format"),
		DefaultResponse: `{"markdown": "# Converted Content\n\nMock transformed markdown from HTML."}`,
	},

	// ── Agent Orchestration ──
	{
		Name:            "agent",
		Description:     "Launch a clone of yourself to handle a focused task autonomously. The clone shares your system prompt, tools, and capabilities, but operates in its own context. Launch multiple agents concurrently whenever possible. Set background: true for long-running tasks — you will be automatically notified when they complete. Each invocation starts with a fresh context unless you provide task_id to resume a previous session.",
		Schema:          objSchema("prompt", "description", "background", "task_id"),
		DefaultResponse: `{"task_id": "mock-task-001", "status": "completed", "result": "Mock subagent completed the delegated task successfully."}`,
	},
	{
		Name:            "sub_agent",
		Description:     "Launch a subagent to handle complex, multistep tasks autonomously. Specify a subagent_name to select which subagent type to use. Launch multiple subagents concurrently whenever possible. Set background: true for long-running tasks. Each subagent invocation starts with a fresh context unless you provide task_id to resume.",
		Schema:          objSchema("subagent", "prompt", "description", "background", "task_id"),
		DefaultResponse: `{"task_id": "mock-task-002", "status": "completed", "result": "Mock subagent completed the task."}`,
	},
	{
		Name:            "parallel_agent",
		Description:     "Spawn multiple subagents in parallel and wait for all to complete. Use when you have independent tasks that can run concurrently. max_concurrent controls how many run simultaneously (default: 5, max: 25). Maximum 25 tasks per call. Each task runs in isolation with no shared context.",
		Schema:          objSchema("tasks", "max_concurrent", "timeout_secs"),
		DefaultResponse: `{"results": [{"description": "mock task", "status": "completed", "output": "Done."}], "all_succeeded": true, "total": 1, "succeeded": 1, "failed": 0}`,
	},
	{
		Name:            "batch",
		Description:     "Executes multiple independent tool calls concurrently to reduce latency. 1-25 tool calls per batch. All calls start in parallel; ordering NOT guaranteed. Partial failures do not stop other tool calls. Do NOT use batch within another batch. Good for: reading many files, grep+glob+read combos, multiple bash commands.",
		Schema:          objSchema("calls"),
		DefaultResponse: `{"results": [{"status": "ok", "output": "mock batch result"}]}`,
	},
	{
		Name:            "join",
		Description:     "Wait for multiple background subagent tasks to complete. Use after spawning background subagents with background: true. Blocks until all specified tasks complete or timeout is reached. Tasks already completed return immediately. Default timeout: 300 seconds.",
		Schema:          objSchema("task_ids", "timeout_secs"),
		DefaultResponse: `{"completed": [{"task_id": "mock-task-001", "status": "completed", "output": "Task result."}], "all_succeeded": true, "total": 1, "succeeded": 1, "failed": 0}`,
	},

	// ── Task Management ──
	{
		Name:            "todowrite",
		Description:     "Create and manage a structured task list for your current session. Uses replace-all semantics — each call sends the complete list. Track tasks as pending, in_progress, completed, or cancelled with priority levels (high/medium/low). Only have ONE task in_progress at any time. Complete current tasks before starting new ones. Use for complex multistep tasks (3+ steps).",
		Schema:          objSchema("todos"),
		DefaultResponse: `{"status": "ok", "count": 3}`,
	},
	{
		Name:            "todoread",
		Description:     "Read the current todo/task list. Returns the full list of tasks with their content, status, and priority. Returns an empty list if no todos have been written yet.",
		Schema:          objSchema(),
		DefaultResponse: `{"todos": [{"content": "Mock task item", "status": "pending", "priority": "medium"}]}`,
	},

	// ── Journal ──
	{
		Name:            "journal_write",
		Description:     "Write an entry to the conversation journal. The journal is a persistent log that survives context resets. Record key decisions, important discoveries, user preferences, architectural choices, blockers or constraints, and milestones. Keep entries concise (1-3 sentences). Use the category field to tag entries: decision, discovery, blocker, progress, preference. Write 2-5 entries per session, not every turn. Focus on the \"why\" not the \"what\".",
		Schema:          objSchema("content", "category"),
		DefaultResponse: `{"status": "ok", "entry_id": "journal-mock-001"}`,
	},
	{
		Name:            "journal_read",
		Description:     "Read the conversation journal. Returns all journal entries including agent notes and checkpoint summaries from previous context chains. Each entry includes the chain index it was written during, so you can trace the conversation's history across context resets.",
		Schema:          objSchema(),
		DefaultResponse: `{"entries": [{"content": "Mock journal entry: key decision recorded.", "category": "decision", "chain_index": 0, "timestamp": "2026-01-01T00:00:00Z"}]}`,
	},

	// ── Skills ──
	{
		Name:            "skill",
		Description:     "Execute a skill within the main conversation. When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge. When users reference a slash command (e.g., /commit, /review-pr), they are referring to a skill. Available skills are listed in system-reminder messages.",
		Schema:          objSchema("skill", "args", "file"),
		DefaultResponse: `{"status": "ok", "result": "Skill executed successfully."}`,
	},

	// ── Memory ──
	{
		Name:            "memory_recall",
		Description:     "Search your long-term memory for relevant context. Use at the START of every conversation to load relevant context before responding. Also use when the user references something from a previous conversation or before making recommendations that should account for past preferences. Input: query (natural language, 1-2 sentences), budget (low/mid/high search depth).",
		Schema:          objSchema("query", "budget"),
		DefaultResponse: `{"memories": [{"content": "Mock recalled memory: user prefers concise responses.", "relevance": 0.95}]}`,
	},
	{
		Name:            "memory_retain",
		Description:     "Store important information to long-term memory so it persists across conversations. Use when the user shares facts, preferences, decisions, deadlines, or commitments. Store decisions WITH reasoning. Do not store greetings, small talk, temporary state, or exact transcripts — distill into clear factual statements.",
		Schema:          objSchema("content", "context", "shared"),
		DefaultResponse: `{"status": "ok", "memory_id": "mem-mock-001"}`,
	},
	{
		Name:            "memory_reflect",
		Description:     "Get a synthesized, reasoned answer by deeply analyzing your full memory. Use INSTEAD of recall when analyzing patterns or trends across many past interactions, when questions require judgment or synthesis, for comprehensive summaries of everything known about a topic, or when detecting contradictions or evolving preferences.",
		Schema:          objSchema("query"),
		DefaultResponse: `{"reflection": "Based on analysis of past interactions, the pattern suggests the user values accuracy over speed and prefers detailed explanations for technical topics."}`,
	},
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
