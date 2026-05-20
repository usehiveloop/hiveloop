package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (h *EmployeeSandboxUpgradeHandler) loadAndStart(ctx context.Context, payload EmployeeSandboxUpgradePayload) (*model.EmployeeSandboxUpgrade, *model.Agent, *model.Sandbox, error) {
	var upgrade model.EmployeeSandboxUpgrade
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND agent_id = ?", payload.UpgradeID, payload.AgentID).
			First(&upgrade).Error; err != nil {
			return err
		}
		switch upgrade.Status {
		case model.EmployeeSandboxUpgradeStatusSucceeded, model.EmployeeSandboxUpgradeStatusFailed:
			return nil
		case model.EmployeeSandboxUpgradeStatusQueued, model.EmployeeSandboxUpgradeStatusRunning:
		default:
			return fmt.Errorf("unsupported employee sandbox upgrade status %q", upgrade.Status)
		}
		return tx.Model(&upgrade).Updates(map[string]any{
			"status": model.EmployeeSandboxUpgradeStatusRunning,
			"phase":  model.EmployeeSandboxUpgradePhaseBackup,
		}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("load employee sandbox upgrade: %w", err)
	}
	if upgrade.Status == model.EmployeeSandboxUpgradeStatusSucceeded || upgrade.Status == model.EmployeeSandboxUpgradeStatusFailed {
		return nil, nil, nil, nil
	}
	upgrade.Status = model.EmployeeSandboxUpgradeStatusRunning
	upgrade.Phase = model.EmployeeSandboxUpgradePhaseBackup

	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", upgrade.AgentID, upgrade.OrgID, "archived").
		First(&agent).Error; err != nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseBackup, "employee not found")
		return nil, nil, nil, fmt.Errorf("load employee: %w", err)
	}
	if agent.OrgID == nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseBackup, "employee missing org")
		return nil, nil, nil, fmt.Errorf("employee missing org")
	}
	if ok, err := h.hasActiveSlackConnection(ctx, upgrade.OrgID); err != nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseBackup, err.Error())
		return nil, nil, nil, err
	} else if !ok {
		err := fmt.Errorf("organization must have an active Slack connection")
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseBackup, err.Error())
		return nil, nil, nil, err
	}

	var oldSandbox model.Sandbox
	query := h.db.WithContext(ctx).Where("agent_id = ? AND org_id = ? AND status <> ?", upgrade.AgentID, upgrade.OrgID, string(sandbox.StatusError))
	if upgrade.OldSandboxID != nil {
		query = query.Where("id = ?", *upgrade.OldSandboxID)
	}
	if err := query.Order("created_at DESC").Limit(1).First(&oldSandbox).Error; err != nil {
		h.markFailed(ctx, &upgrade, model.EmployeeSandboxUpgradePhaseBackup, "current sandbox not found")
		return nil, nil, nil, fmt.Errorf("load current sandbox: %w", err)
	}
	if upgrade.OldSandboxID == nil {
		if err := h.db.WithContext(ctx).Model(&upgrade).Update("old_sandbox_id", oldSandbox.ID).Error; err != nil {
			return nil, nil, nil, fmt.Errorf("record old sandbox: %w", err)
		}
		upgrade.OldSandboxID = &oldSandbox.ID
	}
	return &upgrade, &agent, &oldSandbox, nil
}

func (h *EmployeeSandboxUpgradeHandler) hasActiveSlackConnection(ctx context.Context, orgID uuid.UUID) (bool, error) {
	var count int64
	err := h.db.WithContext(ctx).Model(&model.InConnection{}).
		Joins("JOIN in_integrations ON in_integrations.id = in_connections.in_integration_id AND in_integrations.deleted_at IS NULL").
		Where("in_connections.org_id = ? AND in_connections.revoked_at IS NULL AND in_integrations.provider = ?",
			orgID, "slack").
		Count(&count).Error
	return count > 0, err
}

func (h *EmployeeSandboxUpgradeHandler) syncEmployeeRuntime(ctx context.Context, agent *model.Agent, sb *model.Sandbox) error {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, h.compileDeps, agent)
	if err != nil {
		return fmt.Errorf("compile employee config: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if _, err := client.PutConfig(ctx, def); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	if agent.Status != "active" {
		if err := h.db.WithContext(ctx).Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return fmt.Errorf("mark employee active: %w", err)
		}
		agent.Status = "active"
	}
	return nil
}

func (h *EmployeeSandboxUpgradeHandler) runSmokeTest(ctx context.Context, sb *model.Sandbox) error {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt runtime secret for smoke test: %w", err)
	}
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("smoke healthz: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("smoke readyz: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"text":            "upgrade smoke test",
		"conversation_id": "upgrade-smoke-" + sb.ID.String(),
		"user":            "hivy-control-plane",
		"raw": map[string]any{
			"upgrade_smoke": true,
			"sandbox_id":    sb.ID.String(),
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(sb.BridgeURL, "/")+"/gateway/http/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("smoke http gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("smoke http gateway: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (h *EmployeeSandboxUpgradeHandler) markPhase(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase string) error {
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status": model.EmployeeSandboxUpgradeStatusRunning,
		"phase":  phase,
	}).Error; err != nil {
		return fmt.Errorf("mark employee sandbox upgrade %s: %w", phase, err)
	}
	upgrade.Status = model.EmployeeSandboxUpgradeStatusRunning
	upgrade.Phase = phase
	recordEmployeeSandboxUpgradePhase(ctx, upgrade, phase)
	return nil
}

func (h *EmployeeSandboxUpgradeHandler) markFailed(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, phase, message string) {
	now := time.Now().UTC()
	truncated := truncateUpgradeError(message)
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"status":        model.EmployeeSandboxUpgradeStatusFailed,
		"phase":         phase,
		"error_message": truncated,
		"completed_at":  now,
	}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee sandbox upgrade: mark failed: %w", err))
	}
	upgrade.Status = model.EmployeeSandboxUpgradeStatusFailed
	upgrade.Phase = phase
	upgrade.ErrorMessage = &truncated
	upgrade.CompletedAt = &now
	recordEmployeeSandboxUpgradeFailure(ctx, upgrade, phase, truncated)
}

func truncateUpgradeError(msg string) string {
	msg = strings.TrimSpace(msg)
	if len(msg) > 2000 {
		return msg[:2000]
	}
	return msg
}
