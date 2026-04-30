package sentry

import (
	"context"
	"fmt"
	"log/slog"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

func AsynqErrorHandler() asynq.ErrorHandler {
	return asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, taskErr error) {
		taskID, _ := asynq.GetTaskID(ctx)
		retryCount, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)
		queueName, _ := asynq.GetQueueName(ctx)
		isFinalAttempt := retryCount >= maxRetry

		slog.Error("asynq task failed",
			"task_type", task.Type(),
			"task_id", taskID,
			"queue", queueName,
			"retry_count", retryCount,
			"max_retry", maxRetry,
			"final_attempt", isFinalAttempt,
			"error", taskErr,
		)

		if !Enabled() {
			return
		}

		hub := hubFromContext(ctx)
		hub.WithScope(func(scope *sentrygo.Scope) {
			scope.SetTag("asynq.task_type", task.Type())
			scope.SetTag("asynq.queue", queueName)
			scope.SetTag("asynq.final_attempt", boolAsTag(isFinalAttempt))
			scope.SetContext("asynq", sentrygo.Context{
				"task_id":       taskID,
				"retry_count":   retryCount,
				"max_retry":     maxRetry,
				"final_attempt": isFinalAttempt,
			})
			if isFinalAttempt {
				scope.SetLevel(sentrygo.LevelFatal)
				hub.CaptureException(fmt.Errorf("asynq task %s exhausted retries: %w", task.Type(), taskErr))
				return
			}
			hub.AddBreadcrumb(&sentrygo.Breadcrumb{
				Type:     "queue",
				Category: "asynq.retry",
				Message:  fmt.Sprintf("asynq task %s failed (attempt %d/%d): %v", task.Type(), retryCount, maxRetry, taskErr),
				Level:    sentrygo.LevelWarning,
				Data: map[string]any{
					"task_type":   task.Type(),
					"task_id":     taskID,
					"queue":       queueName,
					"retry_count": retryCount,
					"max_retry":   maxRetry,
				},
			}, nil)
		})
	})
}

func boolAsTag(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
