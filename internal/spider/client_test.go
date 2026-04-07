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

	spiderResponse := []Response{
		{Content: "Search result 1", URL: "https://example.com/1", StatusCode: 200},
		{Content: "Search result 2", URL: "https://example.com/2", StatusCode: 200},
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
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
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

	spiderResponse := []Response{
		{Content: "# Transformed content", URL: "https://example.com", StatusCode: 200},
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
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/transform" {
		t.Fatalf("expected path /v1/transform, got %s", captured.Path)
	}
}

func TestCrawl_APIError(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	_, err := client.Crawl(context.Background(), SpiderParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for 429 response, got nil")
	}
}

func TestCrawl_ServerError(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	_, err := client.Crawl(context.Background(), SpiderParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestCrawl_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	_, err := client.Crawl(context.Background(), SpiderParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for invalid JSON response, got nil")
	}
}

func TestCrawl_EmptyResponse(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, []Response{})
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	results, err := client.Crawl(context.Background(), SpiderParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestCrawl_MultiplePages(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	spiderResponse := []Response{
		{Content: "Page 1", URL: "https://example.com/1", StatusCode: 200},
		{Content: "Page 2", URL: "https://example.com/2", StatusCode: 200},
		{Content: "Page 3", URL: "https://example.com/3", StatusCode: 200},
	}

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, spiderResponse)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	limit := 3
	results, err := client.Crawl(context.Background(), SpiderParams{
		URL:   "https://example.com",
		Limit: &limit,
	})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestCrawl_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — the context should be cancelled before this completes
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.Crawl(ctx, SpiderParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSearch_OptionalParams(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, []Response{})
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	fetchContent := true
	_, err := client.Search(context.Background(), SearchParams{
		Search:           "test query",
		FetchPageContent: &fetchContent,
		Country:          "us",
		Language:         "en",
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var sentBody map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &sentBody); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sentBody["fetch_page_content"] != true {
		t.Fatalf("expected fetch_page_content true, got %v", sentBody["fetch_page_content"])
	}
	if sentBody["country"] != "us" {
		t.Fatalf("expected country 'us', got %v", sentBody["country"])
	}
	if sentBody["language"] != "en" {
		t.Fatalf("expected language 'en', got %v", sentBody["language"])
	}
}
