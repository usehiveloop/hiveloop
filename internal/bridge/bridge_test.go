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

func TestNewBridgeClient(t *testing.T) {
	c := NewBridgeClient("https://example.com", "test-key")
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL: got %q", c.baseURL)
	}
	if c.apiKey != "test-key" {
		t.Errorf("apiKey: got %q", c.apiKey)
	}
}

func TestHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header: got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "test-key")
	if err := c.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	if err := c.HealthCheck(context.Background()); err == nil {
		t.Fatal("expected error for unhealthy bridge")
	}
}

func TestUpsertAgent(t *testing.T) {
	var received AgentDefinition
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method: got %q", r.Method)
		}
		if r.URL.Path != "/push/agents/agent-123" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type: got %q", r.Header.Get("Content-Type"))
		}

		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")

	def := AgentDefinition{
		Id:           "agent-123",
		Name:         "Test Agent",
		SystemPrompt: "You are helpful",
		Provider: ProviderConfig{
			Model:        "gpt-4o",
			ProviderType: OpenAi,
			ApiKey:       "ptok_test",
		},
	}

	err := c.UpsertAgent(context.Background(), "agent-123", def)
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}

	if received.Name != "Test Agent" {
		t.Errorf("received name: got %q", received.Name)
	}
}

func TestRemoveAgentDefinition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method: got %q", r.Method)
		}
		if r.URL.Path != "/push/agents/agent-456" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.RemoveAgentDefinition(context.Background(), "agent-456")
	if err != nil {
		t.Fatalf("RemoveAgentDefinition: %v", err)
	}
}

func TestCreateConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q", r.Method)
		}
		if r.URL.Path != "/agents/agent-123/conversations" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateConversationResponse{
			ConversationId: "conv-abc",
			StreamUrl:      "/conversations/conv-abc/stream",
		})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	resp, err := c.CreateConversation(context.Background(), "agent-123")
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if resp.ConversationId != "conv-abc" {
		t.Errorf("conversation_id: got %q", resp.ConversationId)
	}
	if resp.StreamUrl != "/conversations/conv-abc/stream" {
		t.Errorf("stream_url: got %q", resp.StreamUrl)
	}
}

func TestCreateConversationWithOptions_SerialisesMcpServers(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(CreateConversationResponse{
			ConversationId: "conv-mcp",
			StreamUrl:      "/conversations/conv-mcp/stream",
		})
	}))
	defer srv.Close()

	headers := map[string]string{"Authorization": "Bearer tok-123"}
	var transport McpTransport
	transport.FromMcpTransport1(McpTransport1{
		Type:    StreamableHttp,
		Url:     "https://mcp.example.com/forge-context/run-42",
		Headers: &headers,
	})

	client := NewBridgeClient(srv.URL, "key")
	resp, err := client.CreateConversationWithOptions(context.Background(), "agent-1", CreateConversationRequest{
		Provider: &ConversationProviderOverride{
			ProviderType: Anthropic,
			Model:        "claude-sonnet-4-6",
			ApiKey:       "sk-test",
		},
		McpServers: []McpServerDefinition{
			{Name: "forge-context", Transport: transport},
		},
	})
	if err != nil {
		t.Fatalf("CreateConversationWithOptions: %v", err)
	}
	if resp.ConversationId != "conv-mcp" {
		t.Errorf("conversation_id: got %q", resp.ConversationId)
	}

	// Verify both provider and mcp_servers were serialised into the request body.
	if receivedBody["provider"] == nil {
		t.Error("expected provider in request body")
	}
	mcpServers, ok := receivedBody["mcp_servers"].([]any)
	if !ok || len(mcpServers) != 1 {
		t.Fatalf("expected 1 mcp_server in request body, got %v", receivedBody["mcp_servers"])
	}
	first := mcpServers[0].(map[string]any)
	if first["name"] != "forge-context" {
		t.Errorf("mcp_servers[0].name: got %q, want forge-context", first["name"])
	}
}

func TestSendMessage(t *testing.T) {
	var receivedContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv-abc/messages" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		var req SendMessageRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedContent = req.Content
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.SendMessage(context.Background(), "conv-abc", "Hello Bridge!")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if receivedContent != "Hello Bridge!" {
		t.Errorf("received content: got %q", receivedContent)
	}
}

func TestAbortConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv-abc/abort" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "aborted"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.AbortConversation(context.Background(), "conv-abc")
	if err != nil {
		t.Fatalf("AbortConversation: %v", err)
	}
}

func TestEndConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method: got %q", r.Method)
		}
		if r.URL.Path != "/conversations/conv-abc" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ended"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.EndConversation(context.Background(), "conv-abc")
	if err != nil {
		t.Fatalf("EndConversation: %v", err)
	}
}

func TestListApprovals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/agent-1/conversations/conv-1/approvals" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]ApprovalRequest{
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
		json.NewDecoder(r.Body).Decode(&req)
		receivedDecision = req.Decision
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
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
		json.NewDecoder(r.Body).Decode(&body)
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
			w.Write([]byte(e))
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
		w.Write([]byte(`{"error":{"code":"agent_not_found","message":"agent not found"}}`))
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")

	// All methods should return errors on 4xx
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
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "secret-bearer")
	ctx := context.Background()

	c.HealthCheck(ctx)
	c.UpsertAgent(ctx, "a", AgentDefinition{})
	c.RemoveAgentDefinition(ctx, "a")
	c.SendMessage(ctx, "c", "hi")
	c.AbortConversation(ctx, "c")
	c.EndConversation(ctx, "c")

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
		json.NewEncoder(w).Encode(MetricsResponse{
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

