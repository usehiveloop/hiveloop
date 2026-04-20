package email

import (
	"context"
	"log/slog"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// AsynqSender implements Sender by enqueueing email tasks for async delivery.
// The actual sending happens in the Asynq worker via the email:send task handler.
type AsynqSender struct {
	enqueuer enqueue.TaskEnqueuer
}

// NewAsynqSender creates an AsynqSender.
func NewAsynqSender(enqueuer enqueue.TaskEnqueuer) *AsynqSender {
	return &AsynqSender{enqueuer: enqueuer}
}

// Send enqueues an email for async delivery.
func (s *AsynqSender) Send(_ context.Context, msg Message) error {
	task, err := tasks.NewEmailSendTask(msg.To, msg.Subject, msg.Body)
	if err != nil {
		return err
	}
	if _, err := s.enqueuer.Enqueue(task); err != nil {
		slog.Error("failed to enqueue email", "error", err, "to", msg.To)
		return err
	}
	return nil
}
