package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

// cronParser is reused across all schedule computations. Supports the standard
// 5-field cron format (minute hour dom month dow).
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ---------------------------------------------------------------------------
// Periodic poller: finds due cron triggers and enqueues dispatch tasks
// ---------------------------------------------------------------------------

// CronTriggerPollHandler runs every 30 seconds and finds cron triggers that
// are due to fire. For each, it atomically advances NextRunAt and enqueues a
// CronTriggerDispatch task.
//
// The poll window is 30 seconds into the future to compensate for cold-start
// latency: by the time the sandbox is warm, the scheduled time has arrived.
type CronTriggerPollHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewCronTriggerPollHandler creates the poller.
func NewCronTriggerPollHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *CronTriggerPollHandler {
	return &CronTriggerPollHandler{db: db, enqueuer: enqueuer}
}

// Handle processes a TypeCronTriggerPoll periodic task.
func (handler *CronTriggerPollHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	logger := slog.With("task", TypeCronTriggerPoll)

	// Find all cron triggers that are due within the next 30 seconds.
	// This gives sandbox creation time to warm up before the actual
	// scheduled time arrives.
	lookAhead := time.Now().Add(30 * time.Second)

	var dueTriggers []model.RouterTrigger
	if err := handler.db.WithContext(ctx).
		Where("trigger_type = ? AND enabled = TRUE AND next_run_at IS NOT NULL AND next_run_at <= ?",
			"cron", lookAhead).
		Find(&dueTriggers).Error; err != nil {
		return fmt.Errorf("querying due cron triggers: %w", err)
	}

	if len(dueTriggers) == 0 {
		return nil
	}

	logger.Info("cron poll: found due triggers", "count", len(dueTriggers))

	enqueuedCount := 0
	for _, trigger := range dueTriggers {
		// Compute the next run time from the cron schedule.
		schedule, parseErr := cronParser.Parse(trigger.CronSchedule)
		if parseErr != nil {
			logger.Error("cron poll: invalid schedule",
				"trigger_id", trigger.ID,
				"schedule", trigger.CronSchedule,
				"error", parseErr,
			)
			continue
		}

		scheduledAt := *trigger.NextRunAt
		nextRun := schedule.Next(scheduledAt)
		now := time.Now()

		// Atomically claim this trigger by advancing NextRunAt.
		// The WHERE next_run_at = ? clause prevents duplicate fires if
		// another worker already claimed it.
		result := handler.db.WithContext(ctx).
			Model(&model.RouterTrigger{}).
			Where("id = ? AND next_run_at = ?", trigger.ID, trigger.NextRunAt).
			Updates(map[string]any{
				"next_run_at": nextRun,
				"last_run_at": now,
			})
		if result.RowsAffected == 0 {
			// Another worker already claimed this trigger.
			logger.Debug("cron poll: trigger already claimed",
				"trigger_id", trigger.ID,
			)
			continue
		}

		// Enqueue the dispatch task.
		task, taskErr := NewCronTriggerDispatchTask(CronTriggerDispatchPayload{
			RouterTriggerID: trigger.ID,
			OrgID:           trigger.OrgID,
			ScheduledAt:     scheduledAt,
		})
		if taskErr != nil {
			logger.Error("cron poll: failed to build dispatch task",
				"trigger_id", trigger.ID,
				"error", taskErr,
			)
			continue
		}

		if _, enqueueErr := handler.enqueuer.Enqueue(task); enqueueErr != nil {
			logger.Error("cron poll: failed to enqueue dispatch task",
				"trigger_id", trigger.ID,
				"error", enqueueErr,
			)
			continue
		}

		enqueuedCount++
		logger.Info("cron poll: dispatched",
			"trigger_id", trigger.ID,
			"scheduled_at", scheduledAt,
			"next_run_at", nextRun,
		)
	}

	logger.Info("cron poll: complete",
		"due_triggers", len(dueTriggers),
		"enqueued", enqueuedCount,
	)
	return nil
}

// ---------------------------------------------------------------------------
// Single cron trigger dispatch: runs the routing pipeline for one fire
// ---------------------------------------------------------------------------

// CronTriggerDispatchHandler processes a single cron trigger fire. It loads
// the trigger, builds a synthetic payload with schedule context, runs the
// routing pipeline (reusing the same RouterDispatcher as webhooks), and
// enqueues agent conversation creation tasks.
type CronTriggerDispatchHandler struct {
	dispatcher *dispatch.RouterDispatcher
	enqueuer   enqueue.TaskEnqueuer
}

// NewCronTriggerDispatchHandler creates the handler.
func NewCronTriggerDispatchHandler(dispatcher *dispatch.RouterDispatcher, enqueuer enqueue.TaskEnqueuer) *CronTriggerDispatchHandler {
	return &CronTriggerDispatchHandler{dispatcher: dispatcher, enqueuer: enqueuer}
}

// Handle processes a TypeCronTriggerDispatch task.
func (handler *CronTriggerDispatchHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload CronTriggerDispatchPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal cron trigger dispatch payload: %w", err)
	}

	logger := slog.With(
		"task", TypeCronTriggerDispatch,
		"trigger_id", payload.RouterTriggerID,
		"org_id", payload.OrgID,
		"scheduled_at", payload.ScheduledAt,
	)

	logger.Info("cron dispatch: starting")

	// Build a synthetic payload with schedule context so rules can
	// optionally filter by time/day.
	syntheticPayload := map[string]any{
		"scheduled_at": payload.ScheduledAt.Format(time.RFC3339),
		"trigger_type": "cron",
		"trigger_id":   payload.RouterTriggerID.String(),
	}

	// Run the routing pipeline for this specific trigger.
	dispatches, err := handler.dispatcher.RunForTrigger(ctx, payload.RouterTriggerID, syntheticPayload)
	if err != nil {
		logger.Error("cron dispatch: dispatcher failed", "error", err)
		return fmt.Errorf("cron trigger dispatch: %w", err)
	}

	if len(dispatches) == 0 {
		logger.Info("cron dispatch: no agents selected")
		return nil
	}

	// Enqueue a conversation creation job for each dispatch.
	deliveryID := fmt.Sprintf("cron:%s:%s", payload.RouterTriggerID, payload.ScheduledAt.Format(time.RFC3339))
	enqueuedCount := 0
	for _, agentDispatch := range dispatches {
		if agentDispatch.RunIntent != "normal" {
			continue
		}

		instructions := buildCronDispatchInstructions(agentDispatch, payload.ScheduledAt)
		convTask, taskErr := NewAgentConversationCreateTask(AgentConversationCreatePayload{
			AgentID:         agentDispatch.AgentID,
			OrgID:           agentDispatch.ReplyOrgID,
			DeliveryID:      deliveryID,
			RouterTriggerID: agentDispatch.RouterTriggerID,
			RouterPersona:   agentDispatch.RouterPersona,
			MemoryTeam:      agentDispatch.MemoryTeam,
			Instructions:    instructions,
		})
		if taskErr != nil {
			logger.Error("cron dispatch: failed to build conversation create task",
				"agent_id", agentDispatch.AgentID,
				"error", taskErr,
			)
			continue
		}

		if _, enqErr := handler.enqueuer.Enqueue(convTask); enqErr != nil {
			logger.Error("cron dispatch: failed to enqueue conversation create task",
				"agent_id", agentDispatch.AgentID,
				"error", enqErr,
			)
			continue
		}
		enqueuedCount++
	}

	logger.Info("cron dispatch: complete",
		"agents_dispatched", len(dispatches),
		"conversations_enqueued", enqueuedCount,
	)
	return nil
}

// buildCronDispatchInstructions builds the instructions for a cron-triggered
// agent conversation. Includes the router persona, trigger-level instructions,
// and schedule context.
func buildCronDispatchInstructions(agentDispatch dispatch.AgentDispatch, scheduledAt time.Time) string {
	var builder strings.Builder

	if agentDispatch.RouterPersona != "" {
		builder.WriteString(agentDispatch.RouterPersona)
		builder.WriteString("\n\n---\n\n")
	}

	if agentDispatch.TriggerInstructions != "" {
		builder.WriteString(dispatch.SubstituteRefs(agentDispatch.TriggerInstructions, agentDispatch.Refs))
		builder.WriteString("\n\n")
	}

	builder.WriteString(fmt.Sprintf("Scheduled run at: %s\n", scheduledAt.Format(time.RFC3339)))

	return builder.String()
}
