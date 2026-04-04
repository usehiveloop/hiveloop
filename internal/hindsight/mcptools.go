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
	var bankID string
	if identity != nil {
		bankID = "identity-" + identity.ID.String()
	} else {
		bankID = "agent-" + agent.ID.String()
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "llmvault-memory",
		Version: "v1.0.0",
	}, nil)

	// Tag group filter: agent sees own team + shared memories
	tagGroups := buildTagGroups(agent.Team)

	// --- memory_recall ---
	server.AddTool(
		&mcp.Tool{
			Name: "memory_recall",
			Description: `Search your long-term memory for relevant context. Call this tool:
- At the START of every conversation to load relevant context before responding
- When the user references something from a previous conversation ("last time", "as we discussed", "remember when")
- When you need to check if you already know something before asking the user
- Before making a recommendation that should account for past preferences, decisions, or history
- When the user asks about a person, project, or topic you may have encountered before

Returns specific facts, entities, and consolidated observations from past interactions.
Write a short, focused query (1-2 sentences) describing what you need to know.
Do NOT recall and retain in the same turn — retained memories are not immediately available.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "A focused natural language query describing what you want to remember. Examples: 'What are this user's communication preferences?', 'What decisions were made about the billing system?', 'What do we know about Project Atlas?'",
					},
					"budget": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "mid", "high"},
						"description": "Search depth. Use 'low' for quick fact checks (50-100ms). Use 'mid' (default) for most queries — balances speed and thoroughness. Use 'high' only for complex questions requiring deep cross-referencing across many memories (300-500ms).",
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
	retainSharedDesc := "If true, this memory will be visible to ALL agents in the organization, not just your team. Only use for information that is broadly relevant (company announcements, cross-team decisions, org-wide policies). Requires shared memory permission."
	if !agent.SharedMemory {
		retainSharedDesc = "This agent does not have shared memory permission. Always omit this field or set to false."
	}

	server.AddTool(
		&mcp.Tool{
			Name: "memory_retain",
			Description: `Store important information to long-term memory so it persists across conversations. Call this tool when:
- The user shares a fact, preference, decision, deadline, or commitment you should remember
- A significant decision is made or a problem is resolved — store the decision AND the reasoning
- You learn something new about the user, their projects, their team, or their goals
- The user corrects you or expresses a preference about how you should work — store the correction so you never repeat the mistake
- Important relationships between people, projects, or concepts are revealed
- A task outcome, milestone, or status change occurs that future conversations should know about

DO NOT store:
- Greetings, small talk, or conversational filler
- Information you have already stored (avoid duplicates)
- Temporary state or in-progress work details that will change immediately
- Exact conversation transcripts — distill into clear factual statements instead
- Anything the user explicitly asks you not to remember

Write the content as a clear, specific factual statement. Bad: "User talked about React." Good: "User's frontend stack is React with Zustand for state management, migrated from Redux in Q1 2026."`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "A clear, factual statement of what to remember. Write as a specific fact, not a conversation excerpt. Include names, dates, and specifics when available.",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Describe the nature and source of this information. This significantly improves how the memory is indexed and retrieved. Examples: 'Technical architecture discussion', 'User preference stated during onboarding', 'Decision from Q2 planning meeting'. Do NOT use generic values like 'conversation' or 'chat'.",
					},
					"shared": map[string]any{
						"type":        "boolean",
						"description": retainSharedDesc,
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
				if identity == nil {
					return toolError("shared memory requires an identity"), nil
				}
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
			Name: "memory_reflect",
			Description: `Get a synthesized, reasoned answer by deeply analyzing your full memory. Use this INSTEAD of recall when:
- You need to analyze patterns or trends across many past interactions ("How has the user's opinion on X changed over time?")
- The question requires judgment or synthesis, not just fact retrieval ("What should I prioritize based on what I know?")
- You want a comprehensive summary of everything known about a topic ("What is the full picture of Project Atlas?")
- You need to detect contradictions or evolving preferences across different conversations
- The user asks "what do you think?" or "what would you recommend?" based on history

Use recall instead when you need specific facts, quick lookups, or raw citations.
Reflect is slower than recall (1-3 seconds) but produces deeper, more nuanced answers that consider the full breadth of stored knowledge.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The question to reason about. Frame as a question that requires analysis, not just lookup. Examples: 'What are this user's top priorities based on our past interactions?', 'How has the team's approach to testing evolved?', 'What patterns do I see in the problems this user brings to me?'",
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
