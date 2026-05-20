package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
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

		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created"})
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
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "removed"})
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
		_ = json.NewEncoder(w).Encode(CreateConversationResponse{
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
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreateConversationResponse{
			ConversationId: "conv-mcp",
			StreamUrl:      "/conversations/conv-mcp/stream",
		})
	}))
	defer srv.Close()

	headers := map[string]string{"Authorization": "Bearer tok-123"}
	var transport McpTransport
	_ = transport.FromMcpTransport1(McpTransport1{
		Type:    StreamableHttp,
		Url:     "https://mcp.example.com/test-mcp/run-42",
		Headers: &headers,
	})

	client := NewBridgeClient(srv.URL, "key")
	providerOverride := ProviderConfig{
		ProviderType: Anthropic,
		Model:        "claude-sonnet-4-6",
		ApiKey:       "sk-test",
	}
	resp, err := client.CreateConversationWithOptions(context.Background(), "agent-1", CreateConversationRequest{
		Provider: &providerOverride,
		McpServers: &[]McpServerDefinition{
			{Name: "test-mcp", Transport: transport},
		},
	})
	if err != nil {
		t.Fatalf("CreateConversationWithOptions: %v", err)
	}
	if resp.ConversationId != "conv-mcp" {
		t.Errorf("conversation_id: got %q", resp.ConversationId)
	}

	if receivedBody["provider"] == nil {
		t.Error("expected provider in request body")
	}
	mcpServers, ok := receivedBody["mcp_servers"].([]any)
	if !ok || len(mcpServers) != 1 {
		t.Fatalf("expected 1 mcp_server in request body, got %v", receivedBody["mcp_servers"])
	}
	first := mcpServers[0].(map[string]any)
	if first["name"] != "test-mcp" {
		t.Errorf("mcp_servers[0].name: got %q, want test-mcp", first["name"])
	}
}

func TestSendMessage(t *testing.T) {
	var receivedContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/conversations/conv-abc/messages" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		var req SendMessageRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Content != nil {
			receivedContent = *req.Content
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
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
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "aborted"})
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
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ended"})
	}))
	defer srv.Close()

	c := NewBridgeClient(srv.URL, "key")
	err := c.EndConversation(context.Background(), "conv-abc")
	if err != nil {
		t.Fatalf("EndConversation: %v", err)
	}
}
