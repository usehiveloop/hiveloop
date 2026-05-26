package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/logging"
)

// EmailSenderFunc is a function that sends an ad-hoc email.
// This avoids importing the email package (which could create import cycles).
type EmailSenderFunc func(ctx context.Context, to, subject, body, idempotencyKey string) error

// EmailTemplateSenderFunc is a function that sends a template-backed email via
// slug + variables. Lives on the worker and typically calls the configured
// transactional email provider.
type EmailTemplateSenderFunc func(ctx context.Context, to, slug string, variables map[string]string, idempotencyKey string) error

// EmailSendHandler processes email:send tasks.
type EmailSendHandler struct {
	send EmailSenderFunc
}

// NewEmailSendHandler creates an email send handler.
func NewEmailSendHandler(send EmailSenderFunc) *EmailSendHandler {
	return &EmailSendHandler{send: send}
}

// Handle sends an email.
func (h *EmailSendHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p EmailSendPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal email payload: %w", err)
	}

	if err := h.send(ctx, p.To, p.Subject, p.Body, p.IdempotencyKey); err != nil {
		return fmt.Errorf("send email to %s: %w", p.To, err)
	}

	logging.FromContext(ctx).DebugContext(ctx, "email sent", "to", p.To, "subject", p.Subject)
	return nil
}

// EmailSendTemplateHandler processes email:send_template tasks.
type EmailSendTemplateHandler struct {
	send EmailTemplateSenderFunc
}

// NewEmailSendTemplateHandler creates a template email send handler.
func NewEmailSendTemplateHandler(send EmailTemplateSenderFunc) *EmailSendTemplateHandler {
	return &EmailSendTemplateHandler{send: send}
}

// Handle dispatches a template-backed email through the configured sender.
// Errors returned from this handler trigger asynq's built-in retry policy
// (MaxRetry(5) with exponential backoff) — which is the whole reason template
// sends go through the queue instead of the hot HTTP path.
func (h *EmailSendTemplateHandler) Handle(ctx context.Context, t *asynq.Task) error {
	var p EmailSendTemplatePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("unmarshal email template payload: %w", err)
	}

	if err := h.send(ctx, p.To, p.Slug, p.Variables, p.IdempotencyKey); err != nil {
		return fmt.Errorf("send template %s to %s: %w", p.Slug, p.To, err)
	}

	logging.FromContext(ctx).DebugContext(ctx, "email template sent", "to", p.To, "slug", p.Slug)
	return nil
}
