package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/spider"
)

func TestSpiderCrawl_Success(t *testing.T) {
	var captured struct {
		Path string
		Body string
		Auth string
	}
	var mu sync.Mutex

	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured.Path = r.URL.Path
		captured.Body = string(body)
		captured.Auth = r.Header.Get("Authorization")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "# Example", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL:          "https://example.com",
		ReturnFormat: "markdown",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var results []spider.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "# Example" {
		t.Fatalf("expected '# Example', got %q", results[0].Content)
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/crawl" {
		t.Fatalf("expected spider path /v1/crawl, got %s", captured.Path)
	}
	if captured.Auth != "Bearer test-spider-key" {
		t.Fatalf("expected Bearer auth to spider, got %q", captured.Auth)
	}
}

func TestSpiderCrawl_MissingURL(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when URL is missing")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderCrawl_SpiderError(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderSearch_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spider.SearchResponse{
			Content: []spider.SearchResult{
				{Title: "Result 1", Description: "First result", URL: "https://example.com/1"},
				{Title: "Result 2", Description: "Second result", URL: "https://example.com/2"},
			},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/search", spider.SearchParams{
		Search: "test query",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var results spider.SearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(results.Content) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results.Content))
	}
}

func TestSpiderSearch_MissingQuery(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when search is missing")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/search", spider.SearchParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderLinks_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "/about\n/contact", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/links", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderScreenshot_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "base64-screenshot-data", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/screenshot", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderTransform_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spider.TransformResponse{
			Content: []string{"# Transformed"},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/transform", spider.TransformParams{
		Data: []spider.TransformInput{
			{HTML: "<h1>Hello</h1>", URL: "https://example.com"},
		},
		ReturnFormat: "markdown",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderTransform_EmptyData(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when data is empty")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/transform", spider.TransformParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}
