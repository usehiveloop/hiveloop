package sentry

import (
	"context"
	"fmt"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

func StartEnqueueSpan(ctx context.Context, taskType, queueName string) *sentrygo.Span {
	if !Enabled() {
		return nil
	}
	span := sentrygo.StartSpan(ctx, "queue.publish",
		sentrygo.WithDescription(fmt.Sprintf("asynq.enqueue %s", taskType)),
	)
	span.SetData("messaging.system", "asynq")
	span.SetData("messaging.operation", "publish")
	span.SetData("messaging.destination.name", queueName)
	span.SetTag("asynq.task_type", taskType)
	if queueName != "" {
		span.SetTag("asynq.queue", queueName)
	}
	return span
}

func FinishEnqueueSpan(ctx context.Context, span *sentrygo.Span, info *asynq.TaskInfo, enqueueErr error) {
	if span == nil {
		captureEnqueueError(ctx, enqueueErr)
		return
	}
	if info != nil {
		span.SetData("messaging.message.id", info.ID)
		if info.Queue != "" {
			span.SetData("messaging.destination.name", info.Queue)
		}
	}
	if enqueueErr != nil {
		span.Status = sentrygo.SpanStatusInternalError
		span.SetData("error", enqueueErr.Error())
		captureEnqueueError(ctx, enqueueErr)
	} else {
		span.Status = sentrygo.SpanStatusOK
		recordEnqueueBreadcrumb(info)
	}
	span.Finish()
}

func captureEnqueueError(ctx context.Context, enqueueErr error) {
	if enqueueErr == nil || !Enabled() {
		return
	}
	hub := hubFromContext(ctx)
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("asynq.enqueue", "true")
		scope.SetLevel(sentrygo.LevelError)
		hub.CaptureException(fmt.Errorf("asynq enqueue: %w", enqueueErr))
	})
}

func recordEnqueueBreadcrumb(info *asynq.TaskInfo) {
	if !Enabled() || info == nil {
		return
	}
	sentrygo.CurrentHub().AddBreadcrumb(&sentrygo.Breadcrumb{
		Type:     "queue",
		Category: "asynq.enqueue",
		Message:  fmt.Sprintf("enqueued %s -> %s", info.Type, info.Queue),
		Level:    sentrygo.LevelInfo,
		Data: map[string]any{
			"task_type": info.Type,
			"task_id":   info.ID,
			"queue":     info.Queue,
		},
	}, nil)
}
