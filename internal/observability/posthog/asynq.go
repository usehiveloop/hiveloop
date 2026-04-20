package posthog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	ph "github.com/posthog/posthog-go"
)

// AsynqErrorHandler returns an asynq.ErrorHandler that logs and captures every
// task failure to PostHog. The worker's retry logic still runs — this is
// purely for observability.
//
// Safe to use with a nil client; when client is nil the handler only logs.
func AsynqErrorHandler(client ph.Client) asynq.ErrorHandler {
	return asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, taskErr error) {
		taskID, _ := asynq.GetTaskID(ctx)
		retryCount, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		queue, _ := asynq.GetQueueName(ctx)

		slog.Error("asynq task failed",
			"task_type", task.Type(),
			"task_id", taskID,
			"queue", queue,
			"retry_count", retryCount,
			"max_retry", maxRetry,
			"error", taskErr,
		)

		if client == nil {
			return
		}

		title := fmt.Sprintf("asynq task failed: %s", task.Type())
		description := fmt.Sprintf("task_id=%s queue=%s retry=%d/%d error=%v",
			taskID, queue, retryCount, maxRetry, taskErr)
		CaptureException(client, ctx, title, description)
	})
}
