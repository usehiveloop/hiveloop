package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type startEmployeeSandboxUpgradeRequest struct{}

type employeeSandboxUpgradeResponse struct {
	UpgradeID    string     `json:"upgrade_id"`
	Status       string     `json:"status"`
	Phase        string     `json:"phase"`
	OldSandboxID *string    `json:"old_sandbox_id,omitempty"`
	NewSandboxID *string    `json:"new_sandbox_id,omitempty"`
	BackupKey    *string    `json:"backup_key,omitempty"`
	BackupSHA256 *string    `json:"backup_sha256,omitempty"`
	BackupBytes  int64      `json:"backup_bytes,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// @Summary Start an employee sandbox upgrade
// @Description Queues a control-plane upgrade that snapshots the employee runtime SQLite database,
// @Description recreates the sandbox on the current employee image, restores the database,
// @Description syncs config, verifies readiness, pauses the old sandbox, and schedules cleanup.
// @Description If an upgrade is already queued or running for the employee, the active operation is returned.
// @Tags employees
// @Accept json
// @Produce json
// @Param id path string true "Employee agent ID"
// @Success 202 {object} employeeSandboxUpgradeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sandbox/upgrade [post]
func (h *EmployeeHandler) StartSandboxUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logging.FromContext(ctx)

	if h.enqueuer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "employee sandbox upgrades not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}
	var req startEmployeeSandboxUpgradeRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}

	if existing, ok, err := activeEmployeeSandboxUpgrade(ctx, h.db, org.ID, agentID); err != nil {
		log.ErrorContext(ctx, "load active employee sandbox upgrade", "error", err, "employee_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load active upgrade"})
		return
	} else if ok {
		writeJSON(w, http.StatusAccepted, toEmployeeSandboxUpgradeResponse(existing))
		return
	}

	var agent model.Employee
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return
		}
		log.ErrorContext(ctx, "load employee for sandbox upgrade", "error", err, "employee_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return
	}
	if err := h.deleteStaleEmployeeSandboxUpgradeTask(agentID); err != nil {
		log.ErrorContext(ctx, "delete stale employee sandbox upgrade task", "error", err, "employee_id", agentID)
		if strings.Contains(err.Error(), "active state") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "employee sandbox upgrade task is already running"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to clear stale upgrade task"})
		return
	}

	var oldSandbox model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("employee_id = ? AND org_id = ? AND status <> ?", agentID, org.ID, "error").
		Order("created_at DESC").Limit(1).First(&oldSandbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "sandbox not found for employee"})
			return
		}
		log.ErrorContext(ctx, "load employee sandbox for upgrade", "error", err, "employee_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee sandbox"})
		return
	}

	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        org.ID,
		EmployeeID:   agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.EmployeeSandboxUpgradeStatusQueued,
		Phase:        model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := h.db.WithContext(ctx).Create(&upgrade).Error; err != nil {
		log.ErrorContext(ctx, "create employee sandbox upgrade", "error", err, "employee_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upgrade"})
		return
	}

	task, opts, err := tasks.NewEmployeeSandboxUpgradeTask(upgrade.ID, agent.ID)
	if err != nil {
		h.markUpgradeFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseQueued, err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build upgrade task"})
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		h.markUpgradeFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseQueued, err.Error())
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			if existing, ok, loadErr := activeEmployeeSandboxUpgrade(ctx, h.db, org.ID, agentID); loadErr == nil && ok {
				writeJSON(w, http.StatusAccepted, toEmployeeSandboxUpgradeResponse(existing))
				return
			}
		}
		log.ErrorContext(ctx, "enqueue employee sandbox upgrade", "error", err, "upgrade_id", upgrade.ID, "employee_id", agentID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue upgrade"})
		return
	}

	writeJSON(w, http.StatusAccepted, toEmployeeSandboxUpgradeResponse(&upgrade))
}

// @Summary Get an employee sandbox upgrade
// @Description Returns the current status and phase for a sandbox upgrade operation.
// @Tags employees
// @Produce json
// @Param id path string true "Employee agent ID"
// @Param upgradeID path string true "Upgrade operation ID"
// @Success 200 {object} employeeSandboxUpgradeResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/employees/{id}/sandbox/upgrades/{upgradeID} [get]
func (h *EmployeeHandler) GetSandboxUpgrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee id"})
		return
	}
	upgradeID, err := uuid.Parse(chi.URLParam(r, "upgradeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upgrade id"})
		return
	}
	var upgrade model.EmployeeSandboxUpgrade
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND employee_id = ?", upgradeID, org.ID, agentID).
		First(&upgrade).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upgrade not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load upgrade"})
		return
	}
	writeJSON(w, http.StatusOK, toEmployeeSandboxUpgradeResponse(&upgrade))
}

func (h *EmployeeHandler) markUpgradeFailed(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase, msg string) {
	now := time.Now()
	_ = h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.EmployeeSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": msg,
		"completed_at":  now,
	}).Error
	upgrade.Status = model.EmployeeSandboxUpgradeStatusFailed
	upgrade.Phase = phase
	upgrade.ErrorMessage = &msg
	upgrade.CompletedAt = &now
}

func activeEmployeeSandboxUpgrade(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.EmployeeSandboxUpgrade, bool, error) {
	var upgrade model.EmployeeSandboxUpgrade
	err := db.WithContext(ctx).
		Where("org_id = ? AND employee_id = ? AND status IN ?", orgID, agentID, []string{
			model.EmployeeSandboxUpgradeStatusQueued,
			model.EmployeeSandboxUpgradeStatusRunning,
		}).
		Order("created_at DESC").
		First(&upgrade).Error
	if err == nil {
		return &upgrade, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	return nil, false, err
}

func writeEmployeeUpgradeConflict(w http.ResponseWriter, upgrade *model.EmployeeSandboxUpgrade) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error":      "employee sandbox upgrade in progress",
		"upgrade_id": upgrade.ID.String(),
		"status":     upgrade.Status,
		"phase":      upgrade.Phase,
	})
}

func toEmployeeSandboxUpgradeResponse(upgrade *model.EmployeeSandboxUpgrade) employeeSandboxUpgradeResponse {
	resp := employeeSandboxUpgradeResponse{
		UpgradeID:    upgrade.ID.String(),
		Status:       upgrade.Status,
		Phase:        upgrade.Phase,
		ErrorMessage: upgrade.ErrorMessage,
		BackupKey:    upgrade.BackupKey,
		BackupSHA256: upgrade.BackupSHA256,
		BackupBytes:  upgrade.BackupBytes,
		CreatedAt:    upgrade.CreatedAt,
		UpdatedAt:    upgrade.UpdatedAt,
		CompletedAt:  upgrade.CompletedAt,
	}
	if upgrade.OldSandboxID != nil {
		id := upgrade.OldSandboxID.String()
		resp.OldSandboxID = &id
	}
	if upgrade.NewSandboxID != nil {
		id := upgrade.NewSandboxID.String()
		resp.NewSandboxID = &id
	}
	return resp
}
