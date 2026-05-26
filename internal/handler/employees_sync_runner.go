package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
	"gorm.io/gorm"
)

func (h *EmployeeHandler) ensureEmployeeSandbox(ctx context.Context, agent *model.Employee) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent must have org_id")
	}
	var sb model.Sandbox
	err := h.db.WithContext(ctx).
		Where("employee_id = ? AND org_id = ? AND status <> ?", agent.ID, *agent.OrgID, "error").
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
	created, err := h.orchestrator.CreateEmployeeSandbox(ctx, agent, secrets)
	if err != nil {
		return nil, err
	}
	if err := employeeruntime.AttachLatestProxyTokenToSandbox(ctx, h.compileDeps, agent, created.ID); err != nil {
		return nil, fmt.Errorf("tag employee proxy token sandbox: %w", err)
	}
	return created, nil
}

func (h *EmployeeHandler) SyncOrgHivyEmployee(ctx context.Context, orgID uuid.UUID) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("employee sandbox sync not configured")
	}
	agent, err := ensureHivyEmployee(ctx, h.db, orgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy employee: %w", err)
	}
	if err := attachEmployeeRequiredSkillsForAgent(ctx, h.db, orgID, agent); err != nil {
		return fmt.Errorf("attach employee required skills: %w", err)
	}
	if _, _, err := h.SyncEmployee(ctx, agent); err != nil {
		return err
	}
	return nil
}

func (h *EmployeeHandler) SyncEmployee(ctx context.Context, agent *model.Employee) (*model.Sandbox, *employeeruntime.SyncResponse, error) {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("employee sandbox sync not configured")
	}
	sb, err := h.ensureEmployeeSandbox(ctx, agent)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure employee sandbox: %w", err)
	}
	resp, err := h.runEmployeeSync(ctx, agent, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("sync employee sandbox: %w", err)
	}
	return sb, resp, nil
}

func (h *EmployeeHandler) runEmployeeSync(ctx context.Context, agent *model.Employee, sb *model.Sandbox) (*employeeruntime.SyncResponse, error) {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime healthz: %w", err)
	}
	proxyToken, err := employeeruntime.MintProxyToken(ctx, h.compileDeps, agent, sb.ID)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	runtimeEnv, err := employeeruntime.BuildRuntimeEnvWithProxyToken(ctx, h.compileDeps, agent, sb, apiKey, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("load runtime env: %w", err)
	}
	def, err := employeeruntime.CompileWithProxyToken(ctx, h.compileDeps, agent, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)

	currentDef, err := client.GetConfig(ctx)
	needsRestart := err != nil || !agentDefinitionsMatch(currentDef, def)

	var resp *employeeruntime.SyncResponse
	if needsRestart {
		resp, err = client.PutRuntimeConfig(ctx, employeeruntime.ConfigUpdateRequest{
			Definition: def,
			RuntimeEnv: runtimeEnv,
		})
	} else {
		resp, err = client.PutRuntimeConfig(ctx, employeeruntime.ConfigUpdateRequest{
			Definition: def,
			RuntimeEnv: runtimeEnv,
		})
	}
	if err != nil {
		return nil, err
	}

	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime readyz: %w", err)
	}
	h.scheduleEmployeeProxyTokenRefresh(ctx, agent, sb)
	if agent.Status != "active" {
		if agent.OrgID == nil {
			return nil, fmt.Errorf("mark employee active: missing org_id")
		}
		if err := h.db.WithContext(ctx).Model(&model.Employee{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return nil, fmt.Errorf("mark employee active: %w", err)
		}
		agent.Status = "active"
	}

	return resp, nil
}

func (h *EmployeeHandler) scheduleExistingEmployeeProxyTokenRefresh(ctx context.Context, agent *model.Employee) {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil {
		return
	}
	var sb model.Sandbox
	err := h.db.WithContext(ctx).
		Where("employee_id = ? AND org_id = ? AND status <> ?", agent.ID, *agent.OrgID, "error").
		Order("created_at DESC").Limit(1).First(&sb).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Capture(ctx, fmt.Errorf("load employee sandbox for proxy token refresh schedule: %w", err))
		}
		return
	}
	h.scheduleEmployeeProxyTokenRefresh(ctx, agent, &sb)
}

func (h *EmployeeHandler) scheduleEmployeeProxyTokenRefresh(ctx context.Context, agent *model.Employee, sb *model.Sandbox) {
	if err := tasks.ScheduleEmployeeProxyTokenRefresh(ctx, h.db, h.enqueuer, agent, sb); err != nil {
		logging.Capture(ctx, fmt.Errorf("schedule employee proxy token refresh: %w", err))
	}
}

func (h *EmployeeHandler) loadRuntimeEnv(ctx context.Context, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
	return employeeruntime.BuildRuntimeEnv(ctx, h.compileDeps, agent, sb, runtimeSecret)
}

func agentDefinitionsMatch(left, right *employeeruntime.AgentDefinition) bool {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false
	}

	var leftDoc any
	var rightDoc any
	if err := json.Unmarshal(leftJSON, &leftDoc); err != nil {
		return false
	}
	if err := json.Unmarshal(rightJSON, &rightDoc); err != nil {
		return false
	}
	return reflect.DeepEqual(leftDoc, rightDoc)
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
