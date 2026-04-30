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
		queue, _ := asynq.GetQueueName(ctx)

		slog.Error("asynq task failed",
			"task_type", task.Type(),
			"task_id", taskID,
			"queue", queue,
			"retry_count", retryCount,
			"max_retry", maxRetry,
			"error", taskErr,
		)

		if !Enabled() {
			return
		}

		hub := hubFromContext(ctx)
		hub.WithScope(func(scope *sentrygo.Scope) {
			scope.SetTag("asynq.task_type", task.Type())
			scope.SetTag("asynq.queue", queue)
			scope.SetContext("asynq", sentrygo.Context{
				"task_id":     taskID,
				"retry_count": retryCount,
				"max_retry":   maxRetry,
			})
			if retryCount >= maxRetry {
				scope.SetLevel(sentrygo.LevelFatal)
			} else {
				scope.SetLevel(sentrygo.LevelError)
			}
			hub.CaptureException(fmt.Errorf("asynq task %s: %w", task.Type(), taskErr))
		})
	})
}

func AsynqMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			if !Enabled() {
				return next.ProcessTask(ctx, task)
			}

			taskID, _ := asynq.GetTaskID(ctx)
			queue, _ := asynq.GetQueueName(ctx)
			retryCount, _ := asynq.GetRetryCount(ctx)

			hub := sentrygo.CurrentHub().Clone()
			hub.Scope().SetTag("asynq.task_type", task.Type())
			hub.Scope().SetTag("asynq.queue", queue)
			hub.Scope().SetContext("asynq", sentrygo.Context{
				"task_id":     taskID,
				"retry_count": retryCount,
			})
			ctx = sentrygo.SetHubOnContext(ctx, hub)
			applyAttribution(ctx, hub.Scope())

			tx := sentrygo.StartTransaction(
				ctx,
				fmt.Sprintf("asynq.task %s", task.Type()),
				sentrygo.WithOpName("queue.task"),
				sentrygo.WithTransactionSource(sentrygo.SourceTask),
			)
			tx.SetData("messaging.system", "asynq")
			tx.SetData("messaging.destination.name", queue)
			tx.SetData("messaging.message.id", taskID)
			tx.SetData("messaging.message.retry_count", retryCount)

			err := next.ProcessTask(tx.Context(), task) //nolint:contextcheck

			if err != nil {
				tx.Status = sentrygo.SpanStatusInternalError
				tx.SetData("error", err.Error())
			} else {
				tx.Status = sentrygo.SpanStatusOK
			}
			tx.Finish()
			return err
		})
	}
}
