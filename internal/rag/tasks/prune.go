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
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
)

// HandlePrune is the asynq handler for TypeRagPrune. It drives the
// source's SlimConnector to enumerate every currently-present upstream
// doc, diffs the result against rag_documents to find IDs no longer
// present, and cascades the deletion to:
//
//   1. rag-engine via ragclient.Prune (vector + chunk delete)
//   2. rag_document_by_sources junction rows for this source
//   3. rag_documents rows that no source claims any more
//
// Port of try_creating_prune_generator_task at
// backend/onyx/background/celery/tasks/pruning/tasks.py:228-235.
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

	if _, err := deps.RagClient.Prune(ctx, &ragpb.PruneRequest{
		DatasetName:    deps.DatasetName,
		OrgId:          src.OrgIDValue.String(),
		KeepDocIds:     keep,
		IdempotencyKey: fmt.Sprintf("prune-%s-%d", src.ID, time.Now().Unix()),
	}); err != nil {
		return fmt.Errorf("prune %s: ragclient.Prune: %w", src.ID, err)
	}
	if err := deleteOrphanDocs(ctx, deps.DB, deletedIDs); err != nil {
		return err
	}
	return touchLastPruned(ctx, deps.DB, src.ID)
}

// drainSlim reads every SlimDocument the connector reports and returns
// the list of doc IDs that should be retained. Failures are logged but
// don't abort the prune (a failed entity simply isn't in the keep set,
// which would cause a deletion — so we instead return the error so
// callers don't accidentally delete docs because the source was flaky).
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

// pruneDeletedDocs removes the rag_document_by_sources rows for this
// source whose document_id is not in keep, returning the deleted IDs.
// keep being empty is permitted (means "delete every doc"); the caller
// is responsible for the safety check (the rag-engine refuses an
// empty keep set on the gRPC side).
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

// deleteOrphanDocs removes rag_documents rows that no longer have any
// source claim. We only delete the strict orphans — a document
// indexed by two sources where one prunes it stays in rag_documents.
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

// touchLastPruned advances the source's last_pruned column.
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

