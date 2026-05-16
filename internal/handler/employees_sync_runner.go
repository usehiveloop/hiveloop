package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

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
		Where("agent_id = ? AND org_id = ? AND status <> ?", agent.ID, *agent.OrgID, "error").
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
	runtimeEnv, err := h.loadRuntimeEnv(agent)
	if err != nil {
		return nil, fmt.Errorf("load runtime env: %w", err)
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

	currentDef, err := client.GetConfig(ctx)
	needsRestart := err != nil || !agentDefinitionsMatch(currentDef, def)

	var resp *employeeruntime.SyncResponse
	if needsRestart {
		resp, err = client.PutConfig(ctx, def)
	} else {
		resp, err = client.UpdateRuntimeEnv(ctx, runtimeEnv)
	}
	if err != nil {
		return nil, err
	}

	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("employee runtime readyz: %w", err)
	}
	if agent.Status != "active" {
		if agent.OrgID == nil {
			return nil, fmt.Errorf("mark employee active: missing org_id")
		}
		if err := h.db.WithContext(ctx).Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return nil, fmt.Errorf("mark employee active: %w", err)
		}
		agent.Status = "active"
	}

	return resp, nil
}

func (h *EmployeeHandler) loadRuntimeEnv(agent *model.Agent) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil || len(agent.EncryptedEnvVars) == 0 {
		return env, nil
	}

	decrypted, err := h.compileDeps.EncKey.DecryptString(agent.EncryptedEnvVars)
	if err != nil {
		return nil, err
	}
	decrypted = strings.TrimSpace(decrypted)
	if decrypted == "" {
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
	return env, nil
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
