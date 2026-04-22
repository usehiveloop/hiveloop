package email

import (
	"context"
	"log/slog"
)

// LogSender logs emails via slog instead of sending them.
// Useful for development — confirmation and reset links appear in server logs.
type LogSender struct{}

// Send logs a raw ad-hoc message.
func (s *LogSender) Send(_ context.Context, msg Message) error {
	slog.Info("email", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}

// SendTemplate logs a template-backed message. Variables are included so OTP
// codes, confirmation URLs, etc. surface in local dev logs.
func (s *LogSender) SendTemplate(_ context.Context, msg TemplateMessage) error {
	slog.Info("email (template)",
		"to", msg.To,
		"slug", msg.Slug,
		"variables", msg.Variables,
	)
	return nil
}
