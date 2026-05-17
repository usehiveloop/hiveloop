package handler

import (
	"encoding/json"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/streaming"
)

// ConversationHandler proxies conversation operations to Bridge.
type ConversationHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	pusher       *sandbox.Pusher
	eventBus     *streaming.EventBus // nil = use legacy Bridge SSE proxy
	enqueuer     enqueue.TaskEnqueuer
	credits      *billing.CreditsService // nil disables credit gating (useful in tests)
}

func NewConversationHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, pusher *sandbox.Pusher, eventBus *streaming.EventBus) *ConversationHandler {
	return &ConversationHandler{db: db, orchestrator: orchestrator, pusher: pusher, eventBus: eventBus}
}

func (h *ConversationHandler) SetEnqueuer(enqueuer enqueue.TaskEnqueuer) {
	h.enqueuer = enqueuer
}

// SetCredits wires the credit ledger. When set, Create deducts one credit per
// conversation and rejects with 402 when the balance is exhausted.
func (h *ConversationHandler) SetCredits(credits *billing.CreditsService) {
	h.credits = credits
}

type conversationResponse struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status"`
	StreamURL string `json:"stream_url"`
	CreatedAt string `json:"created_at"`
}

type conversationEventResponse struct {
	ID                   string          `json:"id"`
	EventID              string          `json:"event_id"`
	EventType            string          `json:"event_type"`
	AgentID              string          `json:"agent_id"`
	RuntimeConversationID string          `json:"runtime_conversation_id"`
	Timestamp            string          `json:"timestamp"`
	SequenceNumber       int64           `json:"sequence_number"`
	Data                 json.RawMessage `json:"data"`
	CreatedAt            string          `json:"created_at"`
}

// conversationHistoryResponse is the payload shape for GET /conversations/{id}/history.
// It returns events in chronological order so the frontend can render them
// directly before opening the SSE stream.
type conversationHistoryResponse struct {
	ConversationID string                      `json:"conversation_id"`
	Events         []conversationEventResponse `json:"events"`
	// LastEventID is the event_id of the last event in this page. Clients
	// can pass it as the `Last-Event-ID` header when opening the SSE stream
	// to resume from exactly where history ended.
	LastEventID string `json:"last_event_id,omitempty"`
	HasMore     bool   `json:"has_more"`
}

type conversationToolCallResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

type conversationToolGroupResponse struct {
	Name  string                         `json:"name"`
	Calls []conversationToolCallResponse `json:"calls"`
}

type conversationMessageResponse struct {
	ID         string                          `json:"id"`
	Author     string                          `json:"author"`
	Timestamp  string                          `json:"timestamp"`
	Body       string                          `json:"body,omitempty"`
	ToolGroups []conversationToolGroupResponse `json:"tool_groups,omitempty"`
}

type conversationTodoItem struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
}

type conversationMessagesResponse struct {
	Data        []conversationMessageResponse `json:"data"`
	LatestTodos []conversationTodoItem        `json:"latest_todos,omitempty"`
	NextCursor  *string                       `json:"next_cursor,omitempty"`
	HasMore     bool                          `json:"has_more"`
}
