package hindsight

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addReflectTool(server *mcp.Server, client *Client, bankID string, tagGroups []any) {
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
				_ = json.Unmarshal(req.Params.Arguments, &params)
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
}
