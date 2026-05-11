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
				"source_type":   "slack",
				"title":         "Engineering channel",
				"content":       "Deploys require a rollback note.",
			},
		},
		{
			ID:    "chunk-2",
			Score: 0.8,
			Payload: map[string]any{
				"rag_source_id": "slack:C123",
				"source_type":   "slack",
				"content":       "Release owners post status before deploy.",
			},
		},
		{
			ID:    "chunk-3",
			Score: 0.7,
			Payload: map[string]any{
				"rag_source_id": "docs:deploys",
				"source_type":   "docs",
				"title":         "Deploy guide",
				"content":       "Run smoke tests after deploy.",
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
	if groups[1]["source_id"] != "docs:deploys" {
		t.Fatalf("second group source_id = %v", groups[1]["source_id"])
	}
}
