package handler

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func captureEmployeeWebhookIngest(ctx context.Context, stage string, sb *model.Sandbox, event *employeeOutboundEvent, sessionID, source string, err error) {
	if err == nil {
		return
	}
	fields := map[string]any{
		"stage":      stage,
		"session_id": sessionID,
		"source":     source,
	}
	if sb != nil {
		fields["sandbox_id"] = sb.ID.String()
		if sb.OrgID != nil {
			fields["org_id"] = sb.OrgID.String()
		}
		if sb.EmployeeID != nil {
			fields["employee_id"] = sb.EmployeeID.String()
		}
	}
	if event != nil {
		fields["event_type"] = event.EventType
		if !event.At.IsZero() {
			fields["event_at"] = event.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee outbound webhook ingest %s: %w", stage, err), fields)
}

func captureEmployeeSessionEventFailure(ctx context.Context, stage string, entry model.EmployeeSessionEvent, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee session event %s: %w", stage, err), employeeSessionEventSentryFields(stage, entry))
}

func employeeSessionEventSentryFields(stage string, entry model.EmployeeSessionEvent) map[string]any {
	return map[string]any{
		"stage":       stage,
		"org_id":      entry.OrgID.String(),
		"employee_id": entry.EmployeeID.String(),
		"sandbox_id":  entry.SandboxID.String(),
		"session_id":  entry.SessionID,
		"event_type":  entry.EventType,
		"source":      entry.Source,
	}
}
