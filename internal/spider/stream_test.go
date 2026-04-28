package spider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCrawlStream_EmitsResponsesInOrder(t *testing.T) {
	pages := []Response{
		{URL: "https://example.com/", Content: "# Home", StatusCode: 200},
		{URL: "https://example.com/a", Content: "# A", StatusCode: 200},
		{URL: "https://example.com/b", Content: "", StatusCode: 500, Error: "bad gateway"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/crawl" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Content-Type"); got != "application/jsonl" {
			t.Errorf("Content-Type = %q, want application/jsonl", got)
		}
		w.Header().Set("Content-Type", "application/jsonl")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, p := range pages {
			b, _ := json.Marshal(p)
			fmt.Fprintf(w, "%s\n", b)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, errs := c.CrawlStream(ctx, SpiderParams{URL: "https://example.com"})
	got := []Response{}
	for r := range out {
		got = append(got, r)
	}
	if err := <-errs; err != nil {
		t.Fatalf("err channel: %v", err)
	}
	if len(got) != len(pages) {
		t.Fatalf("got %d responses, want %d", len(got), len(pages))
	}
	for i := range pages {
		if got[i].URL != pages[i].URL {
			t.Errorf("[%d] url = %q, want %q", i, got[i].URL, pages[i].URL)
		}
	}
}

func TestCrawlStream_PropagatesUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"err":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, errs := c.CrawlStream(ctx, SpiderParams{URL: "https://example.com"})
	for range out {
	}
	err := <-errs
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
