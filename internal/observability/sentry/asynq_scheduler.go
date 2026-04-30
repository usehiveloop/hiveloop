package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
)

func AsynqSchedulerOpts(location *time.Location) *asynq.SchedulerOpts {
	return &asynq.SchedulerOpts{
		Location:        location,
		PostEnqueueFunc: handleSchedulerPostEnqueue,
	}
}

func handleSchedulerPostEnqueue(info *asynq.TaskInfo, enqueueErr error) {
	if enqueueErr != nil {
		slog.Error("asynq scheduler enqueue failed", "error", enqueueErr)
		captureSchedulerEnqueueError(info, enqueueErr)
		return
	}
	recordSchedulerBreadcrumb(info)
}

func captureSchedulerEnqueueError(info *asynq.TaskInfo, enqueueErr error) {
	if !Enabled() {
		return
	}
	hub := sentrygo.CurrentHub().Clone()
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("asynq.scheduler", "true")
		if info != nil {
			scope.SetTag("asynq.task_type", info.Type)
			scope.SetTag("asynq.queue", info.Queue)
			scope.SetContext("asynq", sentrygo.Context{
				"task_id": info.ID,
				"queue":   info.Queue,
			})
		}
		scope.SetLevel(sentrygo.LevelError)
		hub.CaptureException(fmt.Errorf("asynq scheduler enqueue: %w", enqueueErr))
	})
}

func recordSchedulerBreadcrumb(info *asynq.TaskInfo) {
	if !Enabled() || info == nil {
		return
	}
	sentrygo.CurrentHub().AddBreadcrumb(&sentrygo.Breadcrumb{
		Type:     "queue",
		Category: "asynq.scheduler",
		Message:  fmt.Sprintf("scheduled %s -> %s", info.Type, info.Queue),
		Level:    sentrygo.LevelInfo,
		Data: map[string]any{
			"task_type": info.Type,
			"task_id":   info.ID,
			"queue":     info.Queue,
		},
	}, nil)
}

func CaptureAsynqSchedulerError(ctx context.Context, schedulerErr error) {
	if schedulerErr == nil {
		return
	}
	slog.ErrorContext(ctx, "asynq scheduler exited", "error", schedulerErr)
	if !Enabled() {
		return
	}
	hub := hubFromContext(ctx)
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("asynq.scheduler", "true")
		scope.SetLevel(sentrygo.LevelFatal)
		hub.CaptureException(fmt.Errorf("asynq scheduler run: %w", schedulerErr))
	})
}

func CaptureAsynqServerError(ctx context.Context, serverErr error) {
	if serverErr == nil {
		return
	}
	slog.ErrorContext(ctx, "asynq server exited", "error", serverErr)
	if !Enabled() {
		return
	}
	hub := hubFromContext(ctx)
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetTag("asynq.server", "true")
		scope.SetLevel(sentrygo.LevelFatal)
		hub.CaptureException(fmt.Errorf("asynq server run: %w", serverErr))
	})
}
