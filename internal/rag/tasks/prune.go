package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/qdrant"
)

func (d *Deps) HandlePrune(ctx context.Context, t *asynq.Task) error {
	deps := d.withDefaults()
	payload, err := UnmarshalPrune(t.Payload())
	if err != nil {
		return err
	}
	src, err := loadSource(ctx, deps.DB, payload.RAGSourceID)
	if err != nil {
		return err
	}
	conn, err := buildConnector(src, deps)
	if err != nil {
		return err
	}
	slim, ok := conn.(interfaces.SlimConnector)
	if !ok {
		return fmt.Errorf("prune %s: connector kind %q does not implement SlimConnector",
			src.ID, src.KindValue)
	}

	keep, err := drainSlim(ctx, slim, src)
	if err != nil {
		return fmt.Errorf("prune %s: list slim docs: %w", src.ID, err)
	}

	deletedIDs, err := pruneDeletedDocs(ctx, deps.DB, src.ID, keep)
	if err != nil {
		return err
	}
	if len(deletedIDs) == 0 {
		return touchLastPruned(ctx, deps.DB, src.ID)
	}

	pointIDs := make([]string, len(deletedIDs))
	for i, docID := range deletedIDs {
		pointIDs[i] = qdrant.PointID(src.OrgIDValue.String(), docID)
	}
	if err := deps.Qdrant.DeleteByIDs(ctx, deps.Collection, pointIDs); err != nil {
		return fmt.Errorf("prune %s: qdrant delete: %w", src.ID, err)
	}
	if err := deleteOrphanDocs(ctx, deps.DB, deletedIDs); err != nil {
		return err
	}
	return touchLastPruned(ctx, deps.DB, src.ID)
}

func drainSlim(
	ctx context.Context,
	slim interfaces.SlimConnector,
	src *ragmodel.RAGSource,
) ([]string, error) {
	stream, err := slim.ListAllSlim(ctx, src)
	if err != nil {
		return nil, err
	}
	var keep []string
	for item := range stream {
		if item.Failure != nil {
			return nil, fmt.Errorf("slim listing failure: %s", item.Failure.FailureMessage)
		}
		if item.Slim == nil {
			continue
		}
		keep = append(keep, item.Slim.DocID)
	}
	return keep, nil
}

func pruneDeletedDocs(
	ctx context.Context,
	db *gorm.DB,
	sourceID uuid.UUID,
	keep []string,
) ([]string, error) {
	var deletedIDs []string
	q := db.WithContext(ctx).
		Model(&ragmodel.RAGDocumentBySource{}).
		Where("rag_source_id = ?", sourceID)
	if len(keep) > 0 {
		q = q.Where("document_id NOT IN ?", keep)
	}
	if err := q.Pluck("document_id", &deletedIDs).Error; err != nil {
		return nil, fmt.Errorf("prune: select deleted ids: %w", err)
	}
	if len(deletedIDs) == 0 {
		return nil, nil
	}
	if err := db.WithContext(ctx).
		Where("rag_source_id = ? AND document_id IN ?", sourceID, deletedIDs).
		Delete(&ragmodel.RAGDocumentBySource{}).Error; err != nil {
		return nil, fmt.Errorf("prune: delete junction rows: %w", err)
	}
	return deletedIDs, nil
}

// A doc shared by two sources stays in rag_documents — only strict orphans go.
func deleteOrphanDocs(ctx context.Context, db *gorm.DB, candidateIDs []string) error {
	if len(candidateIDs) == 0 {
		return nil
	}
	const q = `
		DELETE FROM rag_documents
		WHERE id IN (?)
		  AND NOT EXISTS (
		    SELECT 1 FROM rag_document_by_sources e
		    WHERE e.document_id = rag_documents.id
		  )
	`
	if err := db.WithContext(ctx).Exec(q, candidateIDs).Error; err != nil {
		return fmt.Errorf("prune: delete orphan docs: %w", err)
	}
	slog.Info("prune: deleted orphan docs", "count", len(candidateIDs))
	return nil
}

func touchLastPruned(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) error {
	now := time.Now()
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGSource{}).
		Where("id = ?", sourceID).
		Updates(map[string]any{
			"last_pruned": now,
			"updated_at":  now,
		}).Error; err != nil {
		return fmt.Errorf("prune: advance last_pruned %s: %w", sourceID, err)
	}
	return nil
}
