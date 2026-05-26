package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	employeeProxyTokenRefreshLead    = 4 * time.Hour
	employeeProxyTokenRefreshTimeout = 2 * time.Minute
	employeeProxyTokenRefreshDedupe  = time.Hour
)

type EmployeeProxyTokenRefreshPayload struct {
	EmployeeID  uuid.UUID `json:"employee_id"`
	SandboxID   uuid.UUID `json:"sandbox_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

func ScheduleEmployeeProxyTokenRefresh(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, agent *model.Employee, sb *model.Sandbox) error {
	if db == nil || enqueuer == nil || agent == nil || agent.OrgID == nil || sb == nil || sb.ID == uuid.Nil {
		return nil
	}
	if agent.ID == uuid.Nil || agent.Harness != "employee-sandbox" {
		return nil
	}
	if sb.EmployeeID == nil || *sb.EmployeeID != agent.ID {
		return nil
	}
	scheduledAt, err := nextEmployeeProxyTokenRefreshAt(ctx, db, agent, sb.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	task, opts, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		EmployeeID:  agent.ID,
		SandboxID:   sb.ID,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue employee proxy token refresh: %w", err)
	}
	return nil
}

func NewEmployeeProxyTokenRefreshTask(payload EmployeeProxyTokenRefreshPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("employee proxy token refresh payload missing ids")
	}
	if payload.ScheduledAt.IsZero() {
		payload.ScheduledAt = time.Now().UTC()
	}
	payload.ScheduledAt = payload.ScheduledAt.UTC()
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal employee proxy token refresh payload: %w", err)
	}
	now := time.Now().UTC()
	uniqueTTL := time.Until(payload.ScheduledAt) + employeeProxyTokenRefreshDedupe
	if uniqueTTL < employeeProxyTokenRefreshDedupe {
		uniqueTTL = employeeProxyTokenRefreshDedupe
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(6),
		asynq.Timeout(employeeProxyTokenRefreshTimeout),
		asynq.Unique(uniqueTTL),
		asynq.TaskID(fmt.Sprintf("employee-proxy-token-refresh:%s:%d", payload.SandboxID, payload.ScheduledAt.Unix())),
	}
	if payload.ScheduledAt.After(now) {
		opts = append(opts, asynq.ProcessAt(payload.ScheduledAt))
	}
	return asynq.NewTask(TypeEmployeeProxyTokenRefresh, body), opts, nil
}

type EmployeeProxyTokenRefreshHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewEmployeeProxyTokenRefreshHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	compileDeps employeeruntime.CompileDeps,
	enqueuer enqueue.TaskEnqueuer,
) *EmployeeProxyTokenRefreshHandler {
	return &EmployeeProxyTokenRefreshHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     enqueuer,
	}
}

func (h *EmployeeProxyTokenRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil || h.enqueuer == nil {
		return fmt.Errorf("employee proxy token refresh handler not configured")
	}
	var payload EmployeeProxyTokenRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal employee proxy token refresh payload: %w", err)
	}
	if payload.EmployeeID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return fmt.Errorf("employee proxy token refresh payload missing ids")
	}
	return h.run(ctx, payload)
}

func (h *EmployeeProxyTokenRefreshHandler) run(ctx context.Context, payload EmployeeProxyTokenRefreshPayload) error {
	agent, sb, ok, err := h.loadAgentAndSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			return fmt.Errorf("refresh employee sandbox url: %w", err)
		}
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt employee runtime secret: %w", err)
	}
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}

	refreshed, err := employeeruntime.MintProxyToken(ctx, h.compileDeps, agent, sb.ID)
	if err != nil {
		return err
	}
	def, err := client.GetConfig(ctx)
	if err != nil {
		h.revokeToken(ctx, refreshed.JTI)
		return fmt.Errorf("employee runtime config: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, employeeruntime.ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: map[string]string{employeeruntime.ProxyAPIKeyEnv: refreshed.Token},
	}); err != nil {
		h.revokeToken(ctx, refreshed.JTI)
		return err
	}
	if err := client.Readyz(ctx); err != nil {
		h.revokeToken(ctx, refreshed.JTI)
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	if err := h.revokeOlderTokens(ctx, agent, sb, refreshed.JTI); err != nil {
		return err
	}
	if err := h.markAgentRefreshed(ctx, agent); err != nil {
		return err
	}
	if err := ScheduleEmployeeProxyTokenRefresh(ctx, h.db, h.enqueuer, agent, sb); err != nil {
		return err
	}
	logging.FromContext(ctx).InfoContext(ctx, "employee proxy token refreshed",
		"employee_id", agent.ID, "sandbox_id", sb.ID, "jti", refreshed.JTI)
	return nil
}

func (h *EmployeeProxyTokenRefreshHandler) markAgentRefreshed(ctx context.Context, agent *model.Employee) error {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.Employee{}).
		Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
		Update("last_proxy_token_refreshed_at", now).Error; err != nil {
		return fmt.Errorf("mark employee proxy token refreshed: %w", err)
	}
	agent.LastProxyTokenRefreshedAt = &now
	return nil
}

func (h *EmployeeProxyTokenRefreshHandler) loadAgentAndSandbox(ctx context.Context, payload EmployeeProxyTokenRefreshPayload) (*model.Employee, *model.Sandbox, bool, error) {
	var agent model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND harness = ? AND status <> ?", payload.EmployeeID, "employee-sandbox", "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("load employee for proxy token refresh: %w", err)
	}
	if agent.OrgID == nil {
		return nil, nil, false, nil
	}
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("id = ? AND employee_id = ? AND org_id = ?", payload.SandboxID, agent.ID, *agent.OrgID).
		First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("load employee sandbox for proxy token refresh: %w", err)
	}
	switch sb.Status {
	case string(sandbox.StatusArchived), string(sandbox.StatusArchiving), string(sandbox.StatusError):
		return nil, nil, false, nil
	}
	return &agent, &sb, true, nil
}

func (h *EmployeeProxyTokenRefreshHandler) revokeOlderTokens(ctx context.Context, agent *model.Employee, sb *model.Sandbox, keepJTI string) error {
	if agent == nil || agent.OrgID == nil || sb == nil || keepJTI == "" {
		return nil
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.Token{}).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND jti != ? AND revoked_at IS NULL",
			*agent.OrgID,
			model.TokenMetaEmployeeID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeEmployeeProxy,
			model.TokenMetaHarness, model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode, model.TokenRuntimeModeEmployee,
			keepJTI).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke older employee proxy tokens: %w", err)
	}
	return nil
}

func (h *EmployeeProxyTokenRefreshHandler) revokeToken(ctx context.Context, jti string) {
	if jti == "" {
		return
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.Token{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", now).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee proxy token refresh: revoke failed token: %w", err))
	}
}

func nextEmployeeProxyTokenRefreshAt(ctx context.Context, db *gorm.DB, agent *model.Employee, sandboxID uuid.UUID, now time.Time) (time.Time, error) {
	if agent == nil || agent.OrgID == nil {
		return now.UTC(), nil
	}
	var tok model.Token
	err := db.WithContext(ctx).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NULL",
			*agent.OrgID,
			model.TokenMetaEmployeeID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeEmployeeProxy,
			model.TokenMetaHarness, model.TokenHarnessEmployeeSandbox,
			model.TokenMetaRuntimeMode, model.TokenRuntimeModeEmployee).
		Where("COALESCE(meta->>?, '') IN (?, '')", model.TokenMetaSandboxID, sandboxID.String()).
		Order("created_at DESC").
		First(&tok).Error
	if err == nil {
		return tok.ExpiresAt.Add(-employeeProxyTokenRefreshLead).UTC(), nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return now.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("load latest employee proxy token: %w", err)
}
