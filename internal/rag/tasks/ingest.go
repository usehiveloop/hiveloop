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

// pollOverlap is how far the next poll's window-start is rewound past
// the previous run's last_successful_index_time, so updates that
// landed in the upstream just before our last fetch are still picked
// up. Mirrors Onyx's POLL_CONNECTOR_OFFSET
// (backend/onyx/background/indexing/run_docfetching.py:445).
const pollOverlap = 5 * time.Minute

// computeIngestWindow returns the [start, end] time range the
// connector should fetch updates within. Port of Onyx's window
// computation at run_docfetching.py:419-451.
//
//	earliest = src.IndexingStart ?? epoch
//	if fromBeginning OR src.LastSuccessfulIndexTime IS NULL:
//	    start = earliest
//	else:
//	    start = max(earliest, src.LastSuccessfulIndexTime - 5min)
//	end = now
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

// errFatalConnector wraps a fatal connector-level failure (e.g. bad
// credentials, missing repo). The ingest handler unwraps it to mark the
// attempt FAILED with the underlying message.
type errFatalConnector struct{ inner error }

func (e *errFatalConnector) Error() string { return e.inner.Error() }
func (e *errFatalConnector) Unwrap() error { return e.inner }

// HandleIngest is the asynq handler for TypeRagIngest. End-to-end:
//
//	1. Load the rag_sources row by ID.
//	2. Look up + build the connector via the registry; type-assert
//	   for RunnableCheckpointed.
//	3. Open a fresh rag_index_attempts row in IN_PROGRESS state.
//	4. Start the heartbeat goroutine; defer its stop.
//	5. Drive Run() — drain documents into batches, push each batch
//	   through ragclient.IngestBatch, upsert per-doc rows in Postgres,
//	   record per-doc failures.
//	6. On clean completion, mark the attempt SUCCESS or
//	   COMPLETED_WITH_ERRORS, persist the final checkpoint, advance the
//	   source's last_successful_index_time + flip
//	   INITIAL_INDEXING → ACTIVE.
//
// Port of Onyx's docfetching_task at
// backend/onyx/background/celery/tasks/docfetching/tasks.py:103-258
// plus the docprocessing pipeline that follows it; we collapse the two
// into one handler because the gRPC IngestBatch combines the two
// operations server-side.
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

	hb := startHeartbeat(ctx, deps.DB, attempt.ID, deps.HeartbeatTick)
	defer hb.stop()

	stats, runErr := runIngest(ctx, deps, src, runnable, attempt, hb)

	finalErr := finalizeAttempt(ctx, deps.DB, src, attempt, stats, runErr, runnable)
	// Stop heartbeat BEFORE returning so no further writes race the
	// finalisation UPDATE. The defer is a backstop.
	hb.stop()
	return finalErr
}

// loadSource fetches the rag_sources row by ID. A missing row returns
// asynq.SkipRetry so the worker doesn't loop on a tombstoned source.
func loadSource(ctx context.Context, db *gorm.DB, id uuid.UUID) (*ragmodel.RAGSource, error) {
	var src ragmodel.RAGSource
	if err := db.WithContext(ctx).First(&src, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("ingest: source %s: %w", id, asynq.SkipRetry)
		}
		return nil, fmt.Errorf("ingest: load source %s: %w", id, err)
	}
	return &src, nil
}

// buildConnector resolves the registered factory for the source's kind
// and constructs the per-source connector instance.
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

// ingestStats accumulates counters drained from the connector channel.
// Persisted onto the attempt row at finalisation time.
type ingestStats struct {
	docsSeen     int
	docsBatched  int
	failures     int
	pollStart    time.Time
	pollEnd      time.Time
}

// runIngest drains the connector's document stream into batches,
// flushes each batch through ragclient.IngestBatch, and records
// per-doc failures into rag_index_attempt_errors. Returns a
// non-nil error only on fatal stream-open failure or context
// cancellation; per-doc errors are accumulated in stats.failures.
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
			if err := flushBatch(ctx, deps, src, attempt, batch); err != nil {
				return stats, err
			}
			stats.docsBatched += len(batch)
			batch = batch[:0]
			hb.touchProgress()
		}
	}
	if len(batch) > 0 {
		if err := flushBatch(ctx, deps, src, attempt, batch); err != nil {
			return stats, err
		}
		stats.docsBatched += len(batch)
		hb.touchProgress()
	}
	stats.pollEnd = windowEnd
	return stats, nil
}

// loadCheckpointBytes returns the raw checkpoint bytes persisted on a
// previous attempt (or empty for the first run).
func loadCheckpointBytes(a *ragmodel.RAGIndexAttempt) []byte {
	if a.CheckpointPointer == nil || *a.CheckpointPointer == "" {
		return nil
	}
	return []byte(*a.CheckpointPointer)
}

// fatal returns the underlying error if err is an errFatalConnector
// wrapper, otherwise nil. Used by finalizeAttempt to distinguish
// "stream open failed" from "context cancelled".
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

