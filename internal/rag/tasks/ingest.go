package tasks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// pollOverlap rewinds the window-start past the previous run's
// last_successful_index_time so updates that landed in the upstream
// just before our last fetch are still picked up.
const pollOverlap = 5 * time.Minute

func computeIngestWindow(src *ragmodel.RAGSource, fromBeginning bool) (time.Time, time.Time) {
	var earliest time.Time
	if src.IndexingStart != nil {
		earliest = *src.IndexingStart
	}
	end := time.Now()
	if fromBeginning || src.LastSuccessfulIndexTime == nil {
		return earliest, end
	}
	start := src.LastSuccessfulIndexTime.Add(-pollOverlap)
	if start.Before(earliest) {
		start = earliest
	}
	return start, end
}

type errFatalConnector struct{ inner error }

func (e *errFatalConnector) Error() string { return e.inner.Error() }
func (e *errFatalConnector) Unwrap() error { return e.inner }

func (d *Deps) HandleIngest(ctx context.Context, t *asynq.Task) error {
	deps := d.withDefaults()
	payload, err := UnmarshalIngest(t.Payload())
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
	runnable, ok := conn.(RunnableCheckpointed)
	if !ok {
		return fmt.Errorf("ingest %s: connector kind %q does not implement RunnableCheckpointed",
			src.ID, src.KindValue)
	}

	attempt, err := openAttempt(ctx, deps.DB, src, payload.FromBeginning)
	if err != nil {
		return err
	}

	stampDocsEstimated(ctx, deps.DB, conn, src, attempt)

	hb := startHeartbeat(ctx, deps.DB, attempt.ID, deps.HeartbeatTick)
	defer hb.stop()

	stats, runErr := runIngest(ctx, deps, src, runnable, attempt, hb)

	finalErr := finalizeAttempt(ctx, deps, src, attempt, stats, runErr, runnable)
	// Stop heartbeat BEFORE returning so no further writes race the
	// finalisation UPDATE. The defer is a backstop.
	hb.stop()
	return finalErr
}

// A missing source returns asynq.SkipRetry so the worker doesn't loop
// on a tombstoned row. Preloads InConnection so SourceKind() can
// resolve "INTEGRATION" → upstream provider for the connector lookup.
func loadSource(ctx context.Context, db *gorm.DB, id uuid.UUID) (*ragmodel.RAGSource, error) {
	var src ragmodel.RAGSource
	if err := db.WithContext(ctx).
		Preload("InConnection.InIntegration").
		First(&src, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("ingest: source %s: %w", id, asynq.SkipRetry)
		}
		return nil, fmt.Errorf("ingest: load source %s: %w", id, err)
	}
	return &src, nil
}

func buildConnector(src *ragmodel.RAGSource, deps *Deps) (interfaces.Connector, error) {
	factory, err := interfaces.Lookup(src.SourceKind())
	if err != nil {
		return nil, fmt.Errorf("ingest: lookup connector %q: %w", src.SourceKind(), err)
	}
	c, err := factory(src, deps.Nango)
	if err != nil {
		return nil, fmt.Errorf("ingest: build connector %q: %w", src.SourceKind(), err)
	}
	return c, nil
}

type ingestStats struct {
	docsSeen    int
	docsBatched int
	failures    int
	pollStart   time.Time
	pollEnd     time.Time
}

// runIngest returns a non-nil error only on fatal stream-open failure
// or context cancellation; per-doc errors are accumulated in
// stats.failures.
func runIngest(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	runnable RunnableCheckpointed,
	attempt *ragmodel.RAGIndexAttempt,
	hb *heartbeatHandle,
) (ingestStats, error) {
	windowStart, windowEnd := computeIngestWindow(src, attempt.FromBeginning)
	stats := ingestStats{
		pollStart: windowStart,
	}
	checkpointBytes := loadCheckpointBytes(attempt)
	stream, err := runnable.Run(ctx, src, checkpointBytes, windowStart, windowEnd)
	if err != nil {
		return stats, &errFatalConnector{inner: err}
	}

	batch := make([]interfaces.Document, 0, deps.BatchSize)
	for item := range stream {
		if item.Failure != nil {
			stats.failures++
			recordAttemptError(ctx, deps.DB, src.OrgIDValue, src.ID, attempt.ID, item.Failure)
			continue
		}
		if item.Doc == nil {
			continue
		}
		stats.docsSeen++
		batch = append(batch, *item.Doc)
		if len(batch) >= deps.BatchSize {
			n := len(batch)
			if err := flushBatch(ctx, deps, src, attempt, batch); err != nil {
				return stats, err
			}
			stats.docsBatched += n
			bumpAttemptProgress(ctx, deps.DB, attempt.ID, n)
			batch = batch[:0]
			hb.touchProgress()
		}
	}
	if len(batch) > 0 {
		n := len(batch)
		if err := flushBatch(ctx, deps, src, attempt, batch); err != nil {
			return stats, err
		}
		stats.docsBatched += n
		bumpAttemptProgress(ctx, deps.DB, attempt.ID, n)
		hb.touchProgress()
	}
	stats.pollEnd = windowEnd
	return stats, nil
}

func loadCheckpointBytes(a *ragmodel.RAGIndexAttempt) []byte {
	if a.CheckpointPointer == nil || *a.CheckpointPointer == "" {
		return nil
	}
	return []byte(*a.CheckpointPointer)
}

func fatal(err error) error {
	if err == nil {
		return nil
	}
	var fc *errFatalConnector
	if errors.As(err, &fc) {
		return fc.inner
	}
	return err
}

