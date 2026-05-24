package specialisttasks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func NewToolsFunc(service *Service) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if token == nil || token.Meta == nil {
			return
		}
		if tokenType, _ := token.Meta["type"].(string); tokenType != "employee_proxy" {
			return
		}
		registerLaunchTool(server, service, token)
		registerStatusTool(server, service, token)
		registerSendMessageTool(server, service, token)
		registerTerminateTool(server, service, token)
	}
}

func registerLaunchTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name: "specialist_launch_task",
		Description: `Launch an attached specialist to work on a bounded task in its own runtime sandbox.

Use this when the current employee task would benefit from a specialist doing parallel coding, research, review, or implementation work. Provide the specialist slug and a complete brief. The control plane automatically binds the task to the current employee session; do not provide employee_id, sandbox_id, or session_id.

Returns a task_id. Use specialist_task_status with that task_id to check progress. Use specialist_task_send_message only to add context or redirect the task.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"specialist_slug": map[string]any{
					"type":        "string",
					"description": "Slug of an attached specialist, for example software-engineering-specialist. If the slug is invalid, the error response lists attached_specialist_slugs.",
				},
				"brief": map[string]any{
					"type":        "string",
					"description": "Complete task brief for the specialist: objective, relevant context, constraints, expected output, and any files/systems to inspect.",
					"minLength":   1,
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Optional structured context for audit/debugging, such as source issue id or parent task label. Do not put secrets here.",
				},
			},
			"required": []string{"specialist_slug", "brief"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			SpecialistSlug string         `json:"specialist_slug"`
			Brief          string         `json:"brief"`
			Metadata       map[string]any `json:"metadata"`
			SessionID      string         `json:"_hivy_session_id"`
		}
		decodeArgs(req, &params)
		resp, toolErr := service.Launch(ctx, LaunchRequest{
			Token:             token,
			SpecialistSlug:    params.SpecialistSlug,
			Brief:             params.Brief,
			Metadata:          params.Metadata,
			EmployeeSessionID: params.SessionID,
		})
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		return toolJSON(resp)
	})
}

func registerStatusTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name: "specialist_task_status",
		Description: `Read the current state and recent events for a specialist task.

Use this after specialist_launch_task returns a task_id, or when you need to decide whether to wait, send more context, terminate, or summarize the specialist's result. Pass exactly the task_id returned by specialist_launch_task.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "UUID returned by specialist_launch_task.",
				},
			},
			"required": []string{"task_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, toolErr := parseTaskID(req)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		resp, toolErr := service.Status(ctx, token, taskID)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		return toolJSON(resp)
	})
}

func registerSendMessageTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name: "specialist_task_send_message",
		Description: `Send additional context, a correction, or a follow-up instruction to a running specialist task.

Use this only after specialist_launch_task has returned a task_id. Do not use this to create a new task; launch a separate specialist task instead. After sending, call specialist_task_status to observe progress.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string", "description": "UUID returned by specialist_launch_task."},
				"message": map[string]any{"type": "string", "description": "Context or instruction to deliver to the specialist task.", "minLength": 1},
			},
			"required": []string{"task_id", "message"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			TaskID  string `json:"task_id"`
			Message string `json:"message"`
		}
		decodeArgs(req, &params)
		taskID, toolErr := parseUUIDField("task_id", params.TaskID)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		resp, toolErr := service.SendMessage(ctx, token, taskID, params.Message)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		return toolJSON(resp)
	})
}

func registerTerminateTool(server *mcp.Server, service *Service, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name: "specialist_task_terminate",
		Description: `Terminate a specialist task and clean up its runtime sandbox.

Use this when the task is no longer needed, was launched by mistake, is stuck, or should stop before completion. After termination, do not send more messages to the same task_id.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string", "description": "UUID returned by specialist_launch_task."},
				"reason":  map[string]any{"type": "string", "description": "Optional human-readable reason for termination."},
			},
			"required": []string{"task_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var params struct {
			TaskID string `json:"task_id"`
			Reason string `json:"reason"`
		}
		decodeArgs(req, &params)
		taskID, toolErr := parseUUIDField("task_id", params.TaskID)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		resp, toolErr := service.Terminate(ctx, token, taskID, params.Reason)
		if toolErr != nil {
			return toolErrorJSON(toolErr), nil
		}
		return toolJSON(resp)
	})
}

func parseTaskID(req *mcp.CallToolRequest) (uuid.UUID, *ToolError) {
	var params struct {
		TaskID string `json:"task_id"`
	}
	decodeArgs(req, &params)
	return parseUUIDField("task_id", params.TaskID)
}

func parseUUIDField(name, value string) (uuid.UUID, *ToolError) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, newToolError("missing_"+name, name+" is required.", "The required UUID argument was empty.", false, "Use the exact task_id returned by specialist_launch_task.")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, wrapToolError("invalid_"+name, name+" must be a valid UUID.", err, false, "Use the exact task_id returned by specialist_launch_task.")
	}
	return id, nil
}

func decodeArgs(req *mcp.CallToolRequest, out any) {
	if req != nil && req.Params != nil && req.Params.Arguments != nil {
		_ = json.Unmarshal(req.Params.Arguments, out)
	}
}

func toolErrorJSON(err *ToolError) *mcp.CallToolResult {
	bytes, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		bytes = []byte(`{"error_code":"serialize_error","message":"Failed to serialize tool error.","cause":"json marshal failed","retryable":true,"how_to_fix":"Retry the tool call. If it repeats, report that the control plane could not serialize the error response."}`)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(bytes)}}, IsError: true}
}

func toolJSON(value any) (*mcp.CallToolResult, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return toolErrorJSON(wrapToolError("serialize_error", "Failed to serialize tool response.", err, true, "Retry the call. If it repeats, report that the control plane could not serialize the response.")), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(bytes)}}}, nil
}
