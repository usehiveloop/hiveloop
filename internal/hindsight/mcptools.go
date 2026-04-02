package hindsight

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/llmvault/llmvault/internal/model"
)

// BuildMemoryServer creates an MCP server with memory tools (recall, retain, reflect)
// scoped to a specific agent's identity bank.
func BuildMemoryServer(agent *model.Agent, identity *model.Identity, client *Client) *mcp.Server {
	bankID := "identity-" + identity.ID.String()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "llmvault-memory",
		Version: "v1.0.0",
	}, nil)

	// Tag group filter: agent sees own team + shared memories
	tagGroups := buildTagGroups(agent.Team)

	// --- memory_recall ---
	server.AddTool(
		&mcp.Tool{
			Name:        "memory_recall",
			Description: "Search your memory for relevant information. Returns facts, entities, and observations from past conversations and stored knowledge.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Natural language search query describing what you want to remember",
					},
					"budget": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "mid", "high"},
						"description": "Search depth: low=quick lookup, mid=balanced (default), high=thorough analysis",
					},
				},
				"required": []string{"query"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Query  string `json:"query"`
				Budget string `json:"budget"`
			}
			if req.Params.Arguments != nil {
				json.Unmarshal(req.Params.Arguments, &params)
			}
			if params.Query == "" {
				return toolError("query is required"), nil
			}
			budget := params.Budget
			if budget == "" {
				budget = "mid"
			}

			result, err := client.Recall(ctx, bankID, &RecallRequest{
				Query:     params.Query,
				Budget:    budget,
				TagGroups: tagGroups,
			})
			if err != nil {
				return toolError("memory recall failed: " + err.Error()), nil
			}

			return toolJSON(result)
		},
	)

	// --- memory_retain ---
	server.AddTool(
		&mcp.Tool{
			Name:        "memory_retain",
			Description: "Store important information in memory for future recall. Use this to remember facts, decisions, preferences, or anything worth keeping.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "The information to remember",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Optional context about where this information came from or why it matters",
					},
					"shared": map[string]any{
						"type":        "boolean",
						"description": "If true, this memory will be visible to all agents (requires shared memory permission)",
					},
				},
				"required": []string{"content"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Content string `json:"content"`
				Context string `json:"context"`
				Shared  bool   `json:"shared"`
			}
			if req.Params.Arguments != nil {
				json.Unmarshal(req.Params.Arguments, &params)
			}
			if params.Content == "" {
				return toolError("content is required"), nil
			}

			// Build tags based on agent permissions
			tags := []string{"team:" + agent.Team, "agent:" + agent.ID.String()}
			if params.Shared {
				if !agent.SharedMemory {
					return toolError("this agent does not have permission to store shared memories"), nil
				}
				tags = append(tags, "shared")
			}

			result, err := client.Retain(ctx, bankID, &RetainRequest{
				Items: []RetainItem{{
					Content: params.Content,
					Context: params.Context,
					Tags:    tags,
				}},
				Async: true,
			})
			if err != nil {
				return toolError("memory retain failed: " + err.Error()), nil
			}

			return toolJSON(result)
		},
	)

	// --- memory_reflect ---
	server.AddTool(
		&mcp.Tool{
			Name:        "memory_reflect",
			Description: "Get a synthesized, reasoned answer from memory. Unlike recall which returns raw facts, reflect analyzes memories and provides a thoughtful response.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The question to reason about using available memories",
					},
				},
				"required": []string{"query"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Query string `json:"query"`
			}
			if req.Params.Arguments != nil {
				json.Unmarshal(req.Params.Arguments, &params)
			}
			if params.Query == "" {
				return toolError("query is required"), nil
			}

			result, err := client.Reflect(ctx, bankID, &ReflectRequest{
				Query:     params.Query,
				TagGroups: tagGroups,
			})
			if err != nil {
				return toolError("memory reflect failed: " + err.Error()), nil
			}

			return toolJSON(result)
		},
	)

	return server
}

// buildTagGroups creates the tag filter for recall/reflect.
// Agent sees: own team memories + shared memories.
func buildTagGroups(team string) []any {
	return []any{
		map[string]any{
			"or": []any{
				map[string]any{"tags": []string{"team:" + team}, "match": "all_strict"},
				map[string]any{"tags": []string{"shared"}, "match": "all_strict"},
			},
		},
	}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %s", msg)}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
	}, nil
}
