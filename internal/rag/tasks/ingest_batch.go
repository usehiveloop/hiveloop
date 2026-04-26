package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// flushBatch ships one batch of Documents to the rag-engine via gRPC
// IngestBatch and upserts the local rag_documents metadata + the
// rag_document_by_sources junction rows. Any error returned aborts the
// run — IngestBatch failures aren't recoverable mid-run because the
// idempotency key is per-batch and a partial server-side write can't
// be safely re-tried in-place.
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
	req := buildIngestRequest(deps, src, attempt, docs)
	if _, err := deps.RagClient.IngestBatch(ctx, req); err != nil {
		return fmt.Errorf("ingest: IngestBatch (%d docs): %w", len(docs), err)
	}
	return upsertDocsLocal(ctx, deps.DB, src, docs)
}

// buildIngestRequest converts the Document slice into the gRPC request.
// The idempotency key embeds attempt ID + the first doc ID + batch
// size so the same batch retried within the same attempt collapses
// server-side, while a different attempt for the same docs gets its
// own idempotency space.
func buildIngestRequest(
	deps *Deps,
	src *ragmodel.RAGSource,
	attempt *ragmodel.RAGIndexAttempt,
	docs []interfaces.Document,
) *ragpb.IngestBatchRequest {
	pbDocs := make([]*ragpb.DocumentToIngest, 0, len(docs))
	for i := range docs {
		pbDocs = append(pbDocs, toPBDocument(&docs[i]))
	}
	return &ragpb.IngestBatchRequest{
		DatasetName:       deps.DatasetName,
		OrgId:             src.OrgIDValue.String(),
		Mode:              ragpb.IngestionMode_INGESTION_MODE_UPSERT,
		IdempotencyKey:    fmt.Sprintf("attempt-%s-batch-%s-%d", attempt.ID, docs[0].DocID, len(docs)),
		DeclaredVectorDim: deps.DeclaredVectorDim,
		Documents:         pbDocs,
	}
}

// toPBDocument converts a connector-side Document to the gRPC shape.
func toPBDocument(d *interfaces.Document) *ragpb.DocumentToIngest {
	pb := &ragpb.DocumentToIngest{
		DocId:           d.DocID,
		SemanticId:      d.SemanticID,
		Link:            d.Link,
		Acl:             append([]string(nil), d.ACL...),
		IsPublic:        d.IsPublic,
		Metadata:        d.Metadata,
		PrimaryOwners:   append([]string(nil), d.PrimaryOwners...),
		SecondaryOwners: append([]string(nil), d.SecondaryOwners...),
	}
	if d.DocUpdatedAt != nil {
		pb.DocUpdatedAt = timestamppb.New(*d.DocUpdatedAt)
	}
	for i := range d.Sections {
		pb.Sections = append(pb.Sections, &ragpb.Section{
			Text:  d.Sections[i].Text,
			Link:  d.Sections[i].Link,
			Title: d.Sections[i].Title,
		})
	}
	return pb
}

// upsertDocsLocal writes the local rag_documents rows + the
// rag_document_by_sources junction rows. Mirrors what Onyx's
// upsert_documents() does at backend/onyx/db/document.py — without it
// the prune loop has nothing to diff against.
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
				ID:                   d.DocID,
				OrgID:                src.OrgIDValue,
				SemanticID:           d.SemanticID,
				Link:                 strPtr(d.Link),
				ExternalUserEmails:   pq.StringArray(d.ACL), // opaque tokens stored as emails
				ExternalUserGroupIDs: nil,
				IsPublic:             d.IsPublic,
				LastModified:         now,
				LastSynced:           &now,
				DocUpdatedAt:         d.DocUpdatedAt,
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
