package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

const triggerConversationSource = "trigger"

type EmployeeTriggerDispatchHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	catalog      *catalog.Catalog
}

func NewEmployeeTriggerDispatchHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps) *EmployeeTriggerDispatchHandler {
	return &EmployeeTriggerDispatchHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		catalog:      catalog.Global(),
	}
}

func (h *EmployeeTriggerDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload EmployeeTriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	var webhookPayload map[string]any
	if len(payload.PayloadJSON) > 0 {
		if err := json.Unmarshal(payload.PayloadJSON, &webhookPayload); err != nil {
			return fmt.Errorf("decode trigger payload: %w", err)
		}
	}
	if webhookPayload == nil {
		webhookPayload = map[string]any{}
	}

	triggers, err := h.matchTriggers(ctx, payload)
	if err != nil {
		return err
	}
	for _, trigger := range triggers {
		if ok, reason := triggerConditionsMatch(trigger, webhookPayload); !ok {
			logging.FromContext(ctx).InfoContext(ctx, "employee trigger conditions skipped event",
				"trigger_id", trigger.ID, "agent_id", trigger.AgentID, "reason", reason)
			continue
		}
		if err := h.deliver(ctx, payload, trigger, webhookPayload); err != nil {
			logging.Capture(ctx, fmt.Errorf("deliver employee trigger %s: %w", trigger.ID, err))
			return err
		}
	}
	return nil
}

func (h *EmployeeTriggerDispatchHandler) matchTriggers(ctx context.Context, payload EmployeeTriggerDispatchPayload) ([]model.AgentTrigger, error) {
	if payload.TriggerID != nil {
		var trigger model.AgentTrigger
		if err := h.db.WithContext(ctx).
			Preload("Agent").
			Where("id = ? AND org_id = ? AND enabled = true AND trigger_type = ?", *payload.TriggerID, payload.OrgID, "http").
			First(&trigger).Error; err != nil {
			return nil, fmt.Errorf("load http trigger: %w", err)
		}
		if !trigger.Agent.IsEmployee {
			return nil, fmt.Errorf("trigger owner is not an employee")
		}
		return []model.AgentTrigger{trigger}, nil
	}

	eventKeys := []string{eventKey(payload.EventType, payload.EventAction)}
	if payload.EventAction != "" {
		eventKeys = append(eventKeys, payload.EventType)
	}
	var triggers []model.AgentTrigger
	if err := h.db.WithContext(ctx).
		Joins("JOIN agents ON agents.id = agent_triggers.agent_id").
		Where("agent_triggers.org_id = ? AND agent_triggers.connection_id = ? AND agent_triggers.enabled = true AND agent_triggers.trigger_type = ? AND agents.is_employee = true",
			payload.OrgID, payload.ConnectionID, "webhook").
		Where("agent_triggers.trigger_keys && ?", pq.StringArray(eventKeys)).
		Preload("Agent").
		Find(&triggers).Error; err != nil {
		return nil, fmt.Errorf("find employee triggers: %w", err)
	}
	return triggers, nil
}

func (h *EmployeeTriggerDispatchHandler) deliver(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	agent := trigger.Agent
	if agent.ID == uuid.Nil {
		if err := h.db.WithContext(ctx).Where("id = ? AND is_employee = true", trigger.AgentID).First(&agent).Error; err != nil {
			return fmt.Errorf("load employee: %w", err)
		}
	}
	if agent.OrgID == nil {
		return fmt.Errorf("employee missing org")
	}

	sb, err := h.loadEmployeeSandbox(ctx, agent.ID, *agent.OrgID)
	if err != nil {
		return err
	}
	if strings.EqualFold(sb.Status, string(sandbox.StatusStopped)) {
		if err := h.orchestrator.StartEmployeeSandbox(ctx, sb); err != nil {
			return err
		}
	} else if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			return err
		}
	}

	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return fmt.Errorf("decrypt employee runtime key: %w", err)
	}
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		if err := h.syncRuntime(ctx, &agent, sb, client); err != nil {
			return err
		}
	}

	compiled := h.compileMessage(payload, trigger, webhookPayload)
	conv, err := h.findOrCreateTriggerConversation(ctx, &agent, sb, trigger.ID, compiled.ResourceKey, compiled.ConversationID)
	if err != nil {
		return err
	}

	_, err = client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:            compiled.Text,
		ConversationID:  conv.RuntimeConversationID,
		User:            "hiveloop-trigger",
		UserDisplayName: "Hiveloop Trigger",
		Raw:             compiled.Raw,
	})
	if err != nil {
		return fmt.Errorf("post employee trigger message: %w", err)
	}
	return nil
}

func (h *EmployeeTriggerDispatchHandler) loadEmployeeSandbox(ctx context.Context, agentID, orgID uuid.UUID) (*model.Sandbox, error) {
	var sb model.Sandbox
	if err := h.db.WithContext(ctx).
		Where("agent_id = ? AND org_id = ? AND status <> ?", agentID, orgID, string(sandbox.StatusError)).
		Order("created_at DESC").
		First(&sb).Error; err != nil {
		return nil, fmt.Errorf("load employee sandbox: %w", err)
	}
	return &sb, nil
}

func (h *EmployeeTriggerDispatchHandler) syncRuntime(ctx context.Context, agent *model.Agent, sb *model.Sandbox, client *employeeruntime.Client) error {
	def, err := employeeruntime.Compile(ctx, h.compileDeps, agent)
	if err != nil {
		return fmt.Errorf("compile employee config: %w", err)
	}
	def.OutboundChannels = employeeruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)
	if _, err := client.PutConfig(ctx, def); err != nil {
		return fmt.Errorf("employee runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return fmt.Errorf("employee runtime readyz: %w", err)
	}
	return nil
}

type compiledTriggerMessage struct {
	Text           string
	ResourceKey    string
	ConversationID string
	Raw            map[string]any
}

func (h *EmployeeTriggerDispatchHandler) findOrCreateTriggerConversation(ctx context.Context, agent *model.Agent, sb *model.Sandbox, triggerID uuid.UUID, resourceKey, conversationID string) (*model.AgentConversation, error) {
	var conv model.AgentConversation
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			*agent.OrgID, agent.ID, triggerConversationSource, triggerID, resourceKey, "active").
		First(&conv).Error
	if err == nil {
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load trigger conversation: %w", err)
	}

	conv = model.AgentConversation{
		OrgID:                 *agent.OrgID,
		AgentID:               agent.ID,
		SandboxID:             sb.ID,
		RuntimeConversationID: conversationID,
		Source:                triggerConversationSource,
		SourceID:              &triggerID,
		SourceResourceKey:     resourceKey,
		Status:                "active",
		Name:                  "Trigger: " + resourceKey,
	}
	if err := h.db.WithContext(ctx).Create(&conv).Error; err != nil {
		return nil, fmt.Errorf("create trigger conversation: %w", err)
	}
	return &conv, nil
}

func triggerConditionsMatch(trigger model.AgentTrigger, payload map[string]any) (bool, string) {
	if len(trigger.Conditions) == 0 {
		return true, ""
	}
	var match model.TriggerMatch
	if err := json.Unmarshal(trigger.Conditions, &match); err != nil {
		return false, "invalid condition json"
	}
	reason, ok := dispatch.MatchConditions(&match, payload)
	return ok, reason
}
