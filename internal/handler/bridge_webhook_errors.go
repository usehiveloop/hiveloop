package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func captureBridgeWebhookIngest(ctx context.Context, stage string, sb *model.Sandbox, event *webhookEvent, conversationID uuid.UUID, err error) {
	if err == nil {
		return
	}
	fields := bridgeWebhookSentryFields(stage, sb, event, conversationID)
	logging.CaptureWithFields(ctx, fmt.Errorf("bridge webhook ingest %s: %w", stage, err), fields)
}

func bridgeWebhookSentryFields(stage string, sb *model.Sandbox, event *webhookEvent, conversationID uuid.UUID) map[string]any {
	fields := map[string]any{"stage": stage}
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
		fields["event_id"] = event.EventID
		fields["event_type"] = event.EventType
		fields["bridge_agent_id"] = event.AgentID
		fields["runtime_conversation_id"] = event.ConversationID
		fields["sequence_number"] = event.SequenceNumber
		if !event.Timestamp.IsZero() {
			fields["event_timestamp"] = event.Timestamp.UTC().Format(time.RFC3339Nano)
		}
	}
	if conversationID != uuid.Nil {
		fields["conversation_id"] = conversationID.String()
	}
	return fields
}
