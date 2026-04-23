package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TriggerDispatchPayload carries everything the dispatcher needs to decide
// which agents should run for an incoming webhook. The connection is encoded
// by ID — the worker reloads it from the DB so we don't carry secrets across
// the queue boundary.
//
// PayloadJSON is the raw webhook body as bytes (not parsed) so the worker can
// log/replay it verbatim. The dispatcher decodes it on demand.
type TriggerDispatchPayload struct {
	Provider        string     `json:"provider"`
	EventType       string     `json:"event_type"`
	EventAction     string     `json:"event_action"`
	DeliveryID      string     `json:"delivery_id"`
	OrgID           uuid.UUID  `json:"org_id"`
	ConnectionID    uuid.UUID  `json:"connection_id"`
	PayloadJSON     []byte     `json:"payload"`
	RouterTriggerID *uuid.UUID `json:"router_trigger_id,omitempty"` // set to bypass trigger matching (http/cron triggers)
}

// NewTriggerDispatchTask creates a task that runs the dispatcher for a webhook.
//
// MaxRetry is intentionally low (3) — dispatch is fast and any error here is
// either a transient DB issue (worth a retry) or a programmer error (more
// retries don't help). Long timeouts are unnecessary; the dispatch step is
// pure CPU + one DB query, so 30s is generous.
func NewTriggerDispatchTask(payload TriggerDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeTriggerDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
	), nil
}

// NewRouterDispatchTask creates a task that runs the Zira router dispatcher
// for a webhook event. Same payload shape as TriggerDispatchTask — the router
// dispatcher reads the same fields. Timeout is higher (5 minutes) because the
// triage LLM call adds latency.
func NewRouterDispatchTask(payload TriggerDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal router dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeRouterDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(5*time.Minute),
	), nil
}

// AgentConversationCreatePayload carries everything needed to create a sandbox,
// push the agent to Bridge, create a conversation, and send the first message.
type AgentConversationCreatePayload struct {
	AgentID         uuid.UUID `json:"agent_id"`
	OrgID           uuid.UUID `json:"org_id"`
	DeliveryID      string    `json:"delivery_id"`
	ConnectionID    uuid.UUID `json:"connection_id"`
	RouterTriggerID uuid.UUID `json:"router_trigger_id"`
	ResourceKey     string    `json:"resource_key"`
	RouterPersona   string    `json:"router_persona,omitempty"`
	MemoryTeam      string    `json:"memory_team,omitempty"`
	Instructions    string    `json:"instructions"`
}

// NewAgentConversationCreateTask creates a task that provisions a sandbox,
// pushes the agent definition, creates a Bridge conversation, and sends
// the enriched instructions as the first message.
//
// Timeout is 5 minutes — sandbox creation can take 30-60s, plus Bridge push
// and health check. MaxRetry is 1 — sandbox creation is not idempotent.
func NewAgentConversationCreateTask(payload AgentConversationCreatePayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal agent conversation create payload: %w", err)
	}
	return asynq.NewTask(
		TypeAgentConversationCreate,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute),
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

// SubscriptionDispatchPayload carries the info needed to forward a webhook
// event into every conversation that has an active subscription matching
// the event's resource. Shape intentionally mirrors TriggerDispatchPayload
// — if they stay aligned we can eventually share a single payload type.
type SubscriptionDispatchPayload struct {
	Provider     string    `json:"provider"`
	EventType    string    `json:"event_type"`
	EventAction  string    `json:"event_action"`
	DeliveryID   string    `json:"delivery_id"`
	OrgID        uuid.UUID `json:"org_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
	PayloadJSON  []byte    `json:"payload"`
}

// NewSubscriptionDispatchTask creates a task that resolves the event's
// resource_key and forwards the event into every matching active
// conversation_subscription.
//
// Queue is Critical because delivery latency is user-visible: agents expect
// events promptly. MaxRetry 3 handles transient Bridge SendMessage failures.
// Timeout 2 minutes covers the worst case: wake a sleeping sandbox + N
// parallel Bridge SendMessage calls for a fanned-out event.
//
// asynq.Unique with the delivery_id as the basis deduplicates redelivered
// webhooks — Nango occasionally re-sends, and we don't want the agent to
// see the same event twice.
func NewSubscriptionDispatchTask(payload SubscriptionDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal subscription dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeSubscriptionDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
		asynq.Unique(2*time.Minute),
	), nil
}

// ---------------------------------------------------------------------------
// cron_trigger:dispatch
// ---------------------------------------------------------------------------

// CronTriggerDispatchPayload carries the trigger ID and scheduled fire time
// for a cron trigger that is due. The handler loads the trigger from the DB,
// evaluates its routing rules, and enqueues agent conversation creation.
type CronTriggerDispatchPayload struct {
	RouterTriggerID uuid.UUID `json:"router_trigger_id"`
	OrgID           uuid.UUID `json:"org_id"`
	ScheduledAt     time.Time `json:"scheduled_at"` // the intended fire time (NextRunAt before advancement)
}

// NewCronTriggerDispatchTask creates a task that dispatches a single cron trigger.
// Timeout is 5 minutes to accommodate triage LLM calls. MaxRetry is 1 —
// duplicate fires are worse than a missed one.
func NewCronTriggerDispatchTask(payload CronTriggerDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal cron trigger dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeCronTriggerDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(1),
		asynq.Timeout(5*time.Minute),
	), nil
}
