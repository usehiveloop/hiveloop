package specialisttasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func (s *Service) requireReady() *ToolError {
	if s == nil || s.db == nil || s.orchestrator == nil || s.catalog == nil {
		return newToolError("service_unavailable", "Specialist tools are not configured in this control plane.", "Required service dependencies are missing.", true, "Retry later. If this repeats, report that specialist task tools are unavailable.")
	}
	return nil
}

func (s *Service) employeeFromToken(ctx context.Context, token *model.Token) (*model.Employee, *ToolError) {
	if token == nil {
		return nil, newToolError("missing_token", "No MCP token was available for this call.", "The control plane could not identify the calling employee.", false, "Retry from inside the employee runtime using the Hivy MCP tools.")
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	runtimeMode, _ := token.Meta[model.TokenMetaRuntimeMode].(string)
	employeeIDRaw, _ := token.Meta[model.TokenMetaEmployeeID].(string)
	if tokenType != model.TokenTypeEmployeeProxy || runtimeMode != model.TokenRuntimeModeEmployee || strings.TrimSpace(employeeIDRaw) == "" {
		return nil, newToolError("invalid_token_scope", "Specialist tools can only be used by an employee runtime proxy token.", "The MCP token is not scoped to an employee runtime.", false, "Use this tool only from inside the employee runtime conversation.")
	}
	employeeID, err := uuid.Parse(employeeIDRaw)
	if err != nil {
		return nil, wrapToolError("invalid_employee_id", "The MCP token contains an invalid employee_id.", err, false, "Report that the runtime token metadata is malformed.")
	}
	var employee model.Employee
	if err := s.db.WithContext(ctx).Where("id = ? AND org_id = ? AND status <> ?", employeeID, token.OrgID, "archived").First(&employee).Error; err != nil {
		return nil, wrapToolError("employee_not_found", "The calling employee could not be found.", err, false, "Report that the runtime token points to an employee that no longer exists or is not active.")
	}
	return &employee, nil
}

func (s *Service) validateSpecialist(employee *model.Employee, slug string) (*specialists.Definition, *ToolError) {
	def, ok := s.catalog.BySlug(slug)
	if !ok {
		return nil, s.invalidSlugError(employee, fmt.Sprintf("Unknown specialist_slug %q.", slug))
	}
	if !attachedSet(employee.AttachedSpecialists)[slug] {
		return nil, s.invalidSlugError(employee, fmt.Sprintf("Specialist %q is not attached to this employee.", slug))
	}
	return def, nil
}

func (s *Service) invalidSlugError(employee *model.Employee, message string) *ToolError {
	attached := []string{}
	if employee != nil {
		for _, def := range s.catalog.List() {
			if attachedSet(employee.AttachedSpecialists)[def.Slug] {
				attached = append(attached, def.Slug)
			}
		}
	}
	err := newToolError("invalid_specialist_slug", message, "The requested specialist is missing, unknown, or not attached to this employee.", false, "Call specialist_list, then call specialist_launch_task again with one of the attached specialist slugs.")
	err.Details = map[string]any{"attached_specialist_slugs": attached}
	return err
}

func (s *Service) loadOwnedTask(ctx context.Context, token *model.Token, taskID uuid.UUID) (*model.SpecialistTask, *model.Employee, *ToolError) {
	if err := s.requireReady(); err != nil {
		return nil, nil, err
	}
	employee, toolErr := s.employeeFromToken(ctx, token)
	if toolErr != nil {
		return nil, nil, toolErr
	}
	var task model.SpecialistTask
	err := s.db.WithContext(ctx).Where("id = ? AND org_id = ? AND employee_id = ?", taskID, token.OrgID, employee.ID).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, newToolError("task_not_found", "No specialist task was found for this task_id.", "The task id is unknown, belongs to another employee/org, or was deleted.", false, "Check the task_id returned by specialist_launch_task and call specialist_task_status again with the exact value.")
		}
		return nil, nil, wrapToolError("task_load_failed", "Could not load the specialist task.", err, true, "Retry the call. If it repeats, report that task lookup failed.")
	}
	return &task, employee, nil
}

type taskActivity struct {
	LastActivityAt  *time.Time
	MessageCount    int
	ToolCallCount   int
	ToolResultCount int
	ErrorCount      int
	LatestMessage   string
	LatestError     string
}

func (a taskActivity) Summary() string {
	parts := []string{}
	if a.MessageCount > 0 {
		parts = append(parts, fmt.Sprintf("%d message(s)", a.MessageCount))
	}
	if a.ToolCallCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tool call(s)", a.ToolCallCount))
	}
	if a.ToolResultCount > 0 {
		parts = append(parts, fmt.Sprintf("%d tool result(s)", a.ToolResultCount))
	}
	if a.ErrorCount > 0 {
		parts = append(parts, fmt.Sprintf("%d error event(s)", a.ErrorCount))
	}
	if len(parts) == 0 {
		return "no specialist events have been recorded yet"
	}
	return strings.Join(parts, ", ")
}

func (s *Service) taskActivity(ctx context.Context, employee *model.Employee, task *model.SpecialistTask, limit int) (taskActivity, *ToolError) {
	var rows []model.EmployeeSessionEvent
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND specialist_task_id = ?", *employee.OrgID, employee.ID, task.ID).
		Order("event_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return taskActivity{}, wrapToolError("event_load_failed", "Could not load recent specialist events.", err, true, "Retry specialist_task_status. If it repeats, report that task events are unavailable.")
	}
	activity := taskActivity{}
	for i, row := range rows {
		if i == 0 {
			lastActivityAt := row.EventAt
			activity.LastActivityAt = &lastActivityAt
		}
		switch {
		case row.EventType == "agent.message.sent":
			activity.MessageCount++
			if activity.LatestMessage == "" {
				activity.LatestMessage = compactText(payloadString(row.Payload, "text", "message", "content"), 600)
			}
		case row.EventType == "agent.tool.call":
			activity.ToolCallCount++
		case row.EventType == "agent.tool.result":
			activity.ToolResultCount++
		}
		if strings.Contains(row.EventType, "error") || strings.Contains(row.EventType, "failed") {
			activity.ErrorCount++
			if activity.LatestError == "" {
				activity.LatestError = compactText(payloadString(row.Payload, "error", "message", "reason", "cause"), 400)
			}
		}
	}
	return activity, nil
}

func payloadString(payload model.RawJSON, keys ...string) string {
	var value any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return ""
	}
	return nestedString(value, keys...)
}

func nestedString(value any, keys ...string) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		for _, key := range keys {
			if found, ok := typed[key]; ok {
				if text := nestedString(found, keys...); text != "" {
					return text
				}
			}
		}
		for _, found := range typed {
			if text := nestedString(found, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := nestedString(item, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func compactText(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func attachedSet(slugs []string) map[string]bool {
	out := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		out[slug] = true
	}
	return out
}

func wrapToolError(code, message string, err error, retryable bool, howToFix string) *ToolError {
	cause := ""
	if err != nil {
		cause = err.Error()
	}
	return newToolError(code, message, cause, retryable, howToFix)
}

type ToolError struct {
	ErrorCode string         `json:"error_code"`
	Message   string         `json:"message"`
	Cause     string         `json:"cause"`
	Retryable bool           `json:"retryable"`
	HowToFix  string         `json:"how_to_fix"`
	Details   map[string]any `json:"details,omitempty"`
}

func newToolError(code, message, cause string, retryable bool, howToFix string) *ToolError {
	return &ToolError{ErrorCode: code, Message: message, Cause: cause, Retryable: retryable, HowToFix: howToFix}
}
