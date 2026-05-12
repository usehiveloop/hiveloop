package rag

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/embedclient"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func TestSearchKnowledgeBase_LiveSlackCollection(t *testing.T) {
	if os.Getenv("HIVELOOP_E2E_KB_SEARCH") != "1" {
		t.Skip("set HIVELOOP_E2E_KB_SEARCH=1 to run live knowledge-base search")
	}
	collection := requiredSearchEnv(t, "HIVELOOP_E2E_KB_COLLECTION")
	orgID := requiredSearchEnv(t, "HIVELOOP_E2E_KB_ORG_ID")
	llmURL := requiredSearchEnv(t, "LLM_API_URL")
	llmKey := requiredSearchEnv(t, "LLM_API_KEY")
	llmModel := requiredSearchEnv(t, "LLM_MODEL")
	qdrantHost := requiredSearchEnv(t, "QDRANT_HOST")
	qdrantPort := searchEnvInt("QDRANT_PORT", 6334)
	dim := searchEnvInt("LLM_EMBEDDING_DIM", 3072)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	qd, err := qdrant.New(qdrant.Config{
		Host:                   qdrantHost,
		Port:                   qdrantPort,
		UseTLS:                 searchEnvBool("QDRANT_USE_TLS"),
		APIKey:                 os.Getenv("QDRANT_API_KEY"),
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		t.Fatalf("qdrant: %v", err)
	}
	t.Cleanup(func() { _ = qd.Close() })

	embedder := embedclient.NewEmbedder(embedclient.EmbedderConfig{
		BaseURL: llmURL,
		APIKey:  llmKey,
		Model:   llmModel,
		Dim:     uint32(dim), //nolint:gosec // controlled test env value
		Timeout: 45 * time.Second,
	})

	for _, query := range []string{
		"Steam API owned games recently played wishlist review data",
		"deployment rollback notes engineering production deploy",
		"code first design frontend workflow codespaces",
	} {
		vectors, err := embedder.Embed(ctx, []string{query})
		if err != nil {
			t.Fatalf("embed %q: %v", query, err)
		}
		hits, err := qd.Search(ctx, qdrant.SearchRequest{
			Collection:  collection,
			Vector:      vectors[0],
			Filter:      qdrant.BuildACLFilter(orgID, nil, true),
			Limit:       10,
			WithPayload: true,
		})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if len(hits) == 0 {
			t.Fatalf("search %q returned no hits", query)
		}
		groups := groupKnowledgeHitsBySource(hits)
		if len(groups) == 0 {
			t.Fatalf("search %q returned no grouped sources", query)
		}
		t.Logf("query=%q total_hits=%d source_groups=%d", query, len(hits), len(groups))
		for i, hit := range hits {
			if i >= 5 {
				break
			}
			meta, _ := hit.Payload["metadata"].(map[string]any)
			t.Logf("hit score=%.4f title=%v channel=%v date=%v doc_id=%v",
				hit.Score, hit.Payload["semantic_id"], meta["channel_name"], meta["date"], hit.Payload["doc_id"])
		}
	}
}

func requiredSearchEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s is required", key)
	}
	return value
}

func searchEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func searchEnvBool(key string) bool {
	value, _ := strconv.ParseBool(os.Getenv(key))
	return value
}
