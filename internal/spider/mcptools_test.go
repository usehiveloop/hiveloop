package spider

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func callTool(handler mcp.ToolHandler, args map[string]any) (*mcp.CallToolResult, error) {
	var raw json.RawMessage
	if args != nil {
		raw, _ = json.Marshal(args)
	}
	return handler(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: raw,
		},
	})
}

func TestWebFetch_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := []Response{
		{Content: "# Hello World", URL: "https://example.com", StatusCode: 200},
	}

	srv := mockSpiderAPI(t, &captured, &mu, 200, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-api-key")
	result, err := callTool(WebFetchHandler(client), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(textContent(result)), &body); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if body["url"] != "https://example.com" {
		t.Errorf("expected url 'https://example.com', got %v", body["url"])
	}
	if body["content"] != "# Hello World" {
		t.Errorf("expected content '# Hello World', got %v", body["content"])
	}
	if body["status"] != float64(200) {
		t.Errorf("expected status 200, got %v", body["status"])
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.Path != "/v1/crawl" {
		t.Errorf("expected path /v1/crawl, got %s", captured.Path)
	}
	if captured.Auth != "Bearer test-api-key" {
		t.Errorf("expected Bearer auth, got %q", captured.Auth)
	}

	var sentBody map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &sentBody); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sentBody["return_format"] != "markdown" {
		t.Errorf("expected default return_format 'markdown', got %v", sentBody["return_format"])
	}
	if sentBody["request"] != "smart" {
		t.Errorf("expected request type 'smart', got %v", sentBody["request"])
	}
}

func TestWebSearch_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := SearchResponse{
		Content: []SearchResult{
			{Title: "Go Testing", URL: "https://go.dev/doc/testing", Description: "Official Go testing docs"},
			{Title: "Testify", URL: "https://github.com/stretchr/testify", Description: "Testing toolkit"},
		},
	}

	srv := mockSpiderAPI(t, &captured, &mu, 200, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-api-key")
	num := 3
	result, err := callTool(WebSearchHandler(client), map[string]any{"query": "golang testing", "num": num})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}

	var results []SearchResult
	if err := json.Unmarshal([]byte(textContent(result)), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Go Testing" {
		t.Errorf("expected first result title 'Go Testing', got %q", results[0].Title)
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.Path != "/v1/search" {
		t.Errorf("expected path /v1/search, got %s", captured.Path)
	}

	var sentBody map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &sentBody); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sentBody["search"] != "golang testing" {
		t.Errorf("expected search 'golang testing', got %v", sentBody["search"])
	}
	if sentBody["num"] != float64(3) {
		t.Errorf("expected num 3, got %v", sentBody["num"])
	}
}

func TestWebFetch_MissingURL(t *testing.T) {
	srv := mockSpiderAPI(t, &capturedRequest{}, &sync.Mutex{}, 200, []Response{})
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-api-key")
	result, err := callTool(WebFetchHandler(client), map[string]any{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing url, got success")
	}
	if textContent(result) != "Error: url is required" {
		t.Errorf("expected 'Error: url is required', got %q", textContent(result))
	}
}

func textContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
