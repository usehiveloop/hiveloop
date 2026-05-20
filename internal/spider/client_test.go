package spider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type capturedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   string
}

func mockSpiderAPI(t *testing.T, captured *capturedRequest, mu *sync.Mutex, statusCode int, response any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*captured = capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Body:   string(body),
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	}))
}

func TestCrawl_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := []Response{
		{Content: "# Hello World", URL: "https://example.com", StatusCode: 200},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-api-key")
	limit := 1
	results, err := client.Crawl(context.Background(), SpiderParams{
		URL:          "https://example.com",
		Limit:        &limit,
		ReturnFormat: "markdown",
	})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "# Hello World" {
		t.Fatalf("expected '# Hello World', got %q", results[0].Content)
	}
	if results[0].URL != "https://example.com" {
		t.Fatalf("expected URL 'https://example.com', got %q", results[0].URL)
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Method != "POST" {
		t.Fatalf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/v1/crawl" {
		t.Fatalf("expected path /v1/crawl, got %s", captured.Path)
	}
	if captured.Auth != "Bearer test-api-key" {
		t.Fatalf("expected Bearer auth header, got %q", captured.Auth)
	}

	var sentBody map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &sentBody); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sentBody["url"] != "https://example.com" {
		t.Fatalf("expected url in body, got %v", sentBody["url"])
	}
	if sentBody["return_format"] != "markdown" {
		t.Fatalf("expected return_format markdown, got %v", sentBody["return_format"])
	}
}

func TestSearch_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := SearchResponse{
		Content: []SearchResult{
			{Title: "Search result 1", URL: "https://example.com/1", Description: "First"},
			{Title: "Search result 2", URL: "https://example.com/2", Description: "Second"},
		},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	searchLimit := 5
	results, err := client.Search(context.Background(), SearchParams{
		Search:      "golang testing",
		SearchLimit: &searchLimit,
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results.Content) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results.Content))
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/search" {
		t.Fatalf("expected path /v1/search, got %s", captured.Path)
	}

	var sentBody map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &sentBody); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sentBody["search"] != "golang testing" {
		t.Fatalf("expected search query 'golang testing', got %v", sentBody["search"])
	}
}

func TestLinks_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := []Response{
		{Content: "https://example.com/about", URL: "https://example.com", StatusCode: 200},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	results, err := client.Links(context.Background(), SpiderParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Links() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/links" {
		t.Fatalf("expected path /v1/links, got %s", captured.Path)
	}
}

func TestScreenshot_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := []Response{
		{Content: "base64-encoded-image", URL: "https://example.com", StatusCode: 200},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	results, err := client.Screenshot(context.Background(), SpiderParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Screenshot() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/screenshot" {
		t.Fatalf("expected path /v1/screenshot, got %s", captured.Path)
	}
}

func TestTransform_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := TransformResponse{
		Content: []string{"# Transformed content"},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	results, err := client.Transform(context.Background(), TransformParams{
		Data: []TransformInput{
			{HTML: "<h1>Hello</h1>", URL: "https://example.com"},
		},
		ReturnFormat: "markdown",
	})
	if err != nil {
		t.Fatalf("Transform() error: %v", err)
	}
	if len(results.Content) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results.Content))
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/transform" {
		t.Fatalf("expected path /v1/transform, got %s", captured.Path)
	}
}
