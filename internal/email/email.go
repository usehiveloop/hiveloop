package email

import "context"

// Sender is the abstraction for sending emails. Implementations enqueue to a
// background queue, send synchronously via Resend in the worker, or log to
// stdout in dev/tests.
type Sender interface {
	// Send delivers an ad-hoc email with a raw subject and plaintext body.
	// Used for free-form messages that aren't backed by a published template.
	Send(ctx context.Context, msg Message) error

	// SendTemplate delivers an email using a published transactional template
	// referenced by slug.
	SendTemplate(ctx context.Context, msg TemplateMessage) error
}

// Message represents an outgoing ad-hoc email (no template).
type Message struct {
	To             string
	Subject        string
	Body           string
	IdempotencyKey string
}

// TemplateMessage represents an outgoing template-backed email.
type TemplateMessage struct {
	// To is the recipient address.
	To string
	// Slug is the Resend template alias (e.g. "auth-otp-login").
	// Use the typed constants from templates.go (e.g. TmplAuthOtpLogin).
	Slug TemplateSlug
	// Variables are the template substitutions. Every key declared in
	// templateVars for the slug must be present and non-empty.
	Variables      TemplateVars
	IdempotencyKey string
}
