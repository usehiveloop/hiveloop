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

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/specialists"
)

type Service struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	catalog      *specialists.Catalog
	now          func() time.Time
}

func NewService(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps, catalog *specialists.Catalog) *Service {
	return &Service{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		catalog:      catalog,
		now:          time.Now,
	}
}

type LaunchRequest struct {
	Token             *model.Token
	SpecialistSlug    string
	Brief             string
	Metadata          map[string]any
	EmployeeSessionID string
}

type LaunchResponse struct {
	TaskID            string `json:"task_id"`
	SpecialistSlug    string `json:"specialist_slug"`
	EmployeeSessionID string `json:"employee_session_id"`
	SandboxID         string `json:"sandbox_id"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	NextAction        string `json:"next_action"`
}

type TaskStatusResponse struct {
	TaskID            string           `json:"task_id"`
	SpecialistSlug    string           `json:"specialist_slug"`
	EmployeeSessionID string           `json:"employee_session_id"`
	SandboxID         string           `json:"sandbox_id"`
	Status            string           `json:"status"`
	Brief             string           `json:"brief"`
	CreatedAt         time.Time        `json:"created_at"`
	EndedAt           *time.Time       `json:"ended_at,omitempty"`
	RecentEvents      []TaskEventBrief `json:"recent_events"`
	NextAction        string           `json:"next_action"`
}

type TaskEventBrief struct {
	EventType string          `json:"event_type"`
	Source    string          `json:"source"`
	Payload   json.RawMessage `json:"payload"`
	EventAt   time.Time       `json:"event_at"`
}

type MessageResponse struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}

func (s *Service) Launch(ctx context.Context, req LaunchRequest) (*LaunchResponse, *ToolError) {
	if err := s.requireReady(); err != nil {
		return nil, err
	}
	employee, toolErr := s.employeeFromToken(ctx, req.Token)
	if toolErr != nil {
		return nil, toolErr
	}
	req.SpecialistSlug = strings.TrimSpace(req.SpecialistSlug)
	req.Brief = strings.TrimSpace(req.Brief)
	req.EmployeeSessionID = strings.TrimSpace(req.EmployeeSessionID)
	if req.EmployeeSessionID == "" {
		return nil, newToolError("missing_session_context", "The runtime did not provide the current session id.", "The specialist_launch_task tool must be called from inside an active employee runtime session.", false, "Call this tool from an active employee conversation. If you are already in one, retry once; if it repeats, report that runtime session context is missing.")
	}
	if req.SpecialistSlug == "" {
		return nil, s.invalidSlugError(employee, "specialist_slug is required.")
	}
	if req.Brief == "" {
		return nil, newToolError("missing_brief", "brief is required.", "The task brief was empty after trimming whitespace.", false, "Call specialist_launch_task again with a clear task brief that includes the objective, relevant context, and expected output.")
	}
	def, toolErr := s.validateSpecialist(employee, req.SpecialistSlug)
	if toolErr != nil {
		return nil, toolErr
	}

	secrets, err := employeeruntime.PrepareStartup(ctx, s.compileDeps, employee)
	if err != nil {
		return nil, wrapToolError("proxy_token_failed", "Could not prepare specialist runtime credentials.", err, true, "Retry the task. If it fails again, report that the control plane could not mint runtime credentials.")
	}
	sb, err := s.orchestrator.CreateSpecialistRuntimeSandbox(ctx, employee, secrets)
	if err != nil {
		return nil, wrapToolError("sandbox_create_failed", "Could not create the specialist runtime sandbox.", err, true, "Retry later. If this repeats, report the sandbox provisioning error to the user.")
	}
	if err := employeeruntime.AttachLatestProxyTokenToSandbox(ctx, s.compileDeps, employee, sb.ID); err != nil {
		return nil, wrapToolError("proxy_token_attach_failed", "The specialist runtime was created, but the control plane could not bind its proxy token to the sandbox.", err, true, "Retry later. If this repeats, report that specialist startup failed after sandbox creation.")
	}
	runtimeDef, err := employeeruntime.CompileSpecialist(ctx, s.compileDeps, employee, *def)
	if err != nil {
		return nil, wrapToolError("config_compile_failed", "Could not compile specialist runtime config.", err, false, "Report that the specialist configuration could not be compiled.")
	}
	runtimeDef.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(s.compileDeps.Cfg, sb.ID)
	client, err := s.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return nil, wrapToolError("runtime_client_failed", "The specialist sandbox exists, but the control plane could not connect to its runtime API.", err, true, "Retry later. If this repeats, report that the specialist runtime is unavailable.")
	}
	if _, err := client.PutConfig(ctx, runtimeDef); err != nil {
		return nil, wrapToolError("config_push_failed", "Could not push specialist runtime config.", err, true, "Retry later. If this repeats, report that the specialist runtime rejected its config.")
	}

	task := model.SpecialistTask{
		ID:                     uuid.New(),
		OrgID:                  *employee.OrgID,
		EmployeeID:             employee.ID,
		SpecialistSlug:         def.Slug,
		EmployeeSessionID:      req.EmployeeSessionID,
		SandboxID:              sb.ID,
		ParentConversationType: "employee_session",
		ParentConversationID:   req.EmployeeSessionID,
		Brief:                  req.Brief,
		Metadata:               model.JSON(req.Metadata),
		Status:                 "running",
	}
	if task.Metadata == nil {
		task.Metadata = model.JSON{}
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, wrapToolError("task_store_failed", "The specialist runtime was initialized, but the control plane could not store the task record.", err, true, "Retry later. If this repeats, report that task tracking failed after runtime startup.")
	}
	if _, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:           req.Brief,
		ConversationID: req.EmployeeSessionID,
		User:           "hivy-control-plane",
		Raw: map[string]any{
			"specialist_task_id": task.ID.String(),
			"specialist_slug":    def.Slug,
			"source":             "specialist_launch_task",
		},
	}); err != nil {
		_ = s.db.WithContext(ctx).Model(&task).Updates(map[string]any{"status": "error", "updated_at": s.now().UTC()}).Error
		return nil, wrapToolError("initial_message_failed", "The specialist runtime was created, but the task brief could not be delivered.", err, true, "Retry specialist_launch_task. If a task_id was returned previously, use specialist_task_status before launching another duplicate task.")
	}
	return &LaunchResponse{
		TaskID:            task.ID.String(),
		SpecialistSlug:    task.SpecialistSlug,
		EmployeeSessionID: task.EmployeeSessionID,
		SandboxID:         task.SandboxID.String(),
		Status:            task.Status,
		Message:           "Specialist task launched and the brief was delivered.",
		NextAction:        "Use specialist_task_status with this task_id to check progress. Use specialist_task_send_message only if you need to add context or redirect the task.",
	}, nil
}

func (s *Service) Status(ctx context.Context, token *model.Token, taskID uuid.UUID) (*TaskStatusResponse, *ToolError) {
	task, employee, toolErr := s.loadOwnedTask(ctx, token, taskID)
	if toolErr != nil {
		return nil, toolErr
	}
	events, toolErr := s.recentEvents(ctx, employee, task, 30)
	if toolErr != nil {
		return nil, toolErr
	}
	return &TaskStatusResponse{
		TaskID:            task.ID.String(),
		SpecialistSlug:    task.SpecialistSlug,
		EmployeeSessionID: task.EmployeeSessionID,
		SandboxID:         task.SandboxID.String(),
		Status:            task.Status,
		Brief:             task.Brief,
		CreatedAt:         task.CreatedAt,
		EndedAt:           task.EndedAt,
		RecentEvents:      events,
		NextAction:        "If the task is still running, wait and call specialist_task_status again. If more context is needed, call specialist_task_send_message with this task_id.",
	}, nil
}

func (s *Service) SendMessage(ctx context.Context, token *model.Token, taskID uuid.UUID, message string) (*MessageResponse, *ToolError) {
	task, _, toolErr := s.loadOwnedTask(ctx, token, taskID)
	if toolErr != nil {
		return nil, toolErr
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, newToolError("missing_message", "message is required.", "The message was empty after trimming whitespace.", false, "Call specialist_task_send_message again with the context, correction, or instruction you want the specialist to receive.")
	}
	var sb model.Sandbox
	if err := s.db.WithContext(ctx).Where("id = ?", task.SandboxID).First(&sb).Error; err != nil {
		return nil, wrapToolError("sandbox_not_found", "The specialist task sandbox could not be loaded.", err, false, "Call specialist_task_status to check whether the task still exists. If it does, report that its sandbox record is missing.")
	}
	client, err := s.orchestrator.GetRuntimeClient(ctx, &sb)
	if err != nil {
		return nil, wrapToolError("runtime_client_failed", "Could not connect to the specialist runtime API.", err, true, "Retry later. If this repeats, report that the specialist runtime is unavailable.")
	}
	if _, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:           message,
		ConversationID: task.EmployeeSessionID,
		User:           "hivy-control-plane",
		Raw: map[string]any{
			"specialist_task_id": task.ID.String(),
			"specialist_slug":    task.SpecialistSlug,
			"source":             "specialist_task_send_message",
		},
	}); err != nil {
		return nil, wrapToolError("message_send_failed", "Could not send the message to the specialist runtime.", err, true, "Retry once. If it fails again, call specialist_task_status and report the runtime communication problem.")
	}
	return &MessageResponse{TaskID: task.ID.String(), Status: task.Status, Message: "Message delivered to specialist task.", NextAction: "Call specialist_task_status to observe the specialist response or progress."}, nil
}

func (s *Service) Terminate(ctx context.Context, token *model.Token, taskID uuid.UUID, reason string) (*MessageResponse, *ToolError) {
	task, _, toolErr := s.loadOwnedTask(ctx, token, taskID)
	if toolErr != nil {
		return nil, toolErr
	}
	now := s.now().UTC()
	if err := s.db.WithContext(ctx).Model(&task).Updates(map[string]any{
		"status":     "terminated",
		"ended_at":   now,
		"updated_at": now,
	}).Error; err != nil {
		return nil, wrapToolError("task_update_failed", "Could not mark the specialist task as terminated.", err, true, "Retry specialist_task_terminate. If it repeats, report that the task state update failed.")
	}
	var sb model.Sandbox
	if err := s.db.WithContext(ctx).Where("id = ?", task.SandboxID).First(&sb).Error; err == nil {
		_ = s.orchestrator.DeleteSandboxResource(ctx, &sb)
	}
	return &MessageResponse{TaskID: task.ID.String(), Status: "terminated", Message: "Specialist task terminated.", NextAction: "Do not send more messages to this task. Launch a new specialist task if more work is needed."}, nil
}

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
	tokenType, _ := token.Meta["type"].(string)
	employeeIDRaw, _ := token.Meta["employee_id"].(string)
	if tokenType != "employee_proxy" || strings.TrimSpace(employeeIDRaw) == "" {
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
	err := newToolError("invalid_specialist_slug", message, "The requested specialist is missing, unknown, or not attached to this employee.", false, "Call specialist_launch_task again with one of the attached_specialist_slugs values.")
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

func (s *Service) recentEvents(ctx context.Context, employee *model.Employee, task *model.SpecialistTask, limit int) ([]TaskEventBrief, *ToolError) {
	var rows []model.EmployeeMemoryEvent
	if err := s.db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND specialist_task_id = ?", *employee.OrgID, employee.ID, task.ID).
		Order("event_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, wrapToolError("event_load_failed", "Could not load recent specialist events.", err, true, "Retry specialist_task_status. If it repeats, report that task events are unavailable.")
	}
	out := make([]TaskEventBrief, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		out = append(out, TaskEventBrief{
			EventType: row.EventType,
			Source:    row.Source,
			Payload:   json.RawMessage(row.Payload),
			EventAt:   row.EventAt,
		})
	}
	return out, nil
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
