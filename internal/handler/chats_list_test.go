package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestIntegration_ChatList_ReturnsCallerSessionsOnly(t *testing.T) {
	h, _ := newChatHarness(t)
	owner := h.seedOrgAgentSandbox(t)
	stranger := h.seedOrgAgentSandbox(t)

	for _, txt := range []string{"alpha", "beta"} {
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, h.authedReq(t, owner, "POST",
			"/v1/employees/"+owner.agent.ID.String()+"/chats",
			map[string]string{"message": txt}))
		if rr.Code != http.StatusCreated {
			t.Fatalf("create: %d", rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, h.authedReq(t, owner, "GET", "/v1/chats", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rr.Code, rr.Body.String())
	}
	var listed struct {
		Data []struct {
			ID      string `json:"id"`
			AgentID string `json:"agent_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &listed)
	if len(listed.Data) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(listed.Data))
	}

	rr2 := httptest.NewRecorder()
	h.router.ServeHTTP(rr2, h.authedReq(t, stranger, "GET", "/v1/chats", nil))
	var strangerListed struct {
		Data []any `json:"data"`
	}
	_ = json.Unmarshal(rr2.Body.Bytes(), &strangerListed)
	if len(strangerListed.Data) != 0 {
		t.Errorf("stranger should see 0 sessions, got %d", len(strangerListed.Data))
	}
}

func TestIntegration_ChatGet_ReturnsMessages(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "history check"}))
	var resp struct {
		SessionID string `json:"session_id"`
		StreamURL string `json:"stream_url"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	streamRR := httptest.NewRecorder()
	h.router.ServeHTTP(streamRR, httptest.NewRequest("GET", resp.StreamURL, nil))
	if streamRR.Code != http.StatusOK {
		t.Fatalf("stream: %d", streamRR.Code)
	}

	getRR := httptest.NewRecorder()
	h.router.ServeHTTP(getRR, h.authedReq(t, m, "GET", "/v1/chats/"+resp.SessionID, nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("get: %d %s", getRR.Code, getRR.Body.String())
	}
	var detail struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(getRR.Body.Bytes(), &detail)
	if detail.Session.ID != resp.SessionID {
		t.Errorf("session id mismatch")
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(detail.Messages))
	}
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Errorf("roles wrong: %+v", detail.Messages)
	}
}

func TestIntegration_ChatGet_NotFound(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, h.authedReq(t, m, "GET", "/v1/chats/"+uuid.NewString(), nil))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}
