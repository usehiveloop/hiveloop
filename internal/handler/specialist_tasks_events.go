package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/bridgeevents"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func terminateTaskSentryContext(operation string, employee *model.Agent, agentID uuid.UUID, task model.CloudAgentTask, conv model.AgentConversation, sandboxID uuid.UUID) cloudAgentSentryContext {
	return cloudAgentSentryContext{
		Operation:      operation,
		OrgID:          uuidValue(employee.OrgID),
		EmployeeID:     employee.ID,
		CloudAgentID:   agentID,
		TaskID:         task.ID,
		SandboxID:      sandboxID,
		ConversationID: conv.ID,
	}
}

func (h *SpecialistTaskHandler) ensureConversationEndedEvent(ctx context.Context, task model.CloudAgentTask, conv model.AgentConversation, reason string, now time.Time) {
	var count int64
	if err := h.db.Model(&model.ConversationEvent{}).
		Where("conversation_id = ? AND event_type = ?", conv.ID, bridgeevents.EventConversationEnded).
		Count(&count).Error; err != nil || count > 0 {
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "failed to check existing conversation ended event", "conversation_id", conv.ID, "error", err)
			captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
				Operation:      "check_existing_ended_event",
				OrgID:          task.OrgID,
				EmployeeID:     task.EmployeeAgentID,
				CloudAgentID:   task.CloudAgentID,
				TaskID:         task.ID,
				SandboxID:      task.SandboxID,
				ConversationID: conv.ID,
			})
		}
		return
	}

	var maxSequence int64
	if err := h.db.Model(&model.ConversationEvent{}).
		Select("COALESCE(MAX(sequence_number), 0)").
		Where("conversation_id = ?", conv.ID).
		Scan(&maxSequence).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to load conversation event sequence", "conversation_id", conv.ID, "error", err)
		captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "load_event_sequence",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
		return
	}

	data, _ := json.Marshal(map[string]any{
		"reason": reason,
		"source": "cloud_agent_terminate",
	})
	event := model.ConversationEvent{
		OrgID:                 conv.OrgID,
		ConversationID:        conv.ID,
		EventID:               uuid.New().String(),
		EventType:             bridgeevents.EventConversationEnded,
		AgentID:               conv.AgentID.String(),
		RuntimeConversationID: conv.RuntimeConversationID,
		Timestamp:             now,
		SequenceNumber:        maxSequence + 1,
		Data:                  model.RawJSON(data),
	}
	if err := h.db.Create(&event).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to append conversation ended event", "conversation_id", conv.ID, "error", err)
		captureCloudAgentFailure(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "append_ended_event",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
		return
	}

	if err := dispatchCloudAgentCallback(ctx, h.db, h.encKey, h.hooks.EmployeeCallbackRuntime, task, event); err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to forward specialist termination to employee bridge",
			"task_id", task.ID,
			"conversation_id", conv.ID,
			"error", err,
		)
		captureCloudAgentWarning(ctx, "terminate_task", err, cloudAgentSentryContext{
			Operation:      "dispatch_termination_callback",
			OrgID:          task.OrgID,
			EmployeeID:     task.EmployeeAgentID,
			CloudAgentID:   task.CloudAgentID,
			TaskID:         task.ID,
			SandboxID:      task.SandboxID,
			ConversationID: conv.ID,
		})
	}
}
