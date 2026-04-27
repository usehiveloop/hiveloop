package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

type doc struct {
	docID    string
	topic    string
	content  string
	acl      []string
	isPublic bool
}

func genDocs() []doc {
	rng := rand.New(rand.NewSource(42))
	docs := make([]doc, 0, totalDocs)
	docsPerTopic := totalDocs / len(topics)
	for _, t := range topics {
		for j := 0; j < docsPerTopic; j++ {
			tpl := t.templates[j%len(t.templates)]
			extra := fmt.Sprintf(" Variation %d in the %s series with details on %s patterns and tradeoffs.",
				j, t.name, t.name)
			d := doc{
				docID:   fmt.Sprintf("doc-%s-%03d", t.name, j),
				topic:   t.name,
				content: tpl + extra,
			}
			// 60% public, 40% private; alice has access to ~25% of private docs.
			if rng.Intn(10) < 6 {
				d.isPublic = true
			} else if rng.Intn(4) == 0 {
				d.acl = []string{"alice@example.com", "bob@example.com"}
			} else {
				d.acl = []string{"bob@example.com", "carol@example.com"}
			}
			docs = append(docs, d)
		}
	}
	return docs
}

func ingest(ctx context.Context, qd *qdrant.Client, emb *embedclient.Embedder,
	collection, orgID, sourceID string, docs []doc) error {
	for start := 0; start < len(docs); start += embedBatch {
		end := start + embedBatch
		if end > len(docs) {
			end = len(docs)
		}
		chunk := docs[start:end]
		texts := make([]string, len(chunk))
		for i := range chunk {
			texts[i] = chunk[i].content
		}
		t0 := time.Now()
		vectors, err := emb.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed [%d:%d]: %w", start, end, err)
		}
		points := make([]qdrant.Point, len(chunk))
		for i := range chunk {
			d := &chunk[i]
			points[i] = qdrant.Point{
				ID:     qdrant.PointID(orgID, d.docID),
				Vector: vectors[i],
				Payload: map[string]any{
					"org_id":        orgID,
					"rag_source_id": sourceID,
					"doc_id":        d.docID,
					"topic":         d.topic,
					"acl":           append([]string(nil), d.acl...),
					"is_public":     d.isPublic,
					"content":       d.content,
				},
			}
		}
		for u := 0; u < len(points); u += upsertBatch {
			ue := u + upsertBatch
			if ue > len(points) {
				ue = len(points)
			}
			if err := upsertWithRetry(ctx, qd, collection, points[u:ue]); err != nil {
				return fmt.Errorf("upsert [%d:%d]: %w", start+u, start+ue, err)
			}
		}
		log.Printf("ingest: %d/%d (embed+upsert took %s)", end, len(docs), time.Since(t0))
	}
	return nil
}

func upsertWithRetry(ctx context.Context, qd *qdrant.Client, collection string, pts []qdrant.Point) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		err := qd.Upsert(ctx, collection, pts, false)
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("upsert retry %d: %v", attempt+1, err)
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	return lastErr
}
