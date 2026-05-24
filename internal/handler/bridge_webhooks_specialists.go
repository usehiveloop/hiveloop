package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *BridgeWebhookHandler) specialistTaskForConversation(ctx context.Context, conversationID uuid.UUID) (*model.SpecialistTask, bool) {
	var task model.SpecialistTask
	if err := h.db.WithContext(ctx).Where("conversation_id = ?", conversationID).First(&task).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			captureSpecialistFailure(ctx, "bridge_webhook", err, specialistSentryContext{
				Operation:      "load_specialist_task",
				ConversationID: conversationID,
			})
		}
		return nil, false
	}
	return &task, true
}

func (h *BridgeWebhookHandler) forwardSpecialistEvent(ctx context.Context, task model.SpecialistTask, conv *model.EmployeeConversation, event *webhookEvent) {
	if isSpecialistErrorEvent(event.EventType) {
		captureSpecialistFailure(ctx, "bridge_webhook", fmt.Errorf("specialist emitted %s event", event.EventType), specialistSentryContext{
			Operation:      "agent_error_event",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeID,
			SpecialistID:   task.SpecialistID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
	}

	dbEvent := model.ConversationEvent{
		OrgID:                 conv.OrgID,
		ConversationID:        conv.ID,
		EventID:               event.EventID,
		EventType:             event.EventType,
		EmployeeID:            event.EmployeeID,
		RuntimeConversationID: event.ConversationID,
		Timestamp:             event.Timestamp,
		SequenceNumber:        event.SequenceNumber,
		Data:                  model.RawJSON(event.Data),
	}
	if err := dispatchSpecialistCallback(ctx, h.db, h.encKey, h.employeeCallbackRuntime, task, dbEvent); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "webhook: failed to forward specialist event to employee bridge",
			"task_id", task.ID,
			"event_id", event.EventID,
			"event_type", event.EventType,
			"error", err,
		)
		captureSpecialistWarning(ctx, "bridge_webhook", err, specialistSentryContext{
			Operation:      "dispatch_specialist_callback",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeID,
			SpecialistID:   task.SpecialistID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
	}
}

func isSpecialistErrorEvent(eventType string) bool {
	eventType = strings.ToLower(eventType)
	return strings.Contains(eventType, "error") || strings.Contains(eventType, "failed") || strings.Contains(eventType, "failure")
}
