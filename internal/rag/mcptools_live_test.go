package rag

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/qdrant"
)

func liveKnowledgeQdrantClient(t *testing.T) *qdrant.Client {
	t.Helper()
	host := os.Getenv("QDRANT_HOST")
	if host == "" {
		t.Skip("QDRANT_HOST not set; run `make test-services-up` and set QDRANT_HOST=localhost")
	}
	port, _ := strconv.Atoi(os.Getenv("QDRANT_PORT"))
	useTLS, _ := strconv.ParseBool(os.Getenv("QDRANT_USE_TLS"))
	c, err := qdrant.New(qdrant.Config{
		Host:                   host,
		Port:                   port,
		UseTLS:                 useTLS,
		APIKey:                 os.Getenv("QDRANT_API_KEY"),
		SkipCompatibilityCheck: true,
	})
	if err != nil {
		t.Fatalf("dial qdrant: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestKnowledgeSourceGrouping_LiveQdrant(t *testing.T) {
	c := liveKnowledgeQdrantClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const dim = 8
	collection := fmt.Sprintf("knowledge_source_%d", time.Now().UnixNano())
	if err := c.EnsureCollection(ctx, qdrant.CollectionConfig{
		Name: collection, VectorDim: dim, OnDisk: false,
	}); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	t.Cleanup(func() { _ = c.DeleteCollection(context.Background(), collection) })

	orgID := "org-source-roundtrip"
	slackSource := map[string]any{
		"id":            "slack-source",
		"name":          "Engineering Slack",
		"kind":          "INTEGRATION",
		"provider":      "slack",
		"connection_id": "conn-slack",
	}
	docsSource := map[string]any{
		"id":       "docs-source",
		"name":     "Deploy Docs",
		"kind":     "FILE_UPLOAD",
		"provider": "file_upload",
	}
	points := []qdrant.Point{
		{
			ID:     qdrant.PointID(orgID, "slack-source", "deploy-1"),
			Vector: testVector(dim, 1),
			Payload: map[string]any{
				"org_id":        orgID,
				"rag_source_id": "slack-source",
				"source":        slackSource,
				"doc_id":        "deploy-1",
				"semantic_id":   "Deploy policy",
				"content":       "Release owners post status before deploy.",
				"is_public":     true,
			},
		},
		{
			ID:     qdrant.PointID(orgID, "slack-source", "deploy-2"),
			Vector: testVector(dim, 2),
			Payload: map[string]any{
				"org_id":        orgID,
				"rag_source_id": "slack-source",
				"source":        slackSource,
				"doc_id":        "deploy-2",
				"semantic_id":   "Rollback notes",
				"content":       "Deploys require rollback notes.",
				"is_public":     true,
			},
		},
		{
			ID:     qdrant.PointID(orgID, "docs-source", "deploy-guide"),
			Vector: testVector(dim, 3),
			Payload: map[string]any{
				"org_id":        orgID,
				"rag_source_id": "docs-source",
				"source":        docsSource,
				"doc_id":        "deploy-guide",
				"semantic_id":   "Deploy guide",
				"content":       "Run smoke tests after deploy.",
				"is_public":     true,
			},
		},
	}
	if err := c.Upsert(ctx, collection, points, true); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	hits, err := c.Search(ctx, qdrant.SearchRequest{
		Collection:  collection,
		Vector:      testVector(dim, 99),
		Filter:      qdrant.BuildACLFilter(orgID, nil, true),
		Limit:       10,
		WithPayload: true,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	groups := groupKnowledgeHitsBySource(hits)
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2: %#v", len(groups), groups)
	}

	groupByID := map[string]map[string]any{}
	for _, group := range groups {
		id, _ := group["source_id"].(string)
		groupByID[id] = group
	}
	slack := groupByID["slack-source"]
	if slack == nil {
		t.Fatalf("missing slack-source group: %#v", groups)
	}
	source, ok := slack["source"].(map[string]any)
	if !ok {
		t.Fatalf("slack source has type %T", slack["source"])
	}
	if source["name"] != "Engineering Slack" || source["provider"] != "slack" {
		t.Fatalf("slack source metadata did not survive qdrant round trip: %#v", source)
	}
	if slack["result_count"] != 2 {
		t.Fatalf("slack result_count = %v, want 2", slack["result_count"])
	}
}

func testVector(dim int, seed float32) []float32 {
	vector := make([]float32, dim)
	for i := range vector {
		vector[i] = seed + float32(i)/100
	}
	return vector
}
