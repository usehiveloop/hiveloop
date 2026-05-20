package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

func stampDocsEstimated(
	ctx context.Context,
	db *gorm.DB,
	conn interfaces.Connector,
	src *ragmodel.RAGSource,
	attempt *ragmodel.RAGIndexAttempt,
) {
	est, ok := conn.(interfaces.EstimatingConnector)
	if !ok {
		return
	}
	count, err := est.EstimateTotal(ctx, src)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("rag ingest estimate total source=%s attempt=%s: %w", src.ID, attempt.ID, err))
		return
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", attempt.ID).
		Update("docs_estimated", count).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("rag ingest persist docs_estimated attempt=%s: %w", attempt.ID, err))
		return
	}
	attempt.DocsEstimated = &count
}

func bumpAttemptProgress(
	ctx context.Context,
	db *gorm.DB,
	attemptID uuid.UUID,
	delta int,
) {
	if delta <= 0 {
		return
	}
	now := time.Now()
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", attemptID).
		Updates(map[string]any{
			"new_docs_indexed":   gorm.Expr("COALESCE(new_docs_indexed, 0) + ?", delta),
			"total_docs_indexed": gorm.Expr("COALESCE(total_docs_indexed, 0) + ?", delta),
			"last_progress_time": now,
			"time_updated":       now,
		}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("rag ingest bump progress attempt=%s: %w", attemptID, err))
	}
}

// We don't fail the whole attempt over an error-log INSERT failure,
// but we surface it via Warn.
func recordAttemptError(
	ctx context.Context,
	db *gorm.DB,
	orgID, sourceID, attemptID uuid.UUID,
	failure *interfaces.ConnectorFailure,
) {
	if failure == nil {
		return
	}
	row := ragmodel.RAGIndexAttemptError{
		OrgID:          orgID,
		IndexAttemptID: attemptID,
		RAGSourceID:    sourceID,
		FailureMessage: failure.FailureMessage,
	}
	if failure.FailedDocument != nil {
		id := failure.FailedDocument.DocID
		row.DocumentID = &id
		if failure.FailedDocument.DocumentLink != "" {
			link := failure.FailedDocument.DocumentLink
			row.DocumentLink = &link
		}
	}
	if failure.FailedEntity != nil {
		eid := failure.FailedEntity.EntityID
		row.EntityID = &eid
		row.FailedTimeRangeStart = failure.FailedEntity.MissedTimeRangeStart
		row.FailedTimeRangeEnd = failure.FailedEntity.MissedTimeRangeEnd
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("rag ingest record attempt error attempt=%s: %w", attemptID, err))
	}
}
