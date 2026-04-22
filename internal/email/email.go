package email

import "context"

// Sender is the abstraction for sending emails. Implementations enqueue
// to a background queue (production), send synchronously via the Kibamail
// API (worker), or log to stdout (dev/tests).
type Sender interface {
	// Send delivers an ad-hoc email with a raw subject and plaintext body.
	// Used for free-form messages that aren't backed by a published template.
	Send(ctx context.Context, msg Message) error

	// SendTemplate delivers an email using a published Kibamail transactional
	// template referenced by slug. Variables are merged into the template's
	// Handlebars placeholders server-side by Kibamail.
	SendTemplate(ctx context.Context, msg TemplateMessage) error
}

// Message represents an outgoing ad-hoc email (no template).
type Message struct {
	To      string
	Subject string
	Body    string
}

// TemplateMessage represents an outgoing template-backed email.
type TemplateMessage struct {
	// To is the recipient address.
	To string
	// Slug is the Kibamail unique template slug (e.g. "auth-otp-login").
	// Use the typed constants from templates.go (e.g. TmplAuthOtpLogin).
	Slug TemplateSlug
	// Variables are the Handlebars {{key}} substitutions. Every key declared
	// in templateVars for the slug MUST be present and non-empty — the
	// worker validates this before calling Kibamail.
	Variables TemplateVars
}
