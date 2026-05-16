package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// EmployeeTriggerDispatchPayload carries one inbound trigger event to the
// employee trigger dispatcher. For webhook triggers, TriggerID is empty and the
// worker matches all employee triggers for the connection/event. For HTTP
// triggers, TriggerID is set and the worker delivers only that trigger.
type EmployeeTriggerDispatchPayload struct {
	Provider     string     `json:"provider,omitempty"`
	EventType    string     `json:"event_type,omitempty"`
	EventAction  string     `json:"event_action,omitempty"`
	DeliveryID   string     `json:"delivery_id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ConnectionID uuid.UUID  `json:"connection_id,omitempty"`
	TriggerID    *uuid.UUID `json:"trigger_id,omitempty"`
	PayloadJSON  []byte     `json:"payload"`
}

func NewEmployeeTriggerDispatchTask(payload EmployeeTriggerDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal employee trigger dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmployeeTriggerDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}

// ConversationNamePayload is the payload for TypeConversationName tasks.
// The worker loads everything else (conversation, agent, credential, first
// message) from the DB — we only need the ID.
type ConversationNamePayload struct {
	ConversationID uuid.UUID `json:"conversation_id"`
}

// NewConversationNameTask creates a task that generates a title for a
// conversation by calling the cheapest model available to the conversation's
// credential provider. Bulk queue — this is nice-to-have UX, not critical
// path. MaxRetry is 3: transient provider failures are common and the
// handler is idempotent (refuses to overwrite an already-set name).
func NewConversationNameTask(conversationID uuid.UUID) (*asynq.Task, error) {
	encoded, err := json.Marshal(ConversationNamePayload{ConversationID: conversationID})
	if err != nil {
		return nil, fmt.Errorf("marshal conversation name payload: %w", err)
	}
	return asynq.NewTask(
		TypeConversationName,
		encoded,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
		asynq.Unique(5*time.Minute),
	), nil
}
