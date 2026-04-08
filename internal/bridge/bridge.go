package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BridgeClient communicates with a Bridge runtime instance.
type BridgeClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewBridgeClient creates a client for communicating with a Bridge instance.
func NewBridgeClient(baseURL, apiKey string) *BridgeClient {
	return &BridgeClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *BridgeClient) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func doJSON[T any](c *BridgeClient, ctx context.Context, method, path string, body any) (*T, error) {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bridge API error (status %d): %s", resp.StatusCode, b)
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

func doVoid(c *BridgeClient, ctx context.Context, method, path string, body any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bridge API error (status %d): %s", resp.StatusCode, b)
	}
	return nil
}

// --- Agent management (push endpoints) ---

// PushAgents bulk-loads agent definitions into Bridge.
func (c *BridgeClient) PushAgents(ctx context.Context, agents []AgentDefinition) error {
	payload := struct {
		Agents []AgentDefinition `json:"agents"`
	}{Agents: agents}
	return doVoid(c, ctx, http.MethodPost, "/push/agents", payload)
}

// UpsertAgent creates or updates a single agent definition.
func (c *BridgeClient) UpsertAgent(ctx context.Context, agentID string, def AgentDefinition) error {
	return doVoid(c, ctx, http.MethodPut, "/push/agents/"+agentID, def)
}

// HasAgent checks if Bridge already has this agent loaded.
func (c *BridgeClient) HasAgent(ctx context.Context, agentID string) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/agents/"+agentID, nil)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// RemoveAgentDefinition removes an agent from Bridge.
func (c *BridgeClient) RemoveAgentDefinition(ctx context.Context, agentID string) error {
	return doVoid(c, ctx, http.MethodDelete, "/push/agents/"+agentID, nil)
}

// RotateAPIKey rotates an agent's LLM provider API key without downtime.
func (c *BridgeClient) RotateAPIKey(ctx context.Context, agentID string, newKey string) error {
	payload := struct {
		APIKey string `json:"api_key"`
	}{APIKey: newKey}
	return doVoid(c, ctx, http.MethodPatch, "/push/agents/"+agentID+"/api-key", payload)
}

// HydrateConversations pushes conversation history to an agent.
func (c *BridgeClient) HydrateConversations(ctx context.Context, agentID string, conversations []ConversationRecord) error {
	payload := HydrateConversationsRequest{Conversations: conversations}
	return doVoid(c, ctx, http.MethodPost, "/push/agents/"+agentID+"/conversations", payload)
}

// --- Conversation operations ---

// CreateConversationRequest is the optional request body for creating a conversation.
type CreateConversationRequest struct {
	// ApiKey overrides the agent's LLM API key for this conversation only.
	ApiKey string `json:"api_key,omitempty"`
}

// CreateConversation creates a new conversation for an agent.
func (c *BridgeClient) CreateConversation(ctx context.Context, agentID string) (*CreateConversationResponse, error) {
	return doJSON[CreateConversationResponse](c, ctx, http.MethodPost, "/agents/"+agentID+"/conversations", nil)
}

// CreateConversationWithAPIKey creates a new conversation with a per-conversation API key override.
// Used for system agents that don't have their own credential — the proxy token is passed here.
func (c *BridgeClient) CreateConversationWithAPIKey(ctx context.Context, agentID, apiKey string) (*CreateConversationResponse, error) {
	payload := CreateConversationRequest{ApiKey: apiKey}
	return doJSON[CreateConversationResponse](c, ctx, http.MethodPost, "/agents/"+agentID+"/conversations", payload)
}

// SendMessage sends a message to a conversation (async, returns 202).
func (c *BridgeClient) SendMessage(ctx context.Context, convID string, content string) error {
	payload := SendMessageRequest{Content: content}
	return doVoid(c, ctx, http.MethodPost, "/conversations/"+convID+"/messages", payload)
}

// SSEStream opens a raw SSE connection to a conversation stream.
// The caller is responsible for closing the returned ReadCloser.
func (c *BridgeClient) SSEStream(ctx context.Context, convID string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/conversations/"+convID+"/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client without timeout for SSE (long-lived connection)
	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to SSE stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("SSE stream returned status %d: %s", resp.StatusCode, body)
	}

	return resp.Body, nil
}

// AbortConversation aborts the current turn in a conversation.
func (c *BridgeClient) AbortConversation(ctx context.Context, convID string) error {
	return doVoid(c, ctx, http.MethodPost, "/conversations/"+convID+"/abort", nil)
}

// EndConversation ends a conversation permanently.
func (c *BridgeClient) EndConversation(ctx context.Context, convID string) error {
	return doVoid(c, ctx, http.MethodDelete, "/conversations/"+convID, nil)
}

// --- Approval operations ---

// ListApprovals lists pending approval requests for a conversation.
func (c *BridgeClient) ListApprovals(ctx context.Context, agentID, convID string) ([]ApprovalRequest, error) {
	result, err := doJSON[[]ApprovalRequest](c, ctx, http.MethodGet, "/agents/"+agentID+"/conversations/"+convID+"/approvals", nil)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

// ResolveApproval resolves a single approval request.
func (c *BridgeClient) ResolveApproval(ctx context.Context, agentID, convID, requestID string, decision ApprovalDecision) error {
	payload := ApprovalReply{Decision: decision}
	return doVoid(c, ctx, http.MethodPost, "/agents/"+agentID+"/conversations/"+convID+"/approvals/"+requestID, payload)
}

// BulkResolveApprovals resolves multiple approval requests at once.
func (c *BridgeClient) BulkResolveApprovals(ctx context.Context, agentID, convID string, reply BulkApprovalReply) error {
	return doVoid(c, ctx, http.MethodPost, "/agents/"+agentID+"/conversations/"+convID+"/approvals", reply)
}

// --- Health & metrics ---

// HealthCheck checks if Bridge is healthy.
func (c *BridgeClient) HealthCheck(ctx context.Context) error {
	return doVoid(c, ctx, http.MethodGet, "/health", nil)
}

// GetMetrics retrieves metrics from all agents.
func (c *BridgeClient) GetMetrics(ctx context.Context) (*MetricsResponse, error) {
	return doJSON[MetricsResponse](c, ctx, http.MethodGet, "/metrics", nil)
}
