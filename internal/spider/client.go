package spider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with the Spider.cloud REST API.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Spider.cloud API client.
func NewClient(endpoint, apiKey string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Crawl crawls a website starting from the given URL.
// POST /v1/crawl
func (client *Client) Crawl(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/crawl", params)
}

// Search performs a web search and optionally fetches page content.
// POST /v1/search
func (client *Client) Search(ctx context.Context, params SearchParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/search", params)
}

// Links retrieves all links from the given URL.
// POST /v1/links
func (client *Client) Links(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/links", params)
}

// Screenshot takes a screenshot of the given URL.
// POST /v1/screenshot
func (client *Client) Screenshot(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/screenshot", params)
}

// Transform converts HTML content to markdown or text without re-fetching.
// POST /v1/transform
func (client *Client) Transform(ctx context.Context, params TransformParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/transform", params)
}

func (client *Client) doPost(ctx context.Context, path string, body any) ([]Response, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+client.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("spider API error %d: %s", resp.StatusCode, string(respBody))
	}

	var results []Response
	if err := json.Unmarshal(respBody, &results); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, truncate(string(respBody), 500))
	}

	return results, nil
}

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
