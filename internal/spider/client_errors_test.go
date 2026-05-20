package spider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

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
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Crawl(ctx, SpiderParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSearch_OptionalParams(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, SearchResponse{Content: []SearchResult{}})
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
