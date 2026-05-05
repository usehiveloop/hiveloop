package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_ChatStream_TeesAndPersists(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "are you online?"})
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		SessionID string `json:"session_id"`
		StreamURL string `json:"stream_url"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	streamRR := httptest.NewRecorder()
	streamReq := httptest.NewRequest("GET", resp.StreamURL, nil)
	h.router.ServeHTTP(streamRR, streamReq)

	if streamRR.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body=%s", streamRR.Code, streamRR.Body.String())
	}
	if ct := streamRR.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := streamRR.Body.String()
	if !strings.Contains(body, `"Hello"`) {
		t.Errorf("body missing first delta: %q", body)
	}
	if !strings.Contains(body, `[DONE]`) {
		t.Errorf("body missing [DONE] terminator")
	}

	calls, sentBody := h.stub.snapshot()
	if calls != 1 {
		t.Errorf("sidecar called %d times, want 1", calls)
	}
	var sent struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(sentBody, &sent); err != nil {
		t.Fatalf("decode sidecar body: %v", err)
	}
	if sent.Model != "hermes-agent" || !sent.Stream {
		t.Errorf("sidecar body wrong: model=%s stream=%v", sent.Model, sent.Stream)
	}
	if len(sent.Messages) != 1 || sent.Messages[0].Role != "user" || sent.Messages[0].Content != "are you online?" {
		t.Errorf("sent messages wrong: %+v", sent.Messages)
	}

	var msgs []model.ChatMessage
	h.db.Where("session_id = ?", resp.SessionID).Order("created_at ASC").Find(&msgs)
	if len(msgs) != 2 {
		t.Fatalf("expected user + assistant, got %d", len(msgs))
	}
	asst := msgs[1]
	if asst.Role != "assistant" || asst.Content != "Hello world" {
		t.Errorf("assistant row wrong: role=%s content=%q", asst.Role, asst.Content)
	}
	if asst.HermesResponseID != "resp_test_1" {
		t.Errorf("response id = %q, want resp_test_1", asst.HermesResponseID)
	}

	var session model.ChatSession
	h.db.Where("id = ?", resp.SessionID).First(&session)
	if session.LastResponseID != "resp_test_1" {
		t.Errorf("session.last_response_id = %q", session.LastResponseID)
	}
}

func TestIntegration_ChatStream_RecoversFromHermesDown(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	chatCalls := 0
	restartCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/hermes/status":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"state":"running","pid":1}`))
		case "/v1/hermes/restart":
			restartCalls++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"restarted":true}`))
		case "/v1/chat/completions":
			chatCalls++
			if chatCalls == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"error":"dial tcp 127.0.0.1:8642: connect: connection refused"}`))
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`data: {"id":"resp_recover","choices":[{"delta":{"content":"recovered"}}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	expires := time.Now().Add(1 * time.Hour)
	if err := h.db.Model(&model.Sandbox{}).Where("id = ?", m.sb.ID).
		Updates(map[string]any{
			"bridge_url":            srv.URL,
			"bridge_url_expires_at": expires,
		}).Error; err != nil {
		t.Fatalf("rebind sandbox url: %v", err)
	}

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "hello"})
	h.router.ServeHTTP(rr, req)
	var resp struct {
		StreamURL string `json:"stream_url"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	streamRR := httptest.NewRecorder()
	h.router.ServeHTTP(streamRR, httptest.NewRequest("GET", resp.StreamURL, nil))
	if streamRR.Code != http.StatusOK {
		t.Fatalf("stream: %d %s", streamRR.Code, streamRR.Body.String())
	}
	if !strings.Contains(streamRR.Body.String(), "recovered") {
		t.Errorf("body missing recovered content: %s", streamRR.Body.String())
	}
	if restartCalls != 1 {
		t.Errorf("restart calls = %d, want 1", restartCalls)
	}
	if chatCalls != 2 {
		t.Errorf("chat calls = %d, want 2 (1 fail + 1 retry)", chatCalls)
	}
}

func TestIntegration_ChatStream_RejectsBadToken(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr := httptest.NewRecorder()
	req := h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "hi"})
	h.router.ServeHTTP(rr, req)
	var resp struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)

	streamRR := httptest.NewRecorder()
	streamReq := httptest.NewRequest("GET",
		"/v1/chats/"+resp.SessionID+"/stream?token=garbage", nil)
	h.router.ServeHTTP(streamRR, streamReq)
	if streamRR.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", streamRR.Code)
	}
}

func TestIntegration_ChatStream_RejectsTokenForDifferentSession(t *testing.T) {
	h, _ := newChatHarness(t)
	m := h.seedOrgAgentSandbox(t)

	rr1 := httptest.NewRecorder()
	h.router.ServeHTTP(rr1, h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "first"}))
	var first struct {
		SessionID string `json:"session_id"`
		StreamURL string `json:"stream_url"`
	}
	_ = json.Unmarshal(rr1.Body.Bytes(), &first)

	rr2 := httptest.NewRecorder()
	h.router.ServeHTTP(rr2, h.authedReq(t, m, "POST",
		"/v1/employees/"+m.agent.ID.String()+"/chats",
		map[string]string{"message": "second"}))
	var second struct{ SessionID string `json:"session_id"` }
	_ = json.Unmarshal(rr2.Body.Bytes(), &second)

	tokenIdx := strings.Index(first.StreamURL, "token=")
	tok := first.StreamURL[tokenIdx+len("token="):]
	mismatched := "/v1/chats/" + second.SessionID + "/stream?token=" + tok

	streamRR := httptest.NewRecorder()
	h.router.ServeHTTP(streamRR, httptest.NewRequest("GET", mismatched, nil))
	if streamRR.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", streamRR.Code)
	}
}
