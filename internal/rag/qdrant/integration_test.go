package qdrant_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"

	qdrantgo "github.com/qdrant/go-client/qdrant"

	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func liveClient(t *testing.T) *qdrant.Client {
	t.Helper()
	host := os.Getenv("QDRANT_HOST")
	if host == "" {
		t.Skip("QDRANT_HOST not set; skipping live qdrant test")
	}
	port, _ := strconv.Atoi(os.Getenv("QDRANT_PORT"))
	useTLS, _ := strconv.ParseBool(os.Getenv("QDRANT_USE_TLS"))
	c, err := qdrant.New(qdrant.Config{
		Host:   host,
		Port:   port,
		UseTLS: useTLS,
		APIKey: os.Getenv("QDRANT_API_KEY"),
	})
	if err != nil {
		t.Fatalf("dial qdrant: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func randomVector(dim int, seed uint64) []float32 {
	// Deterministic vector for test fixtures; not security-sensitive.
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)) //nolint:gosec
	v := make([]float32, dim)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

// Exercises the contract perm_sync, ingest_batch, prune, and search depend on:
// - upsert with payload survives a scroll and a vector search
// - org-partitioned + (is_public OR acl any-of) filter does what BuildACLFilter says
// - tenant isolation: foreign org_id returns nothing
// - prune-style scroll filter (org_id + rag_source_id) returns only that source's points
// - DeleteByIDs removes points end-to-end
func TestQdrantContract_Live(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const dim = 32
	collection := fmt.Sprintf("contract_%d", time.Now().UnixNano())
	if err := c.EnsureCollection(ctx, qdrant.CollectionConfig{
		Name: collection, VectorDim: dim, OnDisk: false,
	}); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteCollection(context.Background(), collection) })

	orgA, orgB := "orgA", "orgB"
	srcX, srcY := "srcX", "srcY"

	type fixture struct {
		org, src, doc string
		isPublic      bool
		acl           []string
	}
	fixtures := []fixture{
		{orgA, srcX, "d1", true, nil},
		{orgA, srcX, "d2", false, []string{"alice@example.com"}},
		{orgA, srcX, "d3", false, []string{"bob@example.com"}},
		{orgA, srcY, "d4", true, nil},
		{orgB, srcX, "d5", true, nil},
	}

	points := make([]qdrant.Point, len(fixtures))
	for i, f := range fixtures {
		points[i] = qdrant.Point{
			ID:     qdrant.PointID(f.org, f.src, f.doc),
			Vector: randomVector(dim, uint64(i+1)), //nolint:gosec
			Payload: map[string]any{
				"org_id":        f.org,
				"rag_source_id": f.src,
				"doc_id":        f.doc,
				"is_public":     f.isPublic,
				"acl":           append([]string(nil), f.acl...),
			},
		}
	}
	if err := c.Upsert(ctx, collection, points, true); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	cnt, err := c.Count(ctx, collection, nil)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if cnt != uint64(len(fixtures)) {
		t.Fatalf("count = %d, want %d", cnt, len(fixtures))
	}

	t.Run("public-only filter excludes private docs", func(t *testing.T) {
		filter := qdrant.BuildACLFilter(orgA, []string{"nobody@example.com"}, false)
		hits, err := c.Search(ctx, qdrant.SearchRequest{
			Collection: collection, Vector: randomVector(dim, 999),
			Filter: filter, Limit: 10, WithPayload: true,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		for _, h := range hits {
			if pub, _ := h.Payload["is_public"].(bool); !pub {
				t.Errorf("private doc leaked: %v", h.Payload)
			}
		}
	})

	t.Run("alice's ACL surfaces her private doc", func(t *testing.T) {
		filter := qdrant.BuildACLFilter(orgA, []string{"alice@example.com"}, false)
		hits, err := c.Search(ctx, qdrant.SearchRequest{
			Collection: collection, Vector: randomVector(dim, 999),
			Filter: filter, Limit: 10, WithPayload: true,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		var sawAlice bool
		for _, h := range hits {
			if id, _ := h.Payload["doc_id"].(string); id == "d2" {
				sawAlice = true
			}
		}
		if !sawAlice {
			t.Fatalf("alice's private doc d2 missing from hits")
		}
	})

	t.Run("tenant isolation: orgB sees only its own", func(t *testing.T) {
		filter := qdrant.BuildACLFilter(orgB, nil, true)
		hits, err := c.Search(ctx, qdrant.SearchRequest{
			Collection: collection, Vector: randomVector(dim, 999),
			Filter: filter, Limit: 10, WithPayload: true,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		for _, h := range hits {
			if org, _ := h.Payload["org_id"].(string); org != orgB {
				t.Errorf("orgA doc leaked into orgB search: %v", h.Payload)
			}
		}
	})

	t.Run("prune scroll: only orgA/srcX points are seen", func(t *testing.T) {
		filter := qdrant.BuildSourceFilter(orgA, srcX)
		seen := map[string]bool{}
		var offset *qdrantgo.PointId
		for {
			page, err := c.Scroll(ctx, qdrant.ScrollRequest{
				Collection: collection, Filter: filter, Limit: 100,
				Offset: offset, WithPayload: true,
			})
			if err != nil {
				t.Fatalf("scroll: %v", err)
			}
			for _, p := range page.Points {
				doc, _ := p.Payload["doc_id"].(string)
				seen[doc] = true
			}
			if page.NextOffset == nil {
				break
			}
			offset = page.NextOffset
		}
		want := map[string]bool{"d1": true, "d2": true, "d3": true}
		for d := range want {
			if !seen[d] {
				t.Errorf("scroll missed orgA/srcX doc %s", d)
			}
		}
		for d := range seen {
			if !want[d] {
				t.Errorf("scroll surfaced unexpected doc %s (wrong source/org)", d)
			}
		}
	})

	t.Run("DeleteByIDs sweeps a source", func(t *testing.T) {
		toDelete := []string{
			qdrant.PointID(orgA, srcX, "d1"),
			qdrant.PointID(orgA, srcX, "d2"),
			qdrant.PointID(orgA, srcX, "d3"),
		}
		if err := c.DeleteByIDs(ctx, collection, toDelete); err != nil {
			t.Fatalf("DeleteByIDs: %v", err)
		}
		cnt, err := c.Count(ctx, collection, nil)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if cnt != 2 {
			t.Fatalf("after delete count = %d, want 2 (orgA/srcY/d4 and orgB/srcX/d5)", cnt)
		}
	})
}
