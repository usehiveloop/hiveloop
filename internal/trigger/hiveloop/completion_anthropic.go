package hiveloop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicCompletionClient implements CompletionClient for Anthropic's
// Claude models. Translates between the provider-agnostic types and
// Anthropic's tool_use format.
type AnthropicCompletionClient struct {
	client anthropic.Client
}

// NewAnthropicCompletionClient creates a client using the Anthropic SDK.
func NewAnthropicCompletionClient(apiKey string) *AnthropicCompletionClient {
	return &AnthropicCompletionClient{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

func (c *AnthropicCompletionClient) ChatCompletion(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	// Extract system message (Anthropic handles it separately).
	var systemPrompt string
	var messages []anthropic.MessageParam
	for _, message := range req.Messages {
		switch message.Role {
		case "system":
			systemPrompt = message.Content
		case "user":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewTextBlock(message.Content),
			))
		case "assistant":
			if len(message.ToolCalls) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				for _, toolCall := range message.ToolCalls {
					blocks = append(blocks, anthropic.NewToolUseBlock(
						toolCall.ID, json.RawMessage(toolCall.Arguments), toolCall.Name,
					))
				}
				messages = append(messages, anthropic.NewAssistantMessage(blocks...))
			} else {
				messages = append(messages, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(message.Content),
				))
			}
		case "tool":
			messages = append(messages, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(message.ToolCallID, message.Content, false),
			))
		}
	}

	// Build tools.
	var tools []anthropic.ToolUnionParam
	for _, tool := range req.Tools {
		var schema anthropic.ToolInputSchemaParam
		json.Unmarshal(tool.Parameters, &schema)
		tools = append(tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: schema,
			},
		})
	}

	maxTokens := int64(4096)
	if req.MaxTokens > 0 {
		maxTokens = int64(req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  messages,
		Tools:     tools,
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: systemPrompt},
		}
	}
	// Force tool use: Anthropic's equivalent of ToolChoice: "required".
	if req.ToolChoice == "required" {
		params.ToolChoice = anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{
				Type: "any",
			},
		}
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic completion: %w", err)
	}

	result := &CompletionResponse{
		Message: Message{Role: "assistant"},
	}
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			result.Message.Content += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			result.Message.ToolCalls = append(result.Message.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(argsJSON),
			})
		}
	}

	return result, nil
}
