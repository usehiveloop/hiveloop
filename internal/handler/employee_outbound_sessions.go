package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *EmployeeOutboundWebhookHandler) ensureEmployeeSession(ctx context.Context, sb *model.Sandbox, sessionID, source string, payload map[string]any, specialistTask *model.SpecialistTask) (*model.EmployeeConversation, error) {
	if sb.OrgID == nil || sb.EmployeeID == nil {
		return nil, fmt.Errorf("employee sandbox missing org_id or employee_id")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("runtime session_id is required for employee session events")
	}
	if specialistTask != nil && specialistTask.ConversationID != nil {
		var session model.EmployeeConversation
		if err := h.db.WithContext(ctx).
			Where("id = ? AND org_id = ? AND employee_id = ?", *specialistTask.ConversationID, *sb.OrgID, *sb.EmployeeID).
			First(&session).Error; err != nil {
			return nil, fmt.Errorf("load specialist employee session: %w", err)
		}
		return &session, nil
	}
	if source == "" {
		source = employeeEventSource(payload)
	}
	session := model.EmployeeConversation{}
	scope := model.EmployeeConversation{
		OrgID:                 *sb.OrgID,
		EmployeeID:            *sb.EmployeeID,
		SandboxID:             sb.ID,
		RuntimeConversationID: sessionID,
	}
	attrs := model.EmployeeConversation{
		Source:            source,
		SourceResourceKey: employeeSessionSourceResourceKey(payload, sessionID),
		Status:            "active",
		IntegrationScopes: model.JSON{},
	}
	if err := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&session).Error; err != nil {
		return nil, fmt.Errorf("upsert employee session: %w", err)
	}
	return &session, nil
}

func (h *EmployeeOutboundWebhookHandler) markEmployeeSessionEnded(ctx context.Context, sessionID uuid.UUID, at time.Time) {
	if h == nil || h.db == nil || sessionID == uuid.Nil {
		return
	}
	endedAt := at.UTC()
	if endedAt.IsZero() {
		endedAt = h.now().UTC()
	}
	if err := h.db.WithContext(ctx).Model(&model.EmployeeConversation{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"status": "ended", "ended_at": endedAt}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("employee outbound webhook: mark session ended: %w", err))
	}
}

func employeeSessionSourceResourceKey(payload map[string]any, fallback string) string {
	return firstNonEmpty(
		stringValue(payload, "source_resource_key"),
		stringValue(payload, "thread_ts"),
		stringValue(payload, "channel"),
		stringValue(payload, "conversation_id"),
		stringValue(payload, "chat_id"),
		fallback,
	)
}

func employeeSessionEventFromOutbound(sb *model.Sandbox, event *employeeOutboundEvent, payload map[string]any, employeeSessionID uuid.UUID, sessionID string) (model.EmployeeSessionEvent, bool) {
	if sb.OrgID == nil || sb.EmployeeID == nil {
		return model.EmployeeSessionEvent{}, false
	}
	return model.EmployeeSessionEvent{
		OrgID:             *sb.OrgID,
		EmployeeID:        *sb.EmployeeID,
		SandboxID:         sb.ID,
		EmployeeSessionID: employeeSessionID,
		SessionID:         sessionID,
		EventID:           employeeSessionEventID(payload),
		EventType:         event.EventType,
		Source:            employeeEventSource(payload),
		Mode:              stringValueDefault(payload, "mode", "employee"),
		SequenceNumber:    int64Value(payload, "sequence"),
		Payload:           model.RawJSON(event.Payload),
		EventAt:           event.At.UTC(),
	}, true
}

func employeeSessionEventID(payload map[string]any) string {
	return firstNonEmpty(stringValue(payload, "event_id"), stringValue(payload, "id"))
}

func shouldStoreEmployeeSessionEvent(eventType string) bool {
	switch {
	case eventType == "session.created":
		return false
	case eventType == "tool.invoked":
		return false
	case eventType == "agent.final_message":
		return false
	case strings.HasPrefix(eventType, "agent.run."):
		return false
	default:
		return true
	}
}

func shouldTriggerEmployeeMemoryCheckpoint(eventType string) bool {
	switch eventType {
	case "agent.message.sent", "session.completed":
		return true
	default:
		return false
	}
}
