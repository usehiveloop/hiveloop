package hindsight

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addRecallTool(server *mcp.Server, client *Client, bankID string, tagGroups []any) {
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
				_ = json.Unmarshal(req.Params.Arguments, &params)
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
}
