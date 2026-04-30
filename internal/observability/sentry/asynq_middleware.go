package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

func AsynqMiddleware() asynq.MiddlewareFunc {
	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			if !Enabled() {
				return next.ProcessTask(ctx, taskWithUnwrappedPayload(task))
			}

			taskID, _ := asynq.GetTaskID(ctx)
			queueName, _ := asynq.GetQueueName(ctx)
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry, _ := asynq.GetMaxRetry(ctx)

			hub := sentrygo.CurrentHub().Clone()
			scope := hub.Scope()
			scope.SetTag("asynq.task_type", task.Type())
			scope.SetTag("asynq.queue", queueName)
			scope.SetContext("asynq", sentrygo.Context{
				"task_id":     taskID,
				"retry_count": retryCount,
				"max_retry":   maxRetry,
			})
			ctx = sentrygo.SetHubOnContext(ctx, hub)
			applyAttribution(ctx, scope)

			body, traceHeader, baggageHeader, hasTrace := decodeTracePayload(task.Payload())

			transactionOptions := []sentrygo.SpanOption{
				sentrygo.WithOpName("queue.process"),
				sentrygo.WithTransactionSource(sentrygo.SourceTask),
			}
			if hasTrace {
				transactionOptions = append(transactionOptions, sentrygo.ContinueFromHeaders(traceHeader, baggageHeader))
			}

			transaction := sentrygo.StartTransaction(
				ctx,
				fmt.Sprintf("asynq.task %s", task.Type()),
				transactionOptions...,
			)
			transaction.SetData("messaging.system", "asynq")
			transaction.SetData("messaging.operation", "process")
			transaction.SetData("messaging.destination.name", queueName)
			transaction.SetData("messaging.message.id", taskID)
			transaction.SetData("messaging.message.retry_count", retryCount)
			transaction.SetData("messaging.message.body.size", len(body))
			transaction.SetData("messaging.asynq.trace_propagated", hasTrace)

			hub.AddBreadcrumb(&sentrygo.Breadcrumb{
				Type:     "queue",
				Category: "asynq.start",
				Message:  fmt.Sprintf("asynq task %s started (attempt %d/%d)", task.Type(), retryCount, maxRetry),
				Level:    sentrygo.LevelInfo,
				Data: map[string]any{
					"task_type":   task.Type(),
					"task_id":     taskID,
					"queue":       queueName,
					"retry_count": retryCount,
				},
			}, nil)

			processCtx := transaction.Context()
			handlerTask := task
			if hasTrace {
				handlerTask = asynq.NewTask(task.Type(), body)
			}

			var handlerErr error
			runHandlerWithPanicCapture(processCtx, hub, transaction, task, taskID, queueName, func() {
				handlerErr = next.ProcessTask(processCtx, handlerTask) //nolint:contextcheck
			})

			if handlerErr != nil {
				transaction.Status = sentrygo.SpanStatusInternalError
				transaction.SetData("error", handlerErr.Error())
			} else {
				transaction.Status = sentrygo.SpanStatusOK
			}
			transaction.Finish()
			return handlerErr
		})
	}
}

func runHandlerWithPanicCapture(
	ctx context.Context,
	hub *sentrygo.Hub,
	transaction *sentrygo.Span,
	task *asynq.Task,
	taskID, queueName string,
	run func(),
) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		stack := debug.Stack()
		slog.ErrorContext(ctx, "panic recovered in asynq task",
			"panic", recovered,
			"task_type", task.Type(),
			"task_id", taskID,
			"queue", queueName,
			"stack", string(stack),
		)
		hub.WithScope(func(scope *sentrygo.Scope) {
			scope.SetLevel(sentrygo.LevelFatal)
			scope.SetTag("asynq.panic", "true")
			scope.SetContext("panic", sentrygo.Context{
				"value": fmt.Sprintf("%v", recovered),
				"stack": string(stack),
			})
			hub.CaptureException(fmt.Errorf("panic in asynq task %s: %v", task.Type(), recovered))
		})
		transaction.Status = sentrygo.SpanStatusInternalError
		transaction.SetData("error", fmt.Sprintf("panic: %v", recovered))
		transaction.Finish()
		panic(recovered)
	}()
	run()
}

func taskWithUnwrappedPayload(task *asynq.Task) *asynq.Task {
	body, _, _, hasTrace := decodeTracePayload(task.Payload())
	if !hasTrace {
		return task
	}
	return asynq.NewTask(task.Type(), body)
}
