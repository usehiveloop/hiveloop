package main

import (
	"context"
	"fmt"
	"log"

	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

type queryResult struct {
	id       string
	docID    string
	topic    string
	score    float64
	rerank   float64
	isPublic bool
}

func search(ctx context.Context, qd *qdrant.Client, emb *embedclient.Embedder,
	rer *embedclient.Reranker, collection, query string,
	filter map[string]any, rerank bool) ([]queryResult, error) {
	v, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	limit := uint32(searchTopK)
	if rerank {
		limit = rerankPool
	}
	hits, err := qd.Search(ctx, qdrant.SearchRequest{
		Collection:  collection,
		Vector:      v[0],
		Filter:      filter,
		Limit:       limit,
		WithPayload: true,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	results := hitsToResults(hits)
	if rerank && rer != nil {
		return rerankResults(ctx, rer, query, hits, results)
	}
	if len(results) > searchTopK {
		results = results[:searchTopK]
	}
	return results, nil
}

func hitsToResults(hits []qdrant.Hit) []queryResult {
	results := make([]queryResult, 0, len(hits))
	for _, h := range hits {
		r := queryResult{score: h.Score}
		if id, ok := h.ID.(string); ok {
			r.id = id
		}
		if h.Payload != nil {
			if v, ok := h.Payload["doc_id"].(string); ok {
				r.docID = v
			}
			if v, ok := h.Payload["topic"].(string); ok {
				r.topic = v
			}
			if v, ok := h.Payload["is_public"].(bool); ok {
				r.isPublic = v
			}
		}
		results = append(results, r)
	}
	return results
}

func rerankResults(ctx context.Context, rer *embedclient.Reranker, query string,
	hits []qdrant.Hit, base []queryResult) ([]queryResult, error) {
	docs := make([]string, len(hits))
	for i, h := range hits {
		if h.Payload != nil {
			if c, ok := h.Payload["content"].(string); ok {
				docs[i] = truncate(c, 1500)
			}
		}
	}
	rr, err := rer.Rerank(ctx, query, docs, searchTopK)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	out := make([]queryResult, 0, len(rr))
	for _, r := range rr {
		if r.Index < 0 || r.Index >= len(base) {
			continue
		}
		pick := base[r.Index]
		pick.rerank = r.Score
		out = append(out, pick)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func mustOK(name string, ok bool) {
	if !ok {
		log.Fatalf("FAIL: %s", name)
	}
	log.Printf("PASS: %s", name)
}

func topicsFromResults(rs []queryResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.topic
	}
	return out
}

func countTopic(rs []queryResult, topic string) int {
	n := 0
	for _, r := range rs {
		if r.topic == topic {
			n++
		}
	}
	return n
}

func anyMatch(rs []queryResult, predicate func(queryResult) bool) bool {
	for _, r := range rs {
		if predicate(r) {
			return true
		}
	}
	return false
}

func docIDs(rs []queryResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.docID
	}
	return out
}

func isDescending(rs []queryResult) bool {
	for i := 1; i < len(rs); i++ {
		if rs[i].rerank > rs[i-1].rerank {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
