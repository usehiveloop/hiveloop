package email

import "context"

// Sender is the abstraction for sending emails.
// Implementations can use SMTP, an API provider, or just log the message.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Message represents an outgoing email.
type Message struct {
	To      string
	Subject string
	Body    string
}
