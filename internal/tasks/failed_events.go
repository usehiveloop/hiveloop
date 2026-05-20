package tasks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

type FailedEventInput struct {
	OrgID        uuid.UUID
	TriggerID    uuid.UUID
	EventType    string
	Payload      []byte
	Err          error
	AttemptCount int
}

func PersistTerminalFailure(ctx context.Context, db *gorm.DB, in FailedEventInput) error {
	if in.Err == nil {
		return errors.New("failed_events: nil error")
	}
	row := model.FailedEvent{
		OrgID:        in.OrgID,
		TriggerID:    in.TriggerID,
		EventType:    in.EventType,
		Payload:      model.RawJSON(in.Payload),
		Error:        in.Err.Error(),
		AttemptCount: in.AttemptCount,
		FailedAt:     time.Now().UTC(),
		Status:       model.FailedEventStatusPending,
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert failed_event: %w", err)
	}
	return nil
}

type TaskBuilder func(payload []byte) (*asynq.Task, error)

var (
	taskBuildersMu sync.RWMutex
	taskBuilders   = map[string]TaskBuilder{}
)

func RegisterTaskBuilder(eventType string, fn TaskBuilder) {
	taskBuildersMu.Lock()
	defer taskBuildersMu.Unlock()
	taskBuilders[eventType] = fn
}

func lookupTaskBuilder(eventType string) (TaskBuilder, bool) {
	taskBuildersMu.RLock()
	defer taskBuildersMu.RUnlock()
	fn, ok := taskBuilders[eventType]
	return fn, ok
}

var ErrFailedEventNotPending = errors.New("failed_events: row is not pending")

func RetryFailedEvent(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, id uuid.UUID) (*asynq.TaskInfo, error) {
	var row model.FailedEvent
	if err := db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, fmt.Errorf("load failed_event %s: %w", id, err)
	}
	if row.Status != model.FailedEventStatusPending {
		return nil, ErrFailedEventNotPending
	}
	builder, ok := lookupTaskBuilder(row.EventType)
	if !ok {
		return nil, fmt.Errorf("no task builder registered for event_type %q", row.EventType)
	}
	task, err := builder([]byte(row.Payload))
	if err != nil {
		return nil, fmt.Errorf("build task for retry: %w", err)
	}
	info, err := enqueuer.EnqueueContext(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("enqueue retry: %w", err)
	}
	now := time.Now().UTC()
	updates := map[string]any{
		"status":          model.FailedEventStatusRetried,
		"retried_at":      now,
		"retried_task_id": info.ID,
	}
	if err := db.WithContext(ctx).Model(&model.FailedEvent{}).
		Where("id = ?", row.ID).
		Updates(updates).Error; err != nil {
		return info, fmt.Errorf("mark failed_event retried: %w", err)
	}
	return info, nil
}

func DiscardFailedEvent(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	res := db.WithContext(ctx).Model(&model.FailedEvent{}).
		Where("id = ? AND status = ?", id, model.FailedEventStatusPending).
		Update("status", model.FailedEventStatusDiscarded)
	if res.Error != nil {
		return fmt.Errorf("discard failed_event: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrFailedEventNotPending
	}
	return nil
}
