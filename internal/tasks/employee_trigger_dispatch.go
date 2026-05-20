package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const triggerConversationSource = "trigger"

func init() {
	RegisterTaskBuilder(TypeEmployeeTriggerDispatch, func(payload []byte) (*asynq.Task, error) {
		var p EmployeeTriggerDispatchPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("unmarshal employee trigger dispatch payload: %w", err)
		}
		return NewEmployeeTriggerDispatchTask(p)
	})
}

type EmployeeTriggerDispatchHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  employeeruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
	catalog      *catalog.Catalog
}

func NewEmployeeTriggerDispatchHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps employeeruntime.CompileDeps, enqueuer ...enqueue.TaskEnqueuer) *EmployeeTriggerDispatchHandler {
	var q enqueue.TaskEnqueuer
	if len(enqueuer) > 0 {
		q = enqueuer[0]
	}
	return &EmployeeTriggerDispatchHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     q,
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
		logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger match failed: %w", err), map[string]any{
			"org_id":      payload.OrgID.String(),
			"delivery_id": payload.DeliveryID,
			"event_key":   eventKey(payload.EventType, payload.EventAction),
		})
		return err
	}
	for _, trigger := range triggers {
		if ok, reason := triggerConditionsMatch(trigger, webhookPayload); !ok {
			logging.FromContext(ctx).InfoContext(ctx, "employee trigger conditions skipped event",
				"trigger_id", trigger.ID, "agent_id", trigger.AgentID, "reason", reason)
			continue
		}
		if err := h.deliver(ctx, payload, trigger, webhookPayload); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("deliver employee trigger %s: %w", trigger.ID, err), map[string]any{
				"org_id":      payload.OrgID.String(),
				"agent_id":    trigger.AgentID.String(),
				"trigger_id":  trigger.ID.String(),
				"delivery_id": payload.DeliveryID,
				"event_key":   eventKey(payload.EventType, payload.EventAction),
			})
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
		return []model.AgentTrigger{trigger}, nil
	}

	eventKeys := []string{eventKey(payload.EventType, payload.EventAction)}
	if payload.EventAction != "" {
		eventKeys = append(eventKeys, payload.EventType)
	}
	var triggers []model.AgentTrigger
	if err := h.db.WithContext(ctx).
		Joins("JOIN employees ON employees.id = employee_triggers.agent_id").
		Where("employee_triggers.org_id = ? AND employee_triggers.connection_id = ? AND employee_triggers.enabled = true AND employee_triggers.trigger_type = ? AND employees.status <> ?",
			payload.OrgID, payload.ConnectionID, "webhook", "archived").
		Where("employee_triggers.trigger_keys && ?", pq.StringArray(eventKeys)).
		Preload("Agent").
		Find(&triggers).Error; err != nil {
		return nil, fmt.Errorf("find employee triggers: %w", err)
	}
	return triggers, nil
}

func (h *EmployeeTriggerDispatchHandler) deliver(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) error {
	agent := trigger.Agent
	if agent.ID == uuid.Nil {
		if err := h.db.WithContext(ctx).Where("id = ? AND status <> ?", trigger.AgentID, "archived").First(&agent).Error; err != nil {
			return fmt.Errorf("load employee: %w", err)
		}
	}
	if agent.OrgID == nil {
		return fmt.Errorf("employee missing org")
	}

	sb, err := h.loadEmployeeSandbox(ctx, agent.ID, *agent.OrgID)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "load_employee_sandbox", payload, trigger, "", "", err)
		return err
	}
	if strings.EqualFold(sb.Status, string(sandbox.StatusStopped)) {
		if err := h.orchestrator.StartEmployeeSandbox(ctx, sb); err != nil {
			captureTriggerDispatchBoundary(ctx, "start_employee_sandbox", payload, trigger, "", "", err)
			return err
		}
	} else if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			captureTriggerDispatchBoundary(ctx, "refresh_employee_sandbox_url", payload, trigger, "", "", err)
			return err
		}
	}

	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "decrypt_employee_runtime_key", payload, trigger, "", "", err)
		return fmt.Errorf("decrypt employee runtime key: %w", err)
	}
	client := employeeruntime.NewClient(sb.BridgeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		captureTriggerDispatchBoundary(ctx, "employee_runtime_healthz", payload, trigger, "", "", err)
		return fmt.Errorf("employee runtime healthz: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		if err := h.syncRuntime(ctx, &agent, sb, client); err != nil {
			captureTriggerDispatchBoundary(ctx, "employee_runtime_readyz_sync", payload, trigger, "", "", err)
			return err
		}
	}

	compiled := h.compileMessage(payload, trigger, webhookPayload)
	conv, err := h.findOrCreateTriggerConversation(ctx, &agent, sb, trigger.ID, compiled.ResourceKey, compiled.ConversationID)
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "find_or_create_trigger_conversation", payload, trigger, compiled.ResourceKey, "", err)
		return err
	}

	resp, err := client.PostHTTPMessage(ctx, employeeruntime.HTTPMessageRequest{
		Text:            compiled.Text,
		ConversationID:  conv.RuntimeConversationID,
		User:            "hiveloop-trigger",
		UserDisplayName: "Hiveloop Trigger",
		Raw:             compiled.Raw,
	})
	if err != nil {
		captureTriggerDispatchBoundary(ctx, "post_http_message", payload, trigger, compiled.ResourceKey, conv.ID.String(), err)
		return fmt.Errorf("post employee trigger message: %w", err)
	}
	h.enqueueStoreDelivery(ctx, payload, trigger, conv, compiled, resp)
	return nil
}

func (h *EmployeeTriggerDispatchHandler) enqueueStoreDelivery(ctx context.Context, payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, conv *model.AgentConversation, compiled compiledTriggerMessage, resp *employeeruntime.HTTPMessageResponse) {
	if h.enqueuer == nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger delivery store enqueue skipped: enqueuer is nil"), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
		return
	}
	if resp == nil {
		resp = &employeeruntime.HTTPMessageResponse{}
	}
	task, err := NewEmployeeTriggerStoreDeliveryTask(EmployeeTriggerStoreDeliveryPayload{
		OrgID:                 trigger.OrgID,
		AgentID:               trigger.AgentID,
		TriggerID:             trigger.ID,
		ConnectionID:          trigger.ConnectionID,
		DeliveryID:            payload.DeliveryID,
		EventKey:              eventKey(payload.EventType, payload.EventAction),
		ResourceKey:           compiled.ResourceKey,
		ConversationID:        conv.ID,
		RuntimeConversationID: conv.RuntimeConversationID,
		RuntimeSessionID:      resp.SessionID,
		RuntimeStreamID:       resp.StreamID,
		RuntimeTraceID:        resp.TraceID,
		RuntimeTurnID:         resp.TurnID,
		PayloadJSON:           payload.PayloadJSON,
	})
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("build employee trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
		return
	}
	if _, err := h.enqueuer.EnqueueContext(ctx, task); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("enqueue employee trigger delivery store task: %w", err), triggerStoreEnqueueFields(payload, trigger, conv, compiled, resp))
	}
}

func triggerStoreEnqueueFields(payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, conv *model.AgentConversation, compiled compiledTriggerMessage, resp *employeeruntime.HTTPMessageResponse) map[string]any {
	fields := map[string]any{
		"org_id":                  trigger.OrgID.String(),
		"agent_id":                trigger.AgentID.String(),
		"trigger_id":              trigger.ID.String(),
		"delivery_id":             payload.DeliveryID,
		"event_key":               eventKey(payload.EventType, payload.EventAction),
		"resource_key":            compiled.ResourceKey,
		"runtime_conversation_id": "",
		"runtime_session_id":      "",
	}
	if conv != nil {
		fields["conversation_id"] = conv.ID.String()
		fields["runtime_conversation_id"] = conv.RuntimeConversationID
	}
	if resp != nil {
		fields["runtime_session_id"] = resp.SessionID
	}
	return fields
}

func captureTriggerDispatchBoundary(ctx context.Context, stage string, payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, resourceKey, conversationID string, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee trigger dispatch %s: %w", stage, err), map[string]any{
		"stage":           stage,
		"org_id":          payload.OrgID.String(),
		"agent_id":        trigger.AgentID.String(),
		"trigger_id":      trigger.ID.String(),
		"delivery_id":     payload.DeliveryID,
		"event_key":       eventKey(payload.EventType, payload.EventAction),
		"resource_key":    resourceKey,
		"conversation_id": conversationID,
	})
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
