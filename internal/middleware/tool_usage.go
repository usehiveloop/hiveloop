package middleware

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const toolUsageBatchSize = 50

// ToolUsageWriter is a buffered tool usage writer that never blocks the request hot path.
// Follows the same pattern as GenerationWriter.
type ToolUsageWriter struct {
	db            *gorm.DB
	entries       chan model.ToolUsage
	wg            sync.WaitGroup
	flushInterval time.Duration
	closeOnce     sync.Once
}

// NewToolUsageWriter creates a ToolUsageWriter with the given buffer size and starts
// background flushing. Call Shutdown to flush remaining entries on exit.
func NewToolUsageWriter(db *gorm.DB, bufferSize int) *ToolUsageWriter {
	writer := &ToolUsageWriter{
		db:            db,
		entries:       make(chan model.ToolUsage, bufferSize),
		flushInterval: 500 * time.Millisecond,
	}
	writer.wg.Add(1)
	go writer.drain()
	return writer
}

func (writer *ToolUsageWriter) drain() {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("tool usage drain panicked",
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
		}
		writer.wg.Done()
	}()

	batch := make([]model.ToolUsage, 0, toolUsageBatchSize)
	timer := time.NewTimer(writer.flushInterval)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := writer.db.CreateInBatches(batch, toolUsageBatchSize).Error; err != nil {
			slog.Error("tool usage batch write failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-writer.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= toolUsageBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(writer.flushInterval)
			}
		case <-timer.C:
			flush()
			timer.Reset(writer.flushInterval)
		}
	}
}

// Write queues a tool usage entry. It never blocks — if the buffer is full, the
// entry is dropped and a warning is logged.
func (writer *ToolUsageWriter) Write(usage model.ToolUsage) {
	select {
	case writer.entries <- usage:
	default:
		slog.Warn("tool usage buffer full, dropping entry", "id", usage.ID)
	}
}

// Shutdown closes the channel and waits for all queued entries to be flushed.
// Safe to call multiple times.
func (writer *ToolUsageWriter) Shutdown(ctx context.Context) {
	writer.closeOnce.Do(func() {
		close(writer.entries)
	})

	done := make(chan struct{})
	goroutine.Go(ctx, func(context.Context) {
		writer.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("tool usage shutdown timed out, some entries may be lost")
	}
}
