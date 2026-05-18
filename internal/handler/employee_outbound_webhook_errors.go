package handler

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
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
		if sb.AgentID != nil {
			fields["agent_id"] = sb.AgentID.String()
		}
	}
	if event != nil {
		fields["event_type"] = event.EventType
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee outbound webhook ingest %s: %w", stage, err), fields)
}

func captureEmployeeMemoryEventFailure(ctx context.Context, stage string, entry model.EmployeeMemoryEvent, err error) {
	if err == nil {
		return
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee memory event %s: %w", stage, err), employeeMemoryEventSentryFields(stage, entry))
}

func employeeMemoryEventSentryFields(stage string, entry model.EmployeeMemoryEvent) map[string]any {
	return map[string]any{
		"stage":      stage,
		"org_id":     entry.OrgID.String(),
		"agent_id":   entry.AgentID.String(),
		"sandbox_id": entry.SandboxID.String(),
		"session_id": entry.SessionID,
		"event_type": entry.EventType,
		"source":     entry.Source,
	}
}
