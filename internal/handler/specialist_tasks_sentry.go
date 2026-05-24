package handler

import (
	"context"
	"fmt"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

type specialistSentryContext struct {
	OrgID          uuid.UUID
	EmployeeID     uuid.UUID
	SpecialistID   uuid.UUID
	TaskID         uuid.UUID
	SandboxID      uuid.UUID
	ConversationID uuid.UUID
	Operation      string
	Status         string
	Reason         string
}

func captureSpecialistFailure(ctx context.Context, operation string, err error, eventCtx specialistSentryContext) {
	captureSpecialistFailureWithLevel(ctx, sentrygo.LevelError, operation, err, eventCtx)
}

func captureSpecialistWarning(ctx context.Context, operation string, err error, eventCtx specialistSentryContext) {
	captureSpecialistFailureWithLevel(ctx, sentrygo.LevelWarning, operation, err, eventCtx)
}

func captureSpecialistFailureWithLevel(ctx context.Context, level sentrygo.Level, operation string, err error, eventCtx specialistSentryContext) {
	if err == nil {
		return
	}
	hub := sentrygo.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentrygo.CurrentHub()
	}
	hub.WithScope(func(scope *sentrygo.Scope) {
		scope.SetLevel(level)
		scope.SetTag("feature", "specialists")
		scope.SetTag("specialist.operation", operation)
		if eventCtx.Operation != "" {
			scope.SetTag("specialist.phase", eventCtx.Operation)
		}
		if eventCtx.Status != "" {
			scope.SetTag("specialist.status", eventCtx.Status)
		}
		if eventCtx.OrgID != uuid.Nil {
			scope.SetTag("org_id", eventCtx.OrgID.String())
		}
		if eventCtx.EmployeeID != uuid.Nil {
			scope.SetTag("employee_id", eventCtx.EmployeeID.String())
		}
		if eventCtx.SpecialistID != uuid.Nil {
			scope.SetTag("specialist_id", eventCtx.SpecialistID.String())
		}
		if eventCtx.TaskID != uuid.Nil {
			scope.SetTag("specialist.task_id", eventCtx.TaskID.String())
		}
		if eventCtx.SandboxID != uuid.Nil {
			scope.SetTag("sandbox_id", eventCtx.SandboxID.String())
		}
		if eventCtx.ConversationID != uuid.Nil {
			scope.SetTag("conversation_id", eventCtx.ConversationID.String())
		}
		scope.SetContext("specialist", sentrygo.Context{
			"operation":       operation,
			"phase":           eventCtx.Operation,
			"org_id":          uuidStringOrEmpty(eventCtx.OrgID),
			"employee_id":     uuidStringOrEmpty(eventCtx.EmployeeID),
			"specialist_id":   uuidStringOrEmpty(eventCtx.SpecialistID),
			"task_id":         uuidStringOrEmpty(eventCtx.TaskID),
			"sandbox_id":      uuidStringOrEmpty(eventCtx.SandboxID),
			"conversation_id": uuidStringOrEmpty(eventCtx.ConversationID),
			"status":          eventCtx.Status,
			"reason":          eventCtx.Reason,
		})
		hub.CaptureException(fmt.Errorf("specialist %s: %w", operation, err))
	})
}

func uuidStringOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

func uuidValue(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
