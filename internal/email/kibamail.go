package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	kibamail "github.com/kibamail/kibamail/sdks/go"
)

// KibamailSender is the synchronous sender used by the asynq worker. It calls
// the Kibamail HTTP API directly via the official Go SDK.
//
// Handlers on the web tier never call this type directly — they enqueue
// email:send / email:send_template tasks via AsynqSender, and the worker
// routes those tasks to this sender. Asynq then retries on transient
// errors (5xx, rate limits, transport failures).
type KibamailSender struct {
	client    *kibamail.Client
	fromEmail string // bare email, e.g. "betty@notifications.usehiveloop.com"
	fromName  string // optional display name, e.g. "Betty from Hiveloop"
}

// sendEmailRequestExt mirrors kibamail.SendEmailRequest but adds the
// top-level `fromName` field that the API supports but v0.1.0 of the SDK
// does not yet expose. Once a newer SDK version ships with native
// `fromName` (or a structured EmailAddress) support, swap this back to
// the SDK's typed request.
type sendEmailRequestExt struct {
	From        string                          `json:"from"`
	FromName    string                          `json:"fromName,omitempty"`
	To          interface{}                     `json:"to"`
	Subject     string                          `json:"subject,omitempty"`
	Html        string                          `json:"html,omitempty"`
	Text        string                          `json:"text,omitempty"`
	PreviewText string                          `json:"previewText,omitempty"`
	ReplyTo     *kibamail.SendEmailReplyTo      `json:"replyTo,omitempty"`
	Template    *kibamail.SendEmailTemplate     `json:"template,omitempty"`
	Attachments []kibamail.SendEmailAttachment  `json:"attachments,omitempty"`
	Metadata    map[string]string               `json:"metadata,omitempty"`
}

// NewKibamailSender builds a Kibamail-backed sender. Pass the API key, a
// bare sender email (Kibamail rejects RFC "Name <email>" syntax), and an
// optional display name. The HTTP client has a 15 s timeout so stuck
// connections don't block worker goroutines indefinitely.
func NewKibamailSender(apiKey, fromEmail, fromName string) *KibamailSender {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	client := kibamail.NewCustomClient(httpClient, apiKey)
	return &KibamailSender{client: client, fromEmail: fromEmail, fromName: fromName}
}

// send posts the request via the SDK's Client transport so we keep its
// auth header injection, base URL handling, and structured error parsing
// (APIError / RateLimitError) — we only diverge on the request body so
// we can include `fromName`.
func (s *KibamailSender) send(ctx context.Context, body *sendEmailRequestExt) (*kibamail.SendEmailResponse, error) {
	req, err := s.client.NewRequest(ctx, http.MethodPost, "v1/emails/send", body)
	if err != nil {
		return nil, err
	}
	res := new(kibamail.SendEmailResponse)
	// Perform closes resp.Body internally (see SDK client.go); the linter
	// can't see across the package boundary.
	if _, err := s.client.Perform(req, res); err != nil { //nolint:bodyclose
		return nil, err
	}
	return res, nil
}

// Send delivers a raw ad-hoc email (no template). Uses Kibamail's inline-HTML
// path — the body is sent as plain text.
func (s *KibamailSender) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("kibamail: empty recipient")
	}
	body := &sendEmailRequestExt{
		From:     s.fromEmail,
		FromName: s.fromName,
		To:       msg.To,
		Subject:  msg.Subject,
		Text:     msg.Body,
	}
	res, err := s.send(ctx, body)
	if err != nil {
		return wrapKibamailErr("send", err)
	}
	slog.Debug("kibamail: queued", "to", msg.To, "email_id", res.ID)
	return nil
}

// SendTemplate delivers an email using a published Kibamail transactional
// template. Variables are validated against the template's declared variable
// list (see templates.go) before the API call — missing required keys fail
// loudly so asynq retries don't paper over a bug.
func (s *KibamailSender) SendTemplate(ctx context.Context, msg TemplateMessage) error {
	if msg.To == "" {
		return errors.New("kibamail: empty recipient")
	}
	if msg.Slug == "" {
		return errors.New("kibamail: empty template slug")
	}
	if missing := Validate(msg.Slug, msg.Variables); len(missing) > 0 {
		return fmt.Errorf("kibamail: template %s missing variables: %v", msg.Slug, missing)
	}

	vars := make(map[string]interface{}, len(msg.Variables))
	for k, v := range msg.Variables {
		vars[k] = v
	}

	body := &sendEmailRequestExt{
		From:     s.fromEmail,
		FromName: s.fromName,
		To:       msg.To,
		Template: &kibamail.SendEmailTemplate{
			ID:        string(msg.Slug),
			Variables: vars,
		},
		Metadata: map[string]string{
			"template": string(msg.Slug),
		},
	}

	res, err := s.send(ctx, body)
	if err != nil {
		return wrapKibamailErr("send_template", err)
	}
	slog.Debug("kibamail: template queued",
		"to", msg.To, "slug", msg.Slug, "email_id", res.ID,
	)
	return nil
}

// wrapKibamailErr annotates SDK errors with typed fields useful for asynq
// retry decisions. Rate-limit is checked first so callers can honor
// RetryAfter; APIError surfaces field-level ValidationErrors so 422s are
// debuggable without round-tripping to Kibamail support.
func wrapKibamailErr(op string, err error) error {
	var rl *kibamail.RateLimitError
	if errors.As(err, &rl) {
		return fmt.Errorf("kibamail %s: rate limited (retry after %ss): %w",
			op, rl.RetryAfter, err)
	}
	var apiErr *kibamail.APIError
	if errors.As(err, &apiErr) {
		if len(apiErr.ValidationErrors) > 0 {
			parts := make([]string, 0, len(apiErr.ValidationErrors))
			for _, v := range apiErr.ValidationErrors {
				parts = append(parts, fmt.Sprintf("%s[%s]: %s", v.Field, v.Code, v.Message))
			}
			return fmt.Errorf("kibamail %s: code=%s status=%d request_id=%s validation=[%s]: %w",
				op, apiErr.Code, apiErr.StatusCode, apiErr.RequestID, strings.Join(parts, "; "), err)
		}
		return fmt.Errorf("kibamail %s: code=%s status=%d request_id=%s: %w",
			op, apiErr.Code, apiErr.StatusCode, apiErr.RequestID, err)
	}
	return fmt.Errorf("kibamail %s: %w", op, err)
}
