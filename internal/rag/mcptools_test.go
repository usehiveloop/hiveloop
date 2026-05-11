package rag

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func TestGroupKnowledgeHitsBySource(t *testing.T) {
	hits := []qdrant.Hit{
		{
			ID:    "chunk-1",
			Score: 0.9,
			Payload: map[string]any{
				"rag_source_id": "slack:C123",
				"source": map[string]any{
					"id":       "slack:C123",
					"name":     "Engineering channel",
					"kind":     "INTEGRATION",
					"provider": "slack",
				},
				"semantic_id": "Engineering deploy policy",
				"content":     "Deploys require a rollback note.",
			},
		},
		{
			ID:    "chunk-2",
			Score: 0.8,
			Payload: map[string]any{
				"rag_source_id": "slack:C123",
				"source": map[string]any{
					"id":       "slack:C123",
					"name":     "Engineering channel",
					"kind":     "INTEGRATION",
					"provider": "slack",
				},
				"content": "Release owners post status before deploy.",
			},
		},
		{
			ID:    "chunk-3",
			Score: 0.7,
			Payload: map[string]any{
				"rag_source_id": "docs:deploys",
				"source": map[string]any{
					"id":   "docs:deploys",
					"name": "Deploy docs",
					"kind": "FILE_UPLOAD",
				},
				"title":   "Deploy guide",
				"content": "Run smoke tests after deploy.",
			},
		},
	}

	groups := groupKnowledgeHitsBySource(hits)
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2", len(groups))
	}
	if groups[0]["source_id"] != "slack:C123" {
		t.Fatalf("first group source_id = %v", groups[0]["source_id"])
	}
	if groups[0]["result_count"] != 2 {
		t.Fatalf("first group result_count = %v, want 2", groups[0]["result_count"])
	}
	source, ok := groups[0]["source"].(map[string]any)
	if !ok {
		t.Fatalf("first group source has type %T", groups[0]["source"])
	}
	if source["kind"] != "INTEGRATION" {
		t.Fatalf("first group source.kind = %v, want INTEGRATION", source["kind"])
	}
	if source["provider"] != "slack" {
		t.Fatalf("first group source.provider = %v, want slack", source["provider"])
	}
	if source["name"] != "Engineering channel" {
		t.Fatalf("first group source.name = %v", source["name"])
	}
	chunks, ok := groups[0]["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("first group chunks has type %T", groups[0]["chunks"])
	}
	if len(chunks) != 2 {
		t.Fatalf("first group chunks = %d, want 2", len(chunks))
	}
	if chunks[0]["title"] != "Engineering deploy policy" {
		t.Fatalf("first chunk title = %v", chunks[0]["title"])
	}
	if groups[1]["source_id"] != "docs:deploys" {
		t.Fatalf("second group source_id = %v", groups[1]["source_id"])
	}
}

func TestGroupKnowledgeHitsBySourceFallsBackForLegacyPayload(t *testing.T) {
	groups := groupKnowledgeHitsBySource([]qdrant.Hit{
		{
			ID:    "chunk-1",
			Score: 0.9,
			Payload: map[string]any{
				"rag_source_id": "source-1",
				"link":          "https://github.com/acme/api/pull/10",
				"content":       "Use class components in this repo.",
			},
		},
	})
	if len(groups) != 1 {
		t.Fatalf("group count = %d, want 1", len(groups))
	}
	if groups[0]["source_id"] != "source-1" {
		t.Fatalf("source_id = %v, want source-1", groups[0]["source_id"])
	}
}
