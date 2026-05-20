package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// lifecycleHarness sets up the full chain for lifecycle tests.
type lifecycleHarness struct {
	*testHarness
	org     model.Org
	cred    model.Credential
	agent   model.Agent
	sandbox model.Sandbox
	conv    model.AgentConversation
	router  *chi.Mux
}

func newLifecycleHarness(t *testing.T) *lifecycleHarness {
	t.Helper()
	h := newHarness(t)
	suffix := uuid.New().String()[:8]

	org := model.Org{Name: "lifecycle-" + suffix}
	h.db.Create(&org)
	t.Cleanup(func() { h.db.Where("id = ?", org.ID).Delete(&model.Org{}) })

	cred := model.Credential{
		OrgID: org.ID, BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		ProviderID: "openai", EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	h.db.Create(&cred)
	t.Cleanup(func() { h.db.Where("id = ?", cred.ID).Delete(&model.Credential{}) })

	agent := model.Agent{
		OrgID: &org.ID, Name: "lc-agent-" + suffix,
		CredentialID: &cred.ID, SystemPrompt: "test", Model: "gpt-4o",
	}
	h.db.Create(&agent)
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	sandbox := model.Sandbox{
		OrgID:      &org.ID,
		ExternalID: "lc-ext-" + suffix, BridgeURL: "https://test:25434",
		EncryptedBridgeAPIKey: []byte("enc-key"), Status: "running",
	}
	h.db.Create(&sandbox)
	t.Cleanup(func() { h.db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{}) })

	conv := model.AgentConversation{
		OrgID: org.ID, AgentID: agent.ID, SandboxID: sandbox.ID,
		RuntimeConversationID: "lc-conv-" + suffix, Status: "active",
	}
	h.db.Create(&conv)
	t.Cleanup(func() {
		h.db.Where("conversation_id = ?", conv.ID).Delete(&model.ConversationEvent{})
		h.db.Where("id = ?", conv.ID).Delete(&model.AgentConversation{})
	})

	now := time.Now()
	events := []model.ConversationEvent{
		{OrgID: org.ID, ConversationID: conv.ID, EventID: "e1", EventType: "conversation_created", AgentID: "a1", RuntimeConversationID: conv.RuntimeConversationID, Timestamp: now, SequenceNumber: 1, Data: model.RawJSON(`{}`)},
		{OrgID: org.ID, ConversationID: conv.ID, EventID: "e2", EventType: "message_received", AgentID: "a1", RuntimeConversationID: conv.RuntimeConversationID, Timestamp: now, SequenceNumber: 2, Data: model.RawJSON(`{"content":"hello"}`)},
		{OrgID: org.ID, ConversationID: conv.ID, EventID: "e3", EventType: "response_completed", AgentID: "a1", RuntimeConversationID: conv.RuntimeConversationID, Timestamp: now, SequenceNumber: 3, Data: model.RawJSON(`{"content":"hi","usage":{"input_tokens":10}}`)},
		{OrgID: org.ID, ConversationID: conv.ID, EventID: "e4", EventType: "turn_completed", AgentID: "a1", RuntimeConversationID: conv.RuntimeConversationID, Timestamp: now, SequenceNumber: 4, Data: model.RawJSON(`{}`)},
	}
	for i := range events {
		h.db.Create(&events[i])
	}

	convHandler := handler.NewConversationHandler(h.db, nil, nil, nil)
	sandboxHandler := handler.NewSandboxHandler(h.db, nil)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = middleware.WithOrg(r, &org)
			next.ServeHTTP(w, r)
		})
	})
	r.Route("/v1/conversations/{convID}", func(r chi.Router) {
		r.Get("/events", convHandler.ListEvents)
	})
	r.Route("/v1/sandboxes", func(r chi.Router) {
		r.Get("/", sandboxHandler.List)
		r.Get("/{id}", sandboxHandler.Get)
	})

	return &lifecycleHarness{
		testHarness: h,
		org:         org,
		cred:        cred,
		agent:       agent,
		sandbox:     sandbox,
		conv:        conv,
		router:      r,
	}
}

func (lh *lifecycleHarness) request(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rr := httptest.NewRecorder()
	lh.router.ServeHTTP(rr, req)
	return rr
}

func TestListEvents_AllEvents(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/conversations/"+lh.conv.ID.String()+"/events")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data    []json.RawMessage `json:"data"`
		HasMore bool              `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Data) != 4 {
		t.Fatalf("expected 4 events, got %d", len(resp.Data))
	}
}

func TestListEvents_FilterByType(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/conversations/"+lh.conv.ID.String()+"/events?type=message_received")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Data []struct {
			EventType string `json:"event_type"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 message_received event, got %d", len(resp.Data))
	}
	if resp.Data[0].EventType != "message_received" {
		t.Errorf("event_type: got %q", resp.Data[0].EventType)
	}
}

func TestListEvents_NotFound(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/conversations/"+uuid.New().String()+"/events")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestListEvents_IncludesPayload(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/conversations/"+lh.conv.ID.String()+"/events?type=response_completed")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Data []struct {
			EventType string          `json:"event_type"`
			Data      json.RawMessage `json:"data"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Data))
	}
	var data map[string]any
	_ = json.Unmarshal(resp.Data[0].Data, &data)
	if data["content"] != "hi" {
		t.Errorf("content: got %v", data["content"])
	}
}

func TestListSandboxes(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/sandboxes")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Data) < 1 {
		t.Fatal("expected at least 1 sandbox")
	}
	found := false
	for _, s := range resp.Data {
		if s.ID == lh.sandbox.ID.String() {
			found = true
			if s.Status != "running" {
				t.Errorf("status: got %q", s.Status)
			}
		}
	}
	if !found {
		t.Error("sandbox not found in list")
	}
}

func TestListSandboxes_FilterByStatus(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/sandboxes?status=running")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Data []struct{ Status string } `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	for _, s := range resp.Data {
		if s.Status != "running" {
			t.Errorf("expected only running, got %q", s.Status)
		}
	}
}

func TestGetSandbox(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/sandboxes/"+lh.sandbox.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		ExternalID string `json:"external_id"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp.ID != lh.sandbox.ID.String() {
		t.Errorf("id mismatch")
	}
	if resp.Status != "running" {
		t.Errorf("status: got %q", resp.Status)
	}
}

func TestGetSandbox_NotFound(t *testing.T) {
	lh := newLifecycleHarness(t)

	rr := lh.request(t, http.MethodGet, "/v1/sandboxes/"+uuid.New().String())
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}
