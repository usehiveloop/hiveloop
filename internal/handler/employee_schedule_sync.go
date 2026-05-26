package handler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func syncEmployeeScheduleEvent(tx *gorm.DB, event model.EmployeeSessionEvent) error {
	if !strings.HasPrefix(event.EventType, "schedule.") {
		return nil
	}
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode schedule payload: %w", err)
		}
	}
	jobID := stringValue(payload, "job_id")
	if jobID == "" {
		return fmt.Errorf("schedule payload missing job_id")
	}
	if source := strings.ToLower(stringValue(payload, "source")); source != "" && source != "cron" {
		return nil
	}

	schedule, err := upsertEmployeeScheduleFromEvent(tx, event, payload, jobID)
	if err != nil {
		return err
	}
	if strings.HasPrefix(event.EventType, "schedule.run_") {
		return upsertEmployeeScheduleRunFromEvent(tx, event, payload, schedule, jobID)
	}
	return nil
}

func upsertEmployeeScheduleFromEvent(tx *gorm.DB, event model.EmployeeSessionEvent, payload map[string]any, jobID string) (*model.EmployeeSchedule, error) {
	status := scheduleStatusFromEvent(event.EventType, payload)
	cancelledAt := (*time.Time)(nil)
	if event.EventType == "schedule.cancelled" {
		cancelledAt = &event.EventAt
	}
	schedule := model.EmployeeSchedule{
		OrgID:            event.OrgID,
		EmployeeID:       event.EmployeeID,
		SandboxID:        event.SandboxID,
		RuntimeJobID:     jobID,
		Status:           status,
		Channel:          stringValue(payload, "channel"),
		Description:      stringValue(payload, "description"),
		TaskPrompt:       stringValue(payload, "task_prompt"),
		IntervalSeconds:  int64PtrFromPayload(payload, "interval_seconds"),
		RepeatCount:      int64PtrFromPayload(payload, "repeat_count"),
		RepeatCompleted:  int64Value(payload, "repeat_completed"),
		NextRunAt:        timePtrFromPayload(payload, "next_run_at"),
		LastRunAt:        timePtrFromPayload(payload, "last_run_at"),
		LastStatus:       latestScheduleStatus(event.EventType, payload),
		LastError:        firstNonEmpty(stringValue(payload, "last_error"), stringValue(payload, "error")),
		CreatedBySession: stringValue(payload, "created_by_session"),
		RuntimeCreatedAt: timePtrFromPayload(payload, "created_at"),
		CancelledAt:      cancelledAt,
	}
	err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "employee_id"}, {Name: "runtime_job_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"org_id":             schedule.OrgID,
			"sandbox_id":         schedule.SandboxID,
			"status":             schedule.Status,
			"channel":            schedule.Channel,
			"description":        schedule.Description,
			"task_prompt":        schedule.TaskPrompt,
			"interval_seconds":   schedule.IntervalSeconds,
			"repeat_count":       schedule.RepeatCount,
			"repeat_completed":   schedule.RepeatCompleted,
			"next_run_at":        schedule.NextRunAt,
			"last_run_at":        schedule.LastRunAt,
			"last_status":        schedule.LastStatus,
			"last_error":         schedule.LastError,
			"created_by_session": schedule.CreatedBySession,
			"runtime_created_at": schedule.RuntimeCreatedAt,
			"cancelled_at":       schedule.CancelledAt,
			"updated_at":         time.Now(),
		}),
	}).Create(&schedule).Error
	if err != nil {
		return nil, fmt.Errorf("upsert employee schedule: %w", err)
	}
	if err := tx.Where("employee_id = ? AND runtime_job_id = ?", event.EmployeeID, jobID).First(&schedule).Error; err != nil {
		return nil, fmt.Errorf("load employee schedule: %w", err)
	}
	return &schedule, nil
}

func upsertEmployeeScheduleRunFromEvent(tx *gorm.DB, event model.EmployeeSessionEvent, payload map[string]any, schedule *model.EmployeeSchedule, jobID string) error {
	scheduledAt := timePtrFromPayload(payload, "scheduled_at")
	runKey := stringValue(payload, "run_key")
	if runKey == "" {
		if scheduledAt != nil {
			runKey = jobID + ":" + scheduledAt.Format(time.RFC3339)
		} else {
			runKey = jobID + ":" + event.EventAt.Format(time.RFC3339Nano)
		}
	}
	run := model.EmployeeScheduleRun{
		OrgID:        event.OrgID,
		EmployeeID:   event.EmployeeID,
		ScheduleID:   schedule.ID,
		SandboxID:    event.SandboxID,
		RuntimeJobID: jobID,
		RunKey:       runKey,
		Status:       runStatusFromEvent(event.EventType),
		ScheduledAt:  scheduledAt,
		StartedAt:    timePtrFromPayload(payload, "started_at"),
		CompletedAt:  timePtrFromPayload(payload, "completed_at"),
		DurationMS:   int64PtrFromPayload(payload, "duration_ms"),
		Error:        firstNonEmpty(stringValue(payload, "error"), stringValue(payload, "last_error")),
		EventPayload: event.Payload,
	}
	if run.StartedAt == nil && event.EventType == "schedule.run_started" {
		run.StartedAt = &event.EventAt
	}
	if run.CompletedAt == nil && (event.EventType == "schedule.run_completed" || event.EventType == "schedule.run_failed") {
		run.CompletedAt = &event.EventAt
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "schedule_id"}, {Name: "run_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"org_id":         run.OrgID,
			"employee_id":    run.EmployeeID,
			"sandbox_id":     run.SandboxID,
			"runtime_job_id": run.RuntimeJobID,
			"status":         run.Status,
			"scheduled_at":   run.ScheduledAt,
			"started_at":     run.StartedAt,
			"completed_at":   run.CompletedAt,
			"duration_ms":    run.DurationMS,
			"error":          run.Error,
			"event_payload":  run.EventPayload,
			"updated_at":     time.Now(),
		}),
	}).Create(&run).Error
}

func scheduleStatusFromEvent(eventType string, payload map[string]any) string {
	switch eventType {
	case "schedule.paused":
		return "paused"
	case "schedule.resumed", "schedule.created", "schedule.updated":
		return "active"
	case "schedule.cancelled":
		return "cancelled"
	}
	switch strings.ToLower(stringValue(payload, "state")) {
	case "paused":
		return "paused"
	case "completed":
		return "completed"
	default:
		return "active"
	}
}

func latestScheduleStatus(eventType string, payload map[string]any) string {
	switch eventType {
	case "schedule.run_started":
		return "running"
	case "schedule.run_completed":
		return "completed"
	case "schedule.run_failed":
		return "failed"
	default:
		return stringValue(payload, "last_status")
	}
}

func runStatusFromEvent(eventType string) string {
	switch eventType {
	case "schedule.run_completed":
		return "completed"
	case "schedule.run_failed":
		return "failed"
	default:
		return "running"
	}
}

func timePtrFromPayload(payload map[string]any, key string) *time.Time {
	value := stringValue(payload, key)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func int64PtrFromPayload(payload map[string]any, key string) *int64 {
	if value, ok := payload[key]; ok && value != nil {
		n := int64FromAny(value)
		return &n
	}
	return nil
}

func int64Value(payload map[string]any, key string) int64 {
	if value, ok := payload[key]; ok && value != nil {
		return int64FromAny(value)
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		n, _ := typed.Int64()
		return n
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
