package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestListApprovals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/agent-1/conversations/conv-1/approvals" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]ApprovalRequest{
			{Id: "req-1", ToolName: "bash"},
			{Id: "req-2", ToolName: "write"},
		})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	approvals, err := c.ListApprovals(context.Background(), "agent-1", "conv-1")
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(approvals) != 2 {
		t.Fatalf("expected 2 approvals, got %d", len(approvals))
	}
	if approvals[0].ToolName != "bash" {
		t.Errorf("approval 0 tool: got %q", approvals[0].ToolName)
	}
}

func TestResolveApproval(t *testing.T) {
	var receivedDecision ApprovalDecision
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/a/conversations/c/approvals/req-1" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		var req ApprovalReply
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedDecision = req.Decision
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.ResolveApproval(context.Background(), "a", "c", "req-1", ApprovalDecisionApprove)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if receivedDecision != ApprovalDecisionApprove {
		t.Errorf("decision: got %q", receivedDecision)
	}
}

func TestRotateAPIKey(t *testing.T) {
	var receivedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method: got %q", r.Method)
		}
		if r.URL.Path != "/push/agents/agent-1/api-key" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		var body struct {
			APIKey string `json:"api_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedKey = body.APIKey
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.RotateAPIKey(context.Background(), "agent-1", "ptok_new_token")
	if err != nil {
		t.Fatalf("RotateAPIKey: %v", err)
	}
	if receivedKey != "ptok_new_token" {
		t.Errorf("received key: got %q", receivedKey)
	}
}

func TestSSEStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv-1/stream" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("accept header: got %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("Authorization") != "Bearer stream-key" {
			t.Errorf("auth header: got %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		events := []string{
			"data: {\"type\":\"message_start\"}\n\n",
			"data: {\"type\":\"content_delta\",\"delta\":\"Hello\"}\n\n",
			"data: {\"type\":\"done\"}\n\n",
		}
		for _, e := range events {
			_, _ = w.Write([]byte(e))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "stream-key")
	body, err := c.SSEStream(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("SSEStream: %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("reading SSE: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "message_start") {
		t.Error("expected message_start event")
	}
	if !strings.Contains(content, "content_delta") {
		t.Error("expected content_delta event")
	}
	if !strings.Contains(content, "done") {
		t.Error("expected done event")
	}
}

func TestErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"agent_not_found","message":"agent not found"}}`))
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")

	err := c.UpsertAgent(context.Background(), "missing", AgentDefinition{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should contain status code: %v", err)
	}

	_, err = c.CreateConversation(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}

	err = c.SendMessage(context.Background(), "missing", "hello")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAuthHeaderSentOnAllRequests(t *testing.T) {
	var mu sync.Mutex
	authHeaders := make(map[string]string)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaders[r.Method+" "+r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "secret-bearer")
	ctx := context.Background()

	_ = c.HealthCheck(ctx)
	_ = c.UpsertAgent(ctx, "a", AgentDefinition{})
	_ = c.RemoveAgentDefinition(ctx, "a")
	_ = c.SendMessage(ctx, "c", "hi")
	_ = c.AbortConversation(ctx, "c")
	_ = c.EndConversation(ctx, "c")

	mu.Lock()
	defer mu.Unlock()

	for endpoint, auth := range authHeaders {
		if auth != "Bearer secret-bearer" {
			t.Errorf("%s: auth header = %q, want %q", endpoint, auth, "Bearer secret-bearer")
		}
	}

	if len(authHeaders) < 6 {
		t.Errorf("expected at least 6 endpoints called, got %d", len(authHeaders))
	}
}

func TestGetMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(MetricsResponse{
			Global: GlobalMetrics{
				TotalAgents:              3,
				TotalActiveConversations: 5,
				UptimeSecs:               120,
			},
		})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	metrics, err := c.GetMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if metrics.Global.TotalAgents != 3 {
		t.Errorf("total agents: got %d", metrics.Global.TotalAgents)
	}
	if metrics.Global.TotalActiveConversations != 5 {
		t.Errorf("active conversations: got %d", metrics.Global.TotalActiveConversations)
	}
	if metrics.Global.UptimeSecs != 120 {
		t.Errorf("uptime: got %d", metrics.Global.UptimeSecs)
	}
}
