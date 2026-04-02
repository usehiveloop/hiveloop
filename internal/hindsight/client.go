package hindsight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for the Hindsight REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Hindsight API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// --- Request/Response Types ---

// RetainItem is a single item to retain in Hindsight memory.
type RetainItem struct {
	Content    string            `json:"content"`
	Context    string            `json:"context,omitempty"`
	DocumentID string            `json:"document_id,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Timestamp  string            `json:"timestamp,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// RetainRequest is the request body for POST /v1/default/banks/{bankID}/memories.
type RetainRequest struct {
	Items []RetainItem `json:"items"`
	Async bool         `json:"async"`
}

// RetainResponse is the response from the retain endpoint.
type RetainResponse struct {
	Success     bool   `json:"success"`
	BankID      string `json:"bank_id"`
	ItemsCount  int    `json:"items_count"`
	Async       bool   `json:"async"`
	OperationID string `json:"operation_id,omitempty"`
}

// BankConfigUpdate is the request body for PATCH /v1/default/banks/{bankID}/config.
type BankConfigUpdate struct {
	Updates map[string]any `json:"updates"`
}

// CreateMentalModelRequest is the request body for POST /v1/default/banks/{bankID}/mental-models.
type CreateMentalModelRequest struct {
	Name        string              `json:"name"`
	SourceQuery string              `json:"source_query"`
	Trigger     *MentalModelTrigger `json:"trigger,omitempty"`
}

// MentalModelTrigger controls when a mental model auto-refreshes.
type MentalModelTrigger struct {
	RefreshAfterConsolidation bool `json:"refresh_after_consolidation"`
}

// RecallRequest is the request body for POST /v1/default/banks/{bankID}/memories/recall.
type RecallRequest struct {
	Query     string `json:"query"`
	Budget    string `json:"budget,omitempty"`    // "low", "mid", "high"
	TagGroups []any  `json:"tag_groups,omitempty"` // complex tag filters
}

// RecallResponse is the response from the recall endpoint.
type RecallResponse struct {
	Results  []any          `json:"results"`
	Entities map[string]any `json:"entities,omitempty"`
}

// ReflectRequest is the request body for POST /v1/default/banks/{bankID}/reflect.
type ReflectRequest struct {
	Query     string `json:"query"`
	Budget    string `json:"budget,omitempty"`
	TagGroups []any  `json:"tag_groups,omitempty"`
}

// ReflectResponse is the response from the reflect endpoint.
type ReflectResponse struct {
	Text string `json:"text"`
}

// --- API Methods ---

// Retain stores memories in a Hindsight bank.
func (c *Client) Retain(ctx context.Context, bankID string, req *RetainRequest) (*RetainResponse, error) {
	path := fmt.Sprintf("/v1/default/banks/%s/memories", bankID)
	resp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hindsight retain: status %d: %s", resp.StatusCode, string(body))
	}

	var result RetainResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hindsight retain: decoding response: %w", err)
	}
	return &result, nil
}

// Recall searches memories in a Hindsight bank.
func (c *Client) Recall(ctx context.Context, bankID string, req *RecallRequest) (*RecallResponse, error) {
	path := fmt.Sprintf("/v1/default/banks/%s/memories/recall", bankID)
	resp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hindsight recall: status %d: %s", resp.StatusCode, string(body))
	}

	var result RecallResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hindsight recall: decoding response: %w", err)
	}
	return &result, nil
}

// Reflect generates a reasoned answer from a Hindsight bank's memories.
func (c *Client) Reflect(ctx context.Context, bankID string, req *ReflectRequest) (*ReflectResponse, error) {
	path := fmt.Sprintf("/v1/default/banks/%s/reflect", bankID)
	resp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hindsight reflect: status %d: %s", resp.StatusCode, string(body))
	}

	var result ReflectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hindsight reflect: decoding response: %w", err)
	}
	return &result, nil
}

// ConfigureBank updates the configuration for a Hindsight bank.
func (c *Client) ConfigureBank(ctx context.Context, bankID string, cfg *BankConfigUpdate) error {
	path := fmt.Sprintf("/v1/default/banks/%s/config", bankID)
	resp, err := c.do(ctx, http.MethodPatch, path, cfg)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hindsight configure bank: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// CreateMentalModel creates a pre-computed mental model in a bank.
func (c *Client) CreateMentalModel(ctx context.Context, bankID string, req *CreateMentalModelRequest) error {
	path := fmt.Sprintf("/v1/default/banks/%s/mental-models", bankID)
	resp, err := c.do(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 201 Created or 200 OK are both success
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hindsight create mental model: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// HealthCheck verifies the Hindsight API is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hindsight health check: status %d", resp.StatusCode)
	}
	return nil
}

// do executes an HTTP request against the Hindsight API.
func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
