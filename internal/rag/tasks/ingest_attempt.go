package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

var errIngestAttemptAlreadyClaimed = errors.New("ingest attempt already claimed")

func openAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	fromBeginning bool,
	attemptID *uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	if attemptID != nil {
		return claimReservedAttempt(ctx, db, src, *attemptID)
	}
	return createAdHocAttempt(ctx, db, src, fromBeginning)
}

func createAdHocAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	fromBeginning bool,
) (*ragmodel.RAGIndexAttempt, error) {
	now := time.Now()
	a := &ragmodel.RAGIndexAttempt{
		OrgID:            src.OrgIDValue,
		RAGSourceID:      src.ID,
		FromBeginning:    fromBeginning,
		Status:           ragmodel.IndexingStatusInProgress,
		LastProgressTime: &now,
		TimeStarted:      &now,
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSourceForAttempt(ctx, tx, src.ID); err != nil {
			return err
		}
		active, err := activeAttemptExists(ctx, tx, src.ID, nil)
		if err != nil {
			return err
		}
		if active {
			return errIngestAttemptAlreadyClaimed
		}
		if !fromBeginning {
			if cp := lastCheckpointPointer(ctx, tx, src.ID); cp != "" {
				a.CheckpointPointer = &cp
			}
		}
		if err := tx.WithContext(ctx).Create(a).Error; err != nil {
			return fmt.Errorf("create attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errIngestAttemptAlreadyClaimed) {
			return nil, err
		}
		return nil, fmt.Errorf("ingest: open attempt for %s: %w", src.ID, err)
	}
	return a, nil
}

func claimReservedAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	attemptID uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	now := time.Now()
	var attempt ragmodel.RAGIndexAttempt
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSourceForAttempt(ctx, tx, src.ID); err != nil {
			return err
		}
		active, err := activeAttemptExists(ctx, tx, src.ID, &attemptID)
		if err != nil {
			return err
		}
		if active {
			return errIngestAttemptAlreadyClaimed
		}

		res := tx.Model(&ragmodel.RAGIndexAttempt{}).
			Where("id = ? AND rag_source_id = ? AND status = ?", attemptID, src.ID, ragmodel.IndexingStatusNotStarted).
			Updates(map[string]any{
				"status":             ragmodel.IndexingStatusInProgress,
				"last_progress_time": now,
				"time_started":       now,
				"time_updated":       now,
			})
		if res.Error != nil {
			return fmt.Errorf("claim reserved attempt: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			if err := tx.First(&attempt, "id = ? AND rag_source_id = ?", attemptID, src.ID).Error; err != nil {
				return fmt.Errorf("load reserved attempt: %w", err)
			}
			return errIngestAttemptAlreadyClaimed
		}
		if err := tx.First(&attempt, "id = ?", attemptID).Error; err != nil {
			return fmt.Errorf("load claimed attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errIngestAttemptAlreadyClaimed) {
			return nil, err
		}
		return nil, fmt.Errorf("ingest: claim reserved attempt %s for %s: %w", attemptID, src.ID, err)
	}
	return &attempt, nil
}

func lockSourceForAttempt(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID) error {
	var id string
	res := tx.WithContext(ctx).Raw("SELECT id FROM rag_sources WHERE id = ? FOR UPDATE", sourceID).Scan(&id)
	if res.Error != nil {
		return fmt.Errorf("lock source %s: %w", sourceID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("lock source %s: %w", sourceID, gorm.ErrRecordNotFound)
	}
	return nil
}

func activeAttemptExists(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID, exceptID *uuid.UUID) (bool, error) {
	q := tx.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND status IN ?", sourceID, []ragmodel.IndexingStatus{
			ragmodel.IndexingStatusNotStarted,
			ragmodel.IndexingStatusInProgress,
		})
	if exceptID != nil {
		q = q.Where("id <> ?", *exceptID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, fmt.Errorf("count active attempts: %w", err)
	}
	return count > 0, nil
}

func lastCheckpointPointer(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) string {
	var cp *string
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND checkpoint_pointer IS NOT NULL", sourceID).
		Order("time_created DESC").
		Limit(1).
		Pluck("checkpoint_pointer", &cp).Error; err != nil {
		return ""
	}
	if cp == nil {
		return ""
	}
	return *cp
}
