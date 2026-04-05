package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// EmailSenderFunc is a function that sends an email.
// This avoids importing the email package (which could create import cycles).
type EmailSenderFunc func(ctx context.Context, to, subject, body string) error

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

	if err := h.send(ctx, p.To, p.Subject, p.Body); err != nil {
		return fmt.Errorf("send email to %s: %w", p.To, err)
	}

	slog.Debug("email sent", "to", p.To, "subject", p.Subject)
	return nil
}
