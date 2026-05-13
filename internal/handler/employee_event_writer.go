package handler

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const employeeEventBatchSize = 100

type EmployeeEventWriter struct {
	db            *gorm.DB
	entries       chan model.EmployeeMemoryEvent
	wg            sync.WaitGroup
	flushInterval time.Duration
}

func NewEmployeeEventWriter(ctx context.Context, db *gorm.DB, bufferSize int, flushInterval ...time.Duration) *EmployeeEventWriter {
	interval := 500 * time.Millisecond
	if len(flushInterval) > 0 {
		interval = flushInterval[0]
	}
	w := &EmployeeEventWriter{
		db:            db,
		entries:       make(chan model.EmployeeMemoryEvent, bufferSize),
		flushInterval: interval,
	}
	w.wg.Add(1)
	go w.drain(ctx)
	return w
}

func (w *EmployeeEventWriter) drain(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "employee event drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
		w.wg.Done()
	}()

	batch := make([]model.EmployeeMemoryEvent, 0, employeeEventBatchSize)
	timer := time.NewTimer(w.flushInterval)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.db.CreateInBatches(batch, employeeEventBatchSize).Error; err != nil {
			logging.Capture(ctx, err)
			logging.FromContext(ctx).ErrorContext(ctx, "employee event batch write failed", "error", err, "count", len(batch))
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-w.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= employeeEventBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.flushInterval)
			}
		case <-timer.C:
			flush()
			timer.Reset(w.flushInterval)
		}
	}
}

func (w *EmployeeEventWriter) Write(ctx context.Context, entry model.EmployeeMemoryEvent) {
	if w == nil {
		return
	}
	select {
	case w.entries <- entry:
	default:
		if err := w.db.WithContext(ctx).Create(&entry).Error; err != nil {
			logging.Capture(ctx, err)
			logging.FromContext(ctx).ErrorContext(ctx, "employee event direct write failed", "error", err, "event_type", entry.EventType)
		}
	}
}

func (w *EmployeeEventWriter) Shutdown(ctx context.Context) {
	if w == nil {
		return
	}
	close(w.entries)

	done := make(chan struct{})
	goroutine.Go(ctx, func(context.Context) {
		w.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		logging.FromContext(ctx).WarnContext(ctx, "employee event shutdown timed out, some entries may be lost")
	}
}
