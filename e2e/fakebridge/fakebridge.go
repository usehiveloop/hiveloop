// Package fakebridge implements a scriptable HTTP server that speaks the
// new ACP-harness bridge wire contract. It is consumed by hiveloop's
// Wave 3 e2e tests so we can exercise the full ingest/egress pipeline
// (Postgres + Redis + regenerated client + new pusher + new subagent
// path) without spinning up a real bridge process — the bridge binary
// is not yet released against this contract.
//
// Routes implemented:
//
//	PUT  /push/agents/{id}                                   — capture UpsertAgent
//	POST /agents/{id}/conversations                          — capture, return {conversation_id}
//	POST /conversations/{cid}/messages                       — capture, optionally stream scripted SSE back
//	GET  /conversations/{cid}/stream                         — SSE; emit pending scripted events
//	GET  /agents/{id}/conversations/{cid}/approvals          — return pending approvals
//	POST /agents/{id}/conversations/{cid}/approvals/{rid}    — capture decision
//	POST /conversations/{cid}/abort                          — capture cancel
//	DELETE /conversations/{cid}                              — capture end
//	GET  /agents/{id}                                        — 404 (forces UpsertAgent)
//	GET  /health                                             — 200
//
// Webhook delivery (PostWebhook) signs with HMAC-SHA256 over
// "{timestamp}.{payload}" using SignSecret, matching the verification
// logic in internal/handler/bridge_webhooks.go::verifyWebhookSignature.
package fakebridge

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// BridgeEvent is a single SSE/webhook event matching the wire shape the
// hiveloop bridge_webhooks handler expects (see webhookEvent in
// internal/handler/bridge_webhooks.go).
type BridgeEvent struct {
	EventID        string          `json:"event_id"`
	EventType      string          `json:"event_type"`
	AgentID        string          `json:"agent_id"`
	ConversationID string          `json:"conversation_id"`
	Timestamp      time.Time       `json:"timestamp"`
	SequenceNumber int64           `json:"sequence_number"`
	Data           json.RawMessage `json:"data"`
}

// CreatedConversation captures one POST /agents/{id}/conversations call.
type CreatedConversation struct {
	AgentID        string
	ConversationID string
	Body           json.RawMessage
}

// SentMessage captures one POST /conversations/{cid}/messages call.
type SentMessage struct {
	ConversationID string
	Body           json.RawMessage
	Content        string
}

// ApprovalCall captures one POST /agents/{id}/conversations/{cid}/approvals/{rid} call.
type ApprovalCall struct {
	AgentID        string
	ConversationID string
	RequestID      string
	Body           json.RawMessage
	Decision       string
}

// CancelCall captures POST /conversations/{cid}/abort or DELETE /conversations/{cid}.
type CancelCall struct {
	ConversationID string
	Kind           string // "abort" or "end"
}

// Captured records every interesting request the fake bridge has seen.
// All fields are guarded by the parent Server's mu mutex.
type Captured struct {
	UpsertAgents        []bridgepkg.AgentDefinition
	UpsertAgentsRaw     [][]byte
	CreateConversations []CreatedConversation
	Messages            []SentMessage
	Approvals           []ApprovalCall
	Cancels             []CancelCall
	PendingApprovals    []bridgepkg.ApprovalRequest
}

// Server is the scriptable fake bridge.
type Server struct {
	URL     string
	Handler *http.ServeMux

	// ScriptedSSE is invoked the first time SSE is requested for a
	// (agentID, convID) pair. The returned events are emitted in order.
	// On message POST, if PushOnMessage is true, those events are also
	// pushed into the pending queue so the next SSE drain delivers them.
	ScriptedSSE func(agentID, convID string) []BridgeEvent

	// PushOnMessage, when true, calls ScriptedSSE on every POST /messages
	// and queues the result for delivery on the open SSE connection.
	PushOnMessage bool

	// Forced409Agent, if set, will cause the second UpsertAgent with a
	// different ID to return 409 (mirrors bridge's one-agent-per-instance
	// constraint). The first ID seen wins.
	EnforceOneAgent bool

	// SignSecret is the HMAC secret used by PostWebhook. Must match the
	// sandbox.EncryptedBridgeAPIKey decrypted value on the hiveloop side.
	SignSecret []byte

	// WebhookURL is the hiveloop endpoint to POST webhook batches to. Set
	// after constructing the Server (see PostWebhook).
	WebhookURL string

	srv *httptest.Server

	mu       sync.Mutex
	captured Captured
	queues   map[string][]BridgeEvent // convID -> pending SSE events
	loaded   map[string]bool          // agentID -> upserted
}

// New starts an httptest.Server with the bridge routes registered.
// The caller is responsible for calling Close when done.
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

// Close shuts down the underlying httptest server.
func (s *Server) Close() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
	}
}

// Captured returns a snapshot of the recorded requests. The returned
// struct's slices are aliases of the live storage — callers should hold
// the lock if they want to mutate; for read-only inspection in tests,
// call this after the activity quiesces.
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

// SetPendingApprovals seeds the GET approvals response.
func (s *Server) SetPendingApprovals(approvals []bridgepkg.ApprovalRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.captured.PendingApprovals = append([]bridgepkg.ApprovalRequest(nil), approvals...)
}

// QueueEvents pushes additional scripted events to be delivered on the
// next SSE read for (convID).
func (s *Server) QueueEvents(convID string, events []BridgeEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queues[convID] = append(s.queues[convID], events...)
}

// drainQueue pops all queued events for convID.
func (s *Server) drainQueue(convID string) []BridgeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.queues[convID]
	delete(s.queues, convID)
	return q
}

func (s *Server) registerRoutes() {
	// PUT /push/agents/{id}  — UpsertAgent
	// GET /agents/{id}       — HasAgent (we always 404 to force the upsert)
	s.Handler.HandleFunc("/push/agents/", s.handleUpsertAgent)
	s.Handler.HandleFunc("/agents/", s.handleAgentsRouter)

	// POST/DELETE conversations & messaging
	s.Handler.HandleFunc("/conversations/", s.handleConversationsRouter)

	// Health
	s.Handler.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","uptime_secs":1}`))
	})
}

func (s *Server) handleUpsertAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimPrefix(r.URL.Path, "/push/agents/")
	if agentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.EnforceOneAgent {
		// reject if a different agent ID has already been upserted
		for existing := range s.loaded {
			if existing != agentID {
				s.mu.Unlock()
				http.Error(w, `{"error":"only one agent per instance"}`, http.StatusConflict)
				return
			}
		}
	}

	var def bridgepkg.AgentDefinition
	if err := json.Unmarshal(body, &def); err == nil {
		s.captured.UpsertAgents = append(s.captured.UpsertAgents, def)
	}
	s.captured.UpsertAgentsRaw = append(s.captured.UpsertAgentsRaw, append([]byte(nil), body...))
	s.loaded[agentID] = true
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleAgentsRouter dispatches /agents/* paths (HasAgent, CreateConversation,
// approvals).
func (s *Server) handleAgentsRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/agents/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	agentID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		// HasAgent — pretend not loaded so the pusher always pushes.
		w.WriteHeader(http.StatusNotFound)
		return

	case len(parts) == 2 && parts[1] == "conversations" && r.Method == http.MethodPost:
		s.handleCreateConversation(w, r, agentID)
		return

	case len(parts) == 4 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodGet:
		s.handleListApprovals(w, r, agentID, parts[2])
		return

	case len(parts) == 5 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodPost:
		s.handleResolveApproval(w, r, agentID, parts[2], parts[4])
		return

	case len(parts) == 4 && parts[1] == "conversations" && parts[3] == "approvals" && r.Method == http.MethodPost:
		// bulk approvals — not exercised by current e2e tests; ignore
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"resolved":[],"not_found":[]}`))
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request, agentID string) {
	body, _ := io.ReadAll(r.Body)
	convID := fmt.Sprintf("conv-%d", time.Now().UnixNano())
	s.mu.Lock()
	s.captured.CreateConversations = append(s.captured.CreateConversations, CreatedConversation{
		AgentID:        agentID,
		ConversationID: convID,
		Body:           append(json.RawMessage(nil), body...),
	})
	s.mu.Unlock()

	resp := bridgepkg.CreateConversationResponse{
		ConversationId: convID,
		StreamUrl:      s.URL + "/conversations/" + convID + "/stream",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request, agentID, convID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if s.captured.PendingApprovals == nil {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	_ = json.NewEncoder(w).Encode(s.captured.PendingApprovals)
}

func (s *Server) handleResolveApproval(w http.ResponseWriter, r *http.Request, agentID, convID, requestID string) {
	body, _ := io.ReadAll(r.Body)
	var reply struct {
		Decision string `json:"decision"`
	}
	_ = json.Unmarshal(body, &reply)

	s.mu.Lock()
	s.captured.Approvals = append(s.captured.Approvals, ApprovalCall{
		AgentID:        agentID,
		ConversationID: convID,
		RequestID:      requestID,
		Body:           append(json.RawMessage(nil), body...),
		Decision:       reply.Decision,
	})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"request_id":%q,"status":"resolved"}`, requestID)))
}

// handleConversationsRouter dispatches /conversations/{cid}/* paths.
func (s *Server) handleConversationsRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/conversations/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	convID := parts[0]

	switch {
	case len(parts) == 1 && r.Method == http.MethodDelete:
		s.mu.Lock()
		s.captured.Cancels = append(s.captured.Cancels, CancelCall{ConversationID: convID, Kind: "end"})
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ended"}`))
		return

	case len(parts) == 2 && parts[1] == "abort" && r.Method == http.MethodPost:
		s.mu.Lock()
		s.captured.Cancels = append(s.captured.Cancels, CancelCall{ConversationID: convID, Kind: "abort"})
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return

	case len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodPost:
		s.handleSendMessage(w, r, convID)
		return

	case len(parts) == 2 && parts[1] == "stream" && r.Method == http.MethodGet:
		s.handleSSEStream(w, r, convID)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request, convID string) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(body, &req)

	s.mu.Lock()
	s.captured.Messages = append(s.captured.Messages, SentMessage{
		ConversationID: convID,
		Body:           append(json.RawMessage(nil), body...),
		Content:        req.Content,
	})
	s.mu.Unlock()

	if s.PushOnMessage && s.ScriptedSSE != nil {
		// Determine the agent ID for the scripted callback. Fall back to
		// the most-recent CreateConversation entry that mapped to this
		// convID; otherwise pass empty string.
		agentID := ""
		s.mu.Lock()
		for _, cc := range s.captured.CreateConversations {
			if cc.ConversationID == convID {
				agentID = cc.AgentID
				break
			}
		}
		s.mu.Unlock()
		events := s.ScriptedSSE(agentID, convID)
		s.QueueEvents(convID, events)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

// handleSSEStream emits whatever scripted events are queued for convID,
// then closes. Subscribers can call /stream again after queueing more
// events to drain another batch.
func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request, convID string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// First drain: events queued before the connection opened.
	events := s.drainQueue(convID)

	// If nothing queued and ScriptedSSE is set, materialize it once.
	if len(events) == 0 && s.ScriptedSSE != nil {
		// Look up agentID from CreateConversations.
		agentID := ""
		s.mu.Lock()
		for _, cc := range s.captured.CreateConversations {
			if cc.ConversationID == convID {
				agentID = cc.AgentID
				break
			}
		}
		s.mu.Unlock()
		events = s.ScriptedSSE(agentID, convID)
	}

	for _, ev := range events {
		dataBytes, _ := json.Marshal(ev.Data)
		// Bridge SSE format: `event: <type>\ndata: <json>\n\n`.
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType, string(dataBytes))
		if flusher != nil {
			flusher.Flush()
		}
	}

	// Wait briefly for any event queued during the message turn to land.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		more := s.drainQueue(convID)
		for _, ev := range more {
			dataBytes, _ := json.Marshal(ev.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType, string(dataBytes))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if len(more) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// PostWebhook simulates the bridge → hiveloop webhook delivery: signs the
// payload and POSTs it to s.WebhookURL. Returns the response status code
// and body.
func (s *Server) PostWebhook(t *testing.T, events []BridgeEvent) (int, []byte) {
	t.Helper()
	if s.WebhookURL == "" {
		t.Fatal("fakebridge: WebhookURL not configured")
	}
	if len(s.SignSecret) == 0 {
		t.Fatal("fakebridge: SignSecret not configured")
	}

	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}

	timestamp := time.Now().Unix()
	message := fmt.Sprintf("%d.%s", timestamp, string(body))
	mac := hmac.New(sha256.New, s.SignSecret)
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", signature)
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", timestamp))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}

// PostWebhookUnsigned posts a webhook batch without a signature header
// (or with a deliberately wrong signature when wrongSig != ""). Used by
// tests that exercise the rejection path.
func (s *Server) PostWebhookUnsigned(t *testing.T, events []BridgeEvent, wrongSig string) (int, []byte) {
	t.Helper()
	if s.WebhookURL == "" {
		t.Fatal("fakebridge: WebhookURL not configured")
	}
	body, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if wrongSig != "" {
		req.Header.Set("X-Webhook-Signature", wrongSig)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook POST: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody
}
