package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func hindsightMemoryRefresh(enqueuer enqueue.TaskEnqueuer) hindsight.MemoryRefreshFunc {
	return func(ctx context.Context, agent *model.Agent) {
		if enqueuer == nil || agent == nil || !agent.IsEmployee {
			return
		}
		task, err := tasks.NewEmployeeMemoryRefreshTask(tasks.EmployeeMemoryRefreshPayload{
			AgentID: agent.ID,
			Reason:  "memory_forget",
		})
		if err != nil {
			logging.Capture(ctx, err)
			return
		}
		_, err = enqueuer.EnqueueContext(ctx, task,
			asynq.Unique(2*time.Minute),
			asynq.TaskID("employee-memory-refresh:"+agent.ID.String()),
		)
		if err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			logging.Capture(ctx, fmt.Errorf("memory forget: enqueue employee memory refresh: %w", err))
		}
	}
}
