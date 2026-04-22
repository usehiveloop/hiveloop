package email

import (
	"context"
	"log/slog"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// AsynqSender implements Sender by enqueueing email tasks for async delivery.
// The actual sending happens in the Asynq worker via the email:send and
// email:send_template task handlers — which call KibamailSender under the
// hood. Asynq transparently retries on transient failures.
type AsynqSender struct {
	enqueuer enqueue.TaskEnqueuer
}

// NewAsynqSender creates an AsynqSender.
func NewAsynqSender(enqueuer enqueue.TaskEnqueuer) *AsynqSender {
	return &AsynqSender{enqueuer: enqueuer}
}

// Send enqueues an ad-hoc email for async delivery.
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

// SendTemplate enqueues a template-backed email for async delivery. The worker
// validates variables against the slug before calling Kibamail.
func (s *AsynqSender) SendTemplate(_ context.Context, msg TemplateMessage) error {
	task, err := tasks.NewEmailSendTemplateTask(msg.To, string(msg.Slug), msg.Variables)
	if err != nil {
		return err
	}
	if _, err := s.enqueuer.Enqueue(task); err != nil {
		slog.Error("failed to enqueue template email",
			"error", err, "to", msg.To, "slug", msg.Slug,
		)
		return err
	}
	return nil
}
