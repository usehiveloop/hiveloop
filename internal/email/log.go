package email

import (
	"context"
	"log/slog"
)

// LogSender logs emails via slog instead of sending them.
// Useful for development — confirmation and reset links appear in server logs.
type LogSender struct{}

func (s *LogSender) Send(_ context.Context, msg Message) error {
	slog.Info("email", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}
