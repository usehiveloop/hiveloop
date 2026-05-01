package email

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/logging"
)

// LogSender logs emails via slog instead of sending them.
// Useful for development — confirmation and reset links appear in server logs.
type LogSender struct{}

// Send logs a raw ad-hoc message.
func (s *LogSender) Send(ctx context.Context, msg Message) error {
	logging.FromContext(ctx).InfoContext(ctx, "email", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}

// SendTemplate logs a template-backed message. Variables are included so OTP
// codes, confirmation URLs, etc. surface in local dev logs.
func (s *LogSender) SendTemplate(ctx context.Context, msg TemplateMessage) error {
	logging.FromContext(ctx).InfoContext(ctx, "email (template)",
		"to", msg.To,
		"slug", msg.Slug,
		"variables", msg.Variables,
	)
	return nil
}
