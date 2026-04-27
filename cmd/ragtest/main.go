// ragtest is a live end-to-end exercise of the Qdrant + embedclient layer.
// It stands up a throwaway collection, ingests 500 synthetic docs across
// multiple topics + ACL flavors, and runs business-shaped queries:
//   - relevance: a topic query surfaces docs from that topic
//   - rerank: cross-encoder reorders the candidate pool
//   - ACL: private docs only return when caller's email is in the ACL
//   - ACL bypass: bypass surfaces private docs without ACL match
//   - tenant isolation: a different org_id returns nothing
//
// Reads QDRANT_ENDPOINT, QDRANT_API_KEY, LLM_API_URL/KEY/MODEL,
// LLM_EMBEDDING_DIM, RERANKER_BASE_URL/API_KEY/MODEL from the environment.
// Cleans up the test collection on success.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

const (
	totalDocs   = 500
	embedBatch  = 64
	upsertBatch = 64
	searchTopK  = 10
	rerankPool  = 50
)

type cfg struct {
	qdrantEndpoint string
	qdrantAPIKey   string
	embedURL       string
	embedKey       string
	embedModel     string
	embedDim       uint32
	rerankURL      string
	rerankKey      string
	rerankModel    string
}

func loadCfg() (cfg, error) {
	c := cfg{
		qdrantEndpoint: os.Getenv("QDRANT_ENDPOINT"),
		qdrantAPIKey:   os.Getenv("QDRANT_API_KEY"),
		embedURL:       os.Getenv("LLM_API_URL"),
		embedKey:       os.Getenv("LLM_API_KEY"),
		embedModel:     os.Getenv("LLM_MODEL"),
		rerankURL:      os.Getenv("RERANKER_BASE_URL"),
		rerankKey:      os.Getenv("RERANKER_API_KEY"),
		rerankModel:    os.Getenv("RERANKER_MODEL"),
	}
	dimStr := os.Getenv("LLM_EMBEDDING_DIM")
	if dimStr == "" {
		dimStr = "3072"
	}
	d, err := strconv.Atoi(dimStr)
	if err != nil {
		return c, fmt.Errorf("parse LLM_EMBEDDING_DIM: %w", err)
	}
	c.embedDim = uint32(d)
	if c.qdrantEndpoint == "" || c.embedURL == "" || c.embedKey == "" || c.embedModel == "" {
		return c, fmt.Errorf("missing required env: QDRANT_ENDPOINT, LLM_API_URL, LLM_API_KEY, LLM_MODEL")
	}
	return c, nil
}

func setup(ctx context.Context, c cfg) (*qdrant.Client, *embedclient.Embedder, *embedclient.Reranker, string) {
	qd := qdrant.New(qdrant.Config{Endpoint: c.qdrantEndpoint, APIKey: c.qdrantAPIKey})
	emb := embedclient.NewEmbedder(embedclient.EmbedderConfig{
		BaseURL: c.embedURL, APIKey: c.embedKey, Model: c.embedModel, Dim: c.embedDim,
	})
	var rer *embedclient.Reranker
	if c.rerankURL != "" && c.rerankKey != "" && c.rerankModel != "" {
		rer = embedclient.NewReranker(embedclient.RerankerConfig{
			BaseURL: c.rerankURL, APIKey: c.rerankKey, Model: c.rerankModel,
		})
	}
	collection := fmt.Sprintf("ragtest_%d", time.Now().Unix())
	log.Printf("using collection %s, embed model %s (dim %d)", collection, c.embedModel, c.embedDim)
	if err := qd.EnsureCollection(ctx, qdrant.CollectionConfig{
		Name:      collection,
		VectorDim: c.embedDim,
		OnDisk:    true,
	}); err != nil {
		log.Fatalf("EnsureCollection: %v", err)
	}
	if err := qd.CreatePayloadIndex(ctx, collection, "topic", "keyword"); err != nil &&
		!strings.Contains(err.Error(), "already exists") {
		log.Printf("CreatePayloadIndex(topic): %v", err)
	}
	return qd, emb, rer, collection
}

func main() {
	c, err := loadCfg()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()

	qd, emb, rer, collection := setup(ctx, c)
	defer func() {
		if err := qd.DeleteCollection(context.Background(), collection); err != nil {
			log.Printf("cleanup: %v", err)
		} else {
			log.Printf("cleanup: dropped %s", collection)
		}
	}()

	orgA := uuid.New().String()
	orgB := uuid.New().String()
	sourceID := uuid.New().String()

	docs := genDocs()
	log.Printf("generated %d docs", len(docs))

	t0 := time.Now()
	if err := ingest(ctx, qd, emb, collection, orgA, sourceID, docs); err != nil {
		log.Fatalf("ingest: %v", err)
	}
	log.Printf("ingest done in %s", time.Since(t0))

	time.Sleep(3 * time.Second)
	cnt, err := qd.Count(ctx, collection, nil)
	if err != nil {
		log.Fatalf("Count: %v", err)
	}
	log.Printf("collection point count: %d", cnt)
	mustOK("count matches ingested doc count", cnt == uint64(len(docs)))

	runQueries(ctx, qd, emb, rer, collection, orgA, orgB)
	log.Printf("ALL CHECKS PASSED")
}

func runQueries(ctx context.Context, qd *qdrant.Client, emb *embedclient.Embedder,
	rer *embedclient.Reranker, collection, orgA, orgB string) {
	publicFilter := qdrant.BuildACLFilter(orgA, []string{"strangerwithno@access.com"}, false)
	bypassFilter := qdrant.BuildACLFilter(orgA, nil, true)

	// Q1: topic relevance, public-only access.
	q1 := "How does Rust prevent data races at compile time?"
	r1, err := search(ctx, qd, emb, rer, collection, q1, publicFilter, false)
	if err != nil {
		log.Fatalf("Q1: %v", err)
	}
	log.Printf("Q1 topics: %v", topicsFromResults(r1))
	mustOK("Q1: top result is rust", len(r1) > 0 && r1[0].topic == "rust")
	mustOK("Q1: only public docs returned", !anyMatch(r1, func(q queryResult) bool { return !q.isPublic }))
	mustOK("Q1: rust dominates top-10", countTopic(r1, "rust") >= 4)

	// Q2: rerank reorders the candidate pool.
	if rer != nil {
		q2 := "tell me about cross-encoder retrieval pipelines for grounding LLM answers"
		r2nr, err := search(ctx, qd, emb, nil, collection, q2, bypassFilter, false)
		if err != nil {
			log.Fatalf("Q2 no-rerank: %v", err)
		}
		r2rr, err := search(ctx, qd, emb, rer, collection, q2, bypassFilter, true)
		if err != nil {
			log.Fatalf("Q2 rerank: %v", err)
		}
		log.Printf("Q2 no-rerank topics: %v", topicsFromResults(r2nr))
		log.Printf("Q2 rerank topics: %v", topicsFromResults(r2rr))
		mustOK("Q2: rerank surfaces ML topic in top-3",
			countTopic(r2rr[:minInt(3, len(r2rr))], "machinelearning") >= 1)
		mustOK("Q2: rerank scores monotonically decrease", isDescending(r2rr))
	} else {
		log.Printf("Q2 skipped: no reranker configured")
	}

	// Q3: alice's ACL surfaces a private postgres doc that public-only access misses.
	aliceFilter := qdrant.BuildACLFilter(orgA, []string{"alice@example.com"}, false)
	q3 := "Postgres MVCC and dead tuples reclaimed by VACUUM"
	r3a, err := search(ctx, qd, emb, rer, collection, q3, aliceFilter, false)
	if err != nil {
		log.Fatalf("Q3 alice: %v", err)
	}
	r3p, err := search(ctx, qd, emb, rer, collection, q3, publicFilter, false)
	if err != nil {
		log.Fatalf("Q3 public: %v", err)
	}
	log.Printf("Q3 alice top-10 doc_ids: %v", docIDs(r3a))
	log.Printf("Q3 public top-10 doc_ids: %v", docIDs(r3p))
	mustOK("Q3: alice sees a private postgres doc that public-only does not",
		anyMatch(r3a, func(q queryResult) bool { return !q.isPublic && q.topic == "postgres" }))

	// Q4: ACL bypass returns at least one private doc.
	q4 := "kubernetes deployments rolling update"
	r4b, err := search(ctx, qd, emb, rer, collection, q4, bypassFilter, false)
	if err != nil {
		log.Fatalf("Q4 bypass: %v", err)
	}
	r4p, err := search(ctx, qd, emb, rer, collection, q4, publicFilter, false)
	if err != nil {
		log.Fatalf("Q4 public: %v", err)
	}
	mustOK("Q4: bypass yields >= public-only count for same query", len(r4b) >= len(r4p))
	mustOK("Q4: bypass returns at least one private doc",
		anyMatch(r4b, func(q queryResult) bool { return !q.isPublic }))

	// Q5: tenant isolation — orgB has no docs.
	r5, err := search(ctx, qd, emb, nil, collection, "anything",
		qdrant.BuildACLFilter(orgB, nil, true), false)
	if err != nil {
		log.Fatalf("Q5: %v", err)
	}
	mustOK("Q5: foreign org returns zero hits", len(r5) == 0)
}
