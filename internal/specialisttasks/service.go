package specialisttasks

import (
	"context"
	"encoding/json"
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
