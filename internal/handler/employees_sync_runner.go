package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/config"
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

func (h *EmployeeHandler) runEmployeeSync(ctx context.Context, agent *model.Employee, sb *model.Sandbox) (*employeeruntime.SyncResponse, error) {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	def, err := employeeruntime.Compile(ctx, h.compileDeps, agent)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime healthz: %w", err)
	}
	runtimeEnv, err := h.loadRuntimeEnv(ctx, agent, apiKey)
	if err != nil {
		return nil, fmt.Errorf("load runtime env: %w", err)
	}

	currentDef, err := client.GetConfig(ctx)
	needsRestart := err != nil || !agentDefinitionsMatch(currentDef, def)

	var resp *employeeruntime.SyncResponse
	if needsRestart {
		resp, err = client.PutConfig(ctx, def)
		if err == nil {
			_, err = client.UpdateRuntimeEnv(ctx, runtimeEnv)
		}
	} else {
		resp, err = client.UpdateRuntimeEnv(ctx, runtimeEnv)
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

func (h *EmployeeHandler) loadRuntimeEnv(ctx context.Context, agent *model.Employee, runtimeSecret string) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if len(agent.EncryptedEnvVars) == 0 {
		addControlPlaneRuntimeEnv(ctx, h.db, env, h.compileDeps.Cfg, agent, runtimeSecret)
		return env, nil
	}

	decrypted, err := h.compileDeps.EncKey.DecryptString(agent.EncryptedEnvVars)
	if err != nil {
		return nil, err
	}
	decrypted = strings.TrimSpace(decrypted)
	if decrypted == "" {
		addControlPlaneRuntimeEnv(ctx, h.db, env, h.compileDeps.Cfg, agent, runtimeSecret)
		return env, nil
	}

	rawEnv := map[string]string{}
	if err := json.Unmarshal([]byte(decrypted), &rawEnv); err != nil {
		return nil, fmt.Errorf("decode env vars: %w", err)
	}
	for key, value := range rawEnv {
		if strings.HasPrefix(strings.ToUpper(key), "BRIDGE_") {
			continue
		}
		env[key] = value
	}
	addControlPlaneRuntimeEnv(ctx, h.db, env, h.compileDeps.Cfg, agent, runtimeSecret)
	return env, nil
}

func addControlPlaneRuntimeEnv(ctx context.Context, db *gorm.DB, env map[string]string, cfg *config.Config, agent *model.Employee, runtimeSecret string) {
	if env == nil || cfg == nil || agent == nil || agent.ID == uuid.Nil || runtimeSecret == "" {
		return
	}
	bridgeHost := strings.TrimSpace(cfg.SpecialistSandboxHost)
	if bridgeHost == "" {
		return
	}
	env[employeeruntime.EmployeeEnvBugsinkURL] = fmt.Sprintf("https://%s/internal/bugsink-proxy/%s", bridgeHost, agent.ID)
	if agent.OrgID != nil {
		env[employeeruntime.EmployeeEnvBugsinkDashboardBaseURL] = employeeruntime.BugsinkDashboardBaseURL(ctx, db, *agent.OrgID, *agent)
	}
	env[employeeruntime.EmployeeEnvBugsinkToken] = runtimeSecret
	env[employeeruntime.EmployeeEnvLinearURL] = fmt.Sprintf("https://%s/internal/linear-proxy/%s", bridgeHost, agent.ID)
	env[employeeruntime.EmployeeEnvLinearToken] = runtimeSecret
	env[employeeruntime.EmployeeEnvNotionAPIURL] = fmt.Sprintf("https://%s/internal/notion-proxy/%s", bridgeHost, agent.ID)
	env[employeeruntime.EmployeeEnvNotionToken] = runtimeSecret
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
