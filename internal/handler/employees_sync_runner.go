package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/model"
	"gorm.io/gorm"
)

func (h *EmployeeHandler) ensureEmployeeSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent must have org_id")
	}
	var sb model.Sandbox
	err := h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ?", agent.ID, *agent.OrgID).
		Order("created_at DESC").Limit(1).First(&sb).Error
	if err == nil {
		if h.orchestrator.NeedsURLRefresh(&sb) {
			if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, &sb); err != nil {
				return nil, err
			}
		}
		return &sb, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load employee sandbox: %w", err)
	}
	secrets, prepErr := employeeruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if prepErr != nil {
		return nil, prepErr
	}
	return h.orchestrator.CreateEmployeeSandbox(ctx, agent, secrets)
}

func (h *EmployeeHandler) runEmployeeSync(ctx context.Context, agent *model.Agent, sb *model.Sandbox) (*employeeruntime.SyncResponse, error) {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, h.compileDeps, agent)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime healthz: %w", err)
	}
	resp, err := client.PutConfig(ctx, def)
	if err != nil {
		return nil, err
	}
	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime readyz: %w", err)
	}
	return resp, nil
}

func toSyncResponseDTO(resp *employeeruntime.SyncResponse) syncEmployeeResponse {
	out := syncEmployeeResponse{}
	if resp == nil {
		return out
	}
	out.Applied = resp.Applied
	out.Deleted = resp.Deleted
	out.ReposCloned = resp.ReposCloned
	out.RestartTriggered = resp.RestartTriggered
	out.Errors = resp.Errors
	return out
}
