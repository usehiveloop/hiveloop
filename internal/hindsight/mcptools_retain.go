package hindsight

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

type memoryRetainToolResponse struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	BankID      string `json:"bank_id"`
	ItemsCount  int    `json:"items_count"`
	Async       bool   `json:"async"`
	OperationID string `json:"operation_id,omitempty"`
	DocumentID  string `json:"document_id"`
}

func addRetainTool(server *mcp.Server, agent *model.Employee, client *Client, bankID string, memoryTags []string) {
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
					"memory_type": map[string]any{
						"type":        "string",
						"enum":        SupportedMemoryTypes,
						"description": "Durable memory category. Use the closest category for the fact being retained.",
					},
				},
				"required": []string{"content"},
			},
		},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var params struct {
				Content string `json:"content"`
				Context string `json:"context"`
				Type    string `json:"memory_type"`
			}
			if req.Params.Arguments != nil {
				_ = json.Unmarshal(req.Params.Arguments, &params)
			}
			if params.Content == "" {
				return toolError("content is required"), nil
			}
			if params.Type != "" && !IsSupportedMemoryType(params.Type) {
				return toolError("unsupported memory_type: " + params.Type), nil
			}

			tags := append([]string{}, memoryTags...)
			if params.Type != "" {
				tags = append(tags, "memory_type:"+params.Type)
			}
			documentID := "manual:" + agent.ID.String() + ":" + uuid.NewString()
			result, err := client.Retain(ctx, bankID, &RetainRequest{
				Items: []RetainItem{{
					Content:    params.Content,
					Context:    params.Context,
					DocumentID: documentID,
					Tags:       tags,
					Metadata:   map[string]string{"employee_id": agent.ID.String(), "document_id": documentID},
				}},
				Async: true,
			})
			if err != nil {
				return toolError("memory retain failed: " + err.Error()), nil
			}

			return toolJSON(memoryRetainResponse(bankID, documentID, result))
		},
	)
}

func memoryRetainResponse(bankID, documentID string, result *RetainResponse) memoryRetainToolResponse {
	out := memoryRetainToolResponse{
		Message:    "Memory retain has been accepted and will be processed in the background. It may take a little while before memory_recall reflects this new memory.",
		BankID:     bankID,
		DocumentID: documentID,
	}
	if result == nil {
		return out
	}
	out.Success = result.Success
	out.ItemsCount = result.ItemsCount
	out.Async = result.Async
	out.OperationID = result.OperationID
	if result.BankID != "" {
		out.BankID = result.BankID
	}
	return out
}
