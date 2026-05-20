package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/bridgeevents"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *BridgeWebhookHandler) cloudAgentTaskForConversation(ctx context.Context, conversationID uuid.UUID) (*model.CloudAgentTask, bool) {
	var task model.CloudAgentTask
	if err := h.db.WithContext(ctx).Where("conversation_id = ?", conversationID).First(&task).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			captureCloudAgentFailure(ctx, "bridge_webhook", err, cloudAgentSentryContext{
				Operation:      "load_cloud_agent_task",
				ConversationID: conversationID,
			})
		}
		return nil, false
	}
	return &task, true
}

func (h *BridgeWebhookHandler) forwardCloudAgentEvent(ctx context.Context, task model.CloudAgentTask, conv *model.AgentConversation, event *webhookEvent) {
	if isCloudAgentErrorEvent(event.EventType) {
		captureCloudAgentFailure(ctx, "bridge_webhook", fmt.Errorf("cloud agent emitted %s event", event.EventType), cloudAgentSentryContext{
			Operation:      "agent_error_event",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
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
		AgentID:               event.AgentID,
		RuntimeConversationID: event.ConversationID,
		Timestamp:             event.Timestamp,
		SequenceNumber:        event.SequenceNumber,
		Data:                  model.RawJSON(event.Data),
	}
	if err := dispatchCloudAgentCallback(ctx, h.db, h.encKey, h.employeeCallbackRuntime, task, dbEvent); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "webhook: failed to forward cloud agent event to employee bridge",
			"task_id", task.ID,
			"event_id", event.EventID,
			"event_type", event.EventType,
			"error", err,
		)
		captureCloudAgentWarning(ctx, "bridge_webhook", err, cloudAgentSentryContext{
			Operation:      "dispatch_cloud_agent_callback",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: task.ConversationID,
		})
	}
}

func isCloudAgentErrorEvent(eventType string) bool {
	eventType = strings.ToLower(bridgeevents.NormalizeEventType(eventType))
	return strings.Contains(eventType, "error") || strings.Contains(eventType, "failed") || strings.Contains(eventType, "failure")
}
