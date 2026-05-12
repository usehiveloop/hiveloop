package slack

import (
	"context"
	"encoding/json"
	"testing"

	qdrantgo "github.com/qdrant/go-client/qdrant"

	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func scrollSlackRAGPoints(t *testing.T, ctx context.Context, qd *qdrant.Client, collection, orgID, sourceID string) []qdrant.ScrolledPoint {
	t.Helper()
	var points []qdrant.ScrolledPoint
	var offset *qdrantgo.PointId
	for {
		page, err := qd.Scroll(ctx, qdrant.ScrollRequest{
			Collection:  collection,
			Filter:      qdrant.BuildSourceFilter(orgID, sourceID),
			Limit:       100,
			Offset:      offset,
			WithPayload: true,
		})
		if err != nil {
			t.Fatalf("scroll qdrant: %v", err)
		}
		points = append(points, page.Points...)
		if page.NextOffset == nil {
			return points
		}
		offset = page.NextOffset
	}
}

func compactPointDump(points []qdrant.ScrolledPoint, limit int) string {
	if len(points) < limit {
		limit = len(points)
	}
	rows := make([]map[string]any, 0, limit)
	for i := 0; i < limit; i++ {
		payload := points[i].Payload
		content, _ := payload["content"].(string)
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		rows = append(rows, map[string]any{
			"id":          points[i].ID,
			"doc_id":      payload["doc_id"],
			"semantic_id": payload["semantic_id"],
			"link":        payload["link"],
			"source":      payload["source"],
			"metadata":    payload["metadata"],
			"content":     content,
		})
	}
	raw, _ := json.MarshalIndent(rows, "", "  ")
	return string(raw)
}
