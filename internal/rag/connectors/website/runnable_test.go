package website

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	"github.com/usehiveloop/hiveloop/internal/spider"
)

type stubSource struct{ cfg json.RawMessage }

func (s *stubSource) SourceID() string        { return "src-1" }
func (s *stubSource) OrgID() string           { return "org-1" }
func (s *stubSource) SourceKind() string      { return Kind }
func (s *stubSource) Config() json.RawMessage { return s.cfg }

func TestRun_StreamsResponsesAsDocsAndFailures(t *testing.T) {
	pages := []spider.Response{
		{URL: "https://example.com/", Content: "# Home", StatusCode: 200},
		{URL: "https://example.com/a", Content: "", StatusCode: 200},
		{URL: "https://example.com/b", Content: "", StatusCode: 500, Error: "bad gateway"},
		{URL: "https://example.com/c", Content: "## C body", StatusCode: 200},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/jsonl")
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

	cli := spider.NewClient(srv.URL, "k")
	c := NewConnector(WebsiteConfig{URL: "https://example.com", MaxPages: 100}, cli)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := c.Run(ctx, &stubSource{}, nil, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var docs []*interfaces.Document
	var fails []*interfaces.ConnectorFailure
	for ev := range out {
		if ev.Doc != nil {
			docs = append(docs, ev.Doc)
		}
		if ev.Failure != nil {
			fails = append(fails, ev.Failure)
		}
	}
	if len(docs) != 2 {
		t.Fatalf("docs: got %d, want 2 (URLs: %v)", len(docs), urlsOf(docs))
	}
	if len(fails) != 1 {
		t.Fatalf("failures: got %d, want 1", len(fails))
	}
	if got := docs[0].DocID; got != "https://example.com/" {
		t.Errorf("docs[0].DocID = %q, want %q", got, "https://example.com/")
	}
	if got := docs[1].DocID; got != "https://example.com/c" {
		t.Errorf("docs[1].DocID = %q, want %q", got, "https://example.com/c")
	}
	if fails[0].FailedDocument == nil || fails[0].FailedDocument.DocID != "https://example.com/b" {
		t.Errorf("failure didn't pin to /b: %+v", fails[0])
	}
}

func urlsOf(docs []*interfaces.Document) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.DocID
	}
	return out
}
