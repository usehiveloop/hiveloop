package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_ChatCreate_HappyPath(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "hello there"})
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		SessionID string `json:"session_id"`
		StreamURL string `json:"stream_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SessionID == "" {
		t.Errorf("session_id missing")
	}
	if !strings.HasPrefix(resp.StreamURL, "/v1/chats/") {
		t.Errorf("stream_url shape = %q", resp.StreamURL)
	}
	if !strings.Contains(resp.StreamURL, "?token=") {
		t.Errorf("stream_url missing token: %q", resp.StreamURL)
	}

	var msgs []model.ChatMessage
	h.db.Where("session_id = ?", resp.SessionID).Find(&msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello there" {
		t.Errorf("first message wrong: %+v", msgs[0])
	}
}

func TestIntegration_ChatCreate_RejectsEmployeeFromOtherOrg(t *testing.T) {
	h, _ := newChatHarness(t)
	owner := h.seedOrgAgentSandbox(t)
	stranger := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, stranger, "POST",
		"/v1/employees/"+owner.agent.ID.String()+"/chats",
		map[string]string{"message": "hi"})
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org access: status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_ChatCreate_RejectsEmptyMessage(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "  "})
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestIntegration_ChatSend_AppendsToExistingSession(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "first"})
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var first struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &first)

	rr2 := httptest.NewRecorder()
	req2 := h.authedReq(t, m, "POST",
		"/v1/chats/"+first.SessionID+"/messages",
		map[string]string{"message": "second"})
	h.router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("send: %d %s", rr2.Code, rr2.Body.String())
	}

	var msgs []model.ChatMessage
	h.db.Where("session_id = ?", first.SessionID).Order("created_at ASC").Find(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Content != "second" {
		t.Errorf("second message wrong: %q", msgs[1].Content)
	}
}

func TestIntegration_ChatSend_RejectsOtherUsersSession(t *testing.T) {
	h, _ := newChatHarness(t)
	owner := h.seedOrgAgentSandbox(t)
	stranger := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, owner, "POST",
		"/v1/employees/"+owner.agent.ID.String()+"/chats",
		map[string]string{"message": "private"})
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d", rr.Code)
	}
	var first struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &first)

	rr2 := httptest.NewRecorder()
	req2 := h.authedReq(t, stranger, "POST",
		"/v1/chats/"+first.SessionID+"/messages",
		map[string]string{"message": "snooping"})
	h.router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr2.Code)
	}
}
