package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

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
	vectors, err := deps.Embedder.Embed(ctx, contents)
	if err != nil {
		return fmt.Errorf("ingest: embed (%d docs): %w", len(docs), err)
	}

	points := make([]qdrant.Point, 0, len(docs))
	for i := range docs {
		d := &docs[i]
		points = append(points, qdrant.Point{
			ID:      qdrant.PointID(src.OrgIDValue.String(), d.DocID),
			Vector:  vectors[i],
			Payload: buildPayload(src, d, contents[i]),
		})
	}
	if err := deps.Qdrant.Upsert(ctx, deps.Collection, points, true); err != nil {
		return fmt.Errorf("ingest: qdrant upsert (%d docs): %w", len(docs), err)
	}
	return upsertDocsLocal(ctx, deps.DB, src, docs)
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

// Without these local rows the prune loop has nothing to diff against.
func upsertDocsLocal(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	docs []interfaces.Document,
) error {
	now := time.Now()
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range docs {
			d := &docs[i]
			row := ragmodel.RAGDocument{
				ID:                 d.DocID,
				OrgID:              src.OrgIDValue,
				SemanticID:         d.SemanticID,
				Link:               strPtr(d.Link),
				ExternalUserEmails: pq.StringArray(d.ACL),
				IsPublic:           d.IsPublic,
				LastModified:       now,
				LastSynced:         &now,
				DocUpdatedAt:       d.DocUpdatedAt,
			}
			if err := tx.Save(&row).Error; err != nil {
				return fmt.Errorf("upsert rag_document %s: %w", d.DocID, err)
			}
			edge := ragmodel.RAGDocumentBySource{
				DocumentID:     d.DocID,
				RAGSourceID:    src.ID,
				HasBeenIndexed: true,
			}
			if err := tx.Save(&edge).Error; err != nil {
				return fmt.Errorf("upsert rag_document_by_source %s: %w", d.DocID, err)
			}
		}
		return nil
	})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}
