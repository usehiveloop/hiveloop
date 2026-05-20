package handler

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
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
			logging.CaptureWithFields(ctx, fmt.Errorf("employee event drain panicked: %v", r), map[string]any{
				"stage": "employee_event_drain_panic",
			})
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
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.CreateInBatches(batch, employeeEventBatchSize).Error; err != nil {
				return err
			}
			for _, entry := range batch {
				if err := syncEmployeeScheduleEvent(tx, entry); err != nil {
					captureEmployeeMemoryEventFailure(ctx, "batch_sync_schedule", entry, err)
					logging.FromContext(ctx).WarnContext(ctx, "employee schedule sync failed",
						"error", err,
						"event_type", entry.EventType,
						"agent_id", entry.AgentID,
					)
				}
			}
			return nil
		})
		if err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("employee event batch write failed: %w", err), employeeEventBatchSentryFields("batch_write", batch))
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
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			if err := syncEmployeeScheduleEvent(tx, entry); err != nil {
				captureEmployeeMemoryEventFailure(ctx, "direct_sync_schedule", entry, err)
				logging.FromContext(ctx).WarnContext(ctx, "employee schedule sync failed",
					"error", err,
					"event_type", entry.EventType,
					"agent_id", entry.AgentID,
				)
			}
			return nil
		})
		if err != nil {
			captureEmployeeMemoryEventFailure(ctx, "direct_write", entry, err)
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

func employeeEventBatchSentryFields(stage string, batch []model.EmployeeMemoryEvent) map[string]any {
	fields := map[string]any{
		"stage": stage,
		"count": len(batch),
	}
	if len(batch) == 0 {
		return fields
	}
	first := batch[0]
	fields["org_id"] = first.OrgID.String()
	fields["agent_id"] = first.AgentID.String()
	fields["sandbox_id"] = first.SandboxID.String()
	fields["session_id"] = first.SessionID
	fields["event_type"] = first.EventType
	fields["source"] = first.Source
	return fields
}
