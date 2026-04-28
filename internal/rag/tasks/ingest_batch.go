package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func flushBatch(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	attempt *ragmodel.RAGIndexAttempt,
	docs []interfaces.Document,
) error {
	if len(docs) == 0 {
		return nil
	}
	contents := make([]string, len(docs))
	for i := range docs {
		contents[i] = renderContent(&docs[i])
	}
	embedInputs := make([]string, len(contents))
	for i, c := range contents {
		embedInputs[i] = clampForEmbed(c)
	}
	vectors, err := deps.Embedder.Embed(ctx, embedInputs)
	if err != nil {
		return fmt.Errorf("ingest: embed (%d docs): %w", len(docs), err)
	}

	points := make([]qdrant.Point, 0, len(docs))
	orgID := src.OrgIDValue.String()
	sourceID := src.ID.String()
	for i := range docs {
		d := &docs[i]
		points = append(points, qdrant.Point{
			ID:      qdrant.PointID(orgID, sourceID, d.DocID),
			Vector:  vectors[i],
			Payload: buildPayload(src, d, contents[i]),
		})
	}
	if err := deps.Qdrant.Upsert(ctx, deps.Collection, points, true); err != nil {
		return fmt.Errorf("ingest: qdrant upsert (%d docs): %w", len(docs), err)
	}
	return nil
}

// ~6k tokens; under the 8192 ceiling shared by most embedders.
const embedInputCharLimit = 24000

func clampForEmbed(s string) string {
	if len(s) <= embedInputCharLimit {
		return s
	}
	return s[:embedInputCharLimit]
}

func renderContent(d *interfaces.Document) string {
	parts := make([]string, 0, len(d.Sections)+1)
	if d.SemanticID != "" {
		parts = append(parts, d.SemanticID)
	}
	for i := range d.Sections {
		s := &d.Sections[i]
		if s.Title != "" {
			parts = append(parts, s.Title)
		}
		if s.Text != "" {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func buildPayload(src *ragmodel.RAGSource, d *interfaces.Document, content string) map[string]any {
	pl := map[string]any{
		"org_id":         src.OrgIDValue.String(),
		"rag_source_id":  src.ID.String(),
		"doc_id":         d.DocID,
		"semantic_id":    d.SemanticID,
		"link":           d.Link,
		"acl":            append([]string(nil), d.ACL...),
		"is_public":      d.IsPublic,
		"content":        content,
		"primary_owners": append([]string(nil), d.PrimaryOwners...),
	}
	if d.DocUpdatedAt != nil {
		pl["doc_updated_at"] = d.DocUpdatedAt.Unix()
	}
	if d.Metadata != nil {
		pl["metadata"] = d.Metadata
	}
	return pl
}
