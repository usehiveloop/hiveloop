package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func init() {
	RegisterTaskBuilder(TypeEmployeeTriggerStoreDelivery, func(payload []byte) (*asynq.Task, error) {
		var p EmployeeTriggerStoreDeliveryPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("unmarshal employee trigger store delivery payload: %w", err)
		}
		return NewEmployeeTriggerStoreDeliveryTask(p)
	})
}

type EmployeeTriggerStoreDeliveryPayload struct {
	OrgID                 uuid.UUID  `json:"org_id"`
	AgentID               uuid.UUID  `json:"agent_id"`
	TriggerID             uuid.UUID  `json:"trigger_id"`
	ConnectionID          *uuid.UUID `json:"connection_id,omitempty"`
	DeliveryID            string     `json:"delivery_id"`
	EventKey              string     `json:"event_key"`
	ResourceKey           string     `json:"resource_key"`
	ConversationID        uuid.UUID  `json:"conversation_id"`
	RuntimeConversationID string     `json:"runtime_conversation_id"`
	RuntimeSessionID      string     `json:"runtime_session_id"`
	RuntimeStreamID       string     `json:"runtime_stream_id"`
	RuntimeTraceID        string     `json:"runtime_trace_id"`
	RuntimeTurnID         string     `json:"runtime_turn_id"`
	PayloadJSON           []byte     `json:"payload"`
}

func NewEmployeeTriggerStoreDeliveryTask(payload EmployeeTriggerStoreDeliveryPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal employee trigger store delivery payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmployeeTriggerStoreDelivery,
		encoded,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
	), nil
}

type EmployeeTriggerStoreDeliveryHandler struct {
	db *gorm.DB
}

func NewEmployeeTriggerStoreDeliveryHandler(db *gorm.DB) *EmployeeTriggerStoreDeliveryHandler {
	return &EmployeeTriggerStoreDeliveryHandler{db: db}
}

func (h *EmployeeTriggerStoreDeliveryHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload EmployeeTriggerStoreDeliveryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	raw := payload.PayloadJSON
	if len(raw) == 0 {
		raw = []byte("{}")
	}

	row := model.AgentTriggerDelivery{
		OrgID:                 payload.OrgID,
		AgentID:               payload.AgentID,
		TriggerID:             payload.TriggerID,
		ConnectionID:          payload.ConnectionID,
		DeliveryID:            payload.DeliveryID,
		EventKey:              payload.EventKey,
		ResourceKey:           payload.ResourceKey,
		ConversationID:        payload.ConversationID,
		RuntimeConversationID: payload.RuntimeConversationID,
		RuntimeSessionID:      payload.RuntimeSessionID,
		RuntimeStreamID:       payload.RuntimeStreamID,
		RuntimeTraceID:        payload.RuntimeTraceID,
		RuntimeTurnID:         payload.RuntimeTurnID,
		Payload:               model.RawJSON(raw),
	}
	if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("store employee trigger delivery: %w", err), triggerDeliverySentryFields(payload))
		return fmt.Errorf("store employee trigger delivery: %w", err)
	}
	return nil
}

func triggerDeliverySentryFields(payload EmployeeTriggerStoreDeliveryPayload) map[string]any {
	return map[string]any{
		"org_id":                  payload.OrgID.String(),
		"agent_id":                payload.AgentID.String(),
		"trigger_id":              payload.TriggerID.String(),
		"delivery_id":             payload.DeliveryID,
		"event_key":               payload.EventKey,
		"resource_key":            payload.ResourceKey,
		"conversation_id":         payload.ConversationID.String(),
		"runtime_conversation_id": payload.RuntimeConversationID,
		"runtime_session_id":      payload.RuntimeSessionID,
	}
}
