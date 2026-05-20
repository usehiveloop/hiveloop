// Package fakebridge implements a scriptable HTTP server that speaks the
// new ACP-harness bridge wire contract for hivy's e2e tests. The real
// bridge binary is not yet released against this contract, so we mock it.
package fakebridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	bridgepkg "github.com/usehivy/hivy/internal/bridge"
)

type BridgeEvent struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	AgentID        string          `json:"agent_id"`
	ConversationID string          `json:"conversation_id"`
	Timestamp      time.Time       `json:"timestamp"`
	SequenceNumber int64           `json:"sequence_number"`
	Data           json.RawMessage `json:"data"`
}

type CreatedConversation struct {
	AgentID        string
	ConversationID string
	Body           json.RawMessage
}

type SentMessage struct {
	ConversationID string
	Body           json.RawMessage
	Content        string
}

type ApprovalCall struct {
	AgentID        string
	ConversationID string
	RequestID      string
	Body           json.RawMessage
	Decision       string
}

type CancelCall struct {
	ConversationID string
	Kind           string
}

type Captured struct {
	UpsertAgents        []bridgepkg.AgentDefinition
	UpsertAgentsRaw     [][]byte
	CreateConversations []CreatedConversation
	Messages            []SentMessage
	Approvals           []ApprovalCall
	Cancels             []CancelCall
	PendingApprovals    []bridgepkg.ApprovalRequest
}

type Server struct {
	URL     string
	Handler *http.ServeMux

	ScriptedSSE func(agentID, convID string) []BridgeEvent

	// PushOnMessage, when true, calls ScriptedSSE on every POST /messages
	// and queues the result for delivery on the open SSE connection.
	PushOnMessage bool

	// EnforceOneAgent rejects a second UpsertAgent with a different ID
	// (mirrors the bridge's one-agent-per-instance constraint).
	EnforceOneAgent bool

	// SignSecret must match the EncryptedBridgeAPIKey decrypted on the
	// hivy side so PostWebhook signatures verify.
	SignSecret []byte

	WebhookURL string

	srv *httptest.Server

	mu       sync.Mutex
	captured Captured
	queues   map[string][]BridgeEvent
	loaded   map[string]bool
}

func New(t *testing.T) *Server {
	t.Helper()
	mux := http.NewServeMux()
	s := &Server{
		Handler: mux,
		queues:  make(map[string][]BridgeEvent),
		loaded:  make(map[string]bool),
	}
	s.registerRoutes()
	s.srv = httptest.NewServer(mux)
	s.URL = s.srv.URL
	t.Cleanup(s.Close)
	return s
}

func (s *Server) Close() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

func (s *Server) CapturedSnapshot() Captured {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := s.captured
	cp.UpsertAgents = append([]bridgepkg.AgentDefinition(nil), s.captured.UpsertAgents...)
	cp.UpsertAgentsRaw = append([][]byte(nil), s.captured.UpsertAgentsRaw...)
	cp.CreateConversations = append([]CreatedConversation(nil), s.captured.CreateConversations...)
	cp.Messages = append([]SentMessage(nil), s.captured.Messages...)
	cp.Approvals = append([]ApprovalCall(nil), s.captured.Approvals...)
	cp.Cancels = append([]CancelCall(nil), s.captured.Cancels...)
	cp.PendingApprovals = append([]bridgepkg.ApprovalRequest(nil), s.captured.PendingApprovals...)
	return cp
}

func (s *Server) SetPendingApprovals(approvals []bridgepkg.ApprovalRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captured.PendingApprovals = append([]bridgepkg.ApprovalRequest(nil), approvals...)
}

func (s *Server) QueueEvents(convID string, events []BridgeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queues[convID] = append(s.queues[convID], events...)
}

func (s *Server) drainQueue(convID string) []BridgeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queues[convID]
	delete(s.queues, convID)
	return q
}
