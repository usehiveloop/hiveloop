package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	client *kibamail.Client
	from   string // e.g. "Betty from Hiveloop <betty@notifications.usehiveloop.com>"
}

// NewKibamailSender builds a Kibamail-backed sender. Pass the API key and
// a From header formatted as `Name <address@domain>`. The underlying HTTP
// client has a 15 s timeout so stuck connections don't block worker
// goroutines indefinitely.
func NewKibamailSender(apiKey, from string) *KibamailSender {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	client := kibamail.NewCustomClient(httpClient, apiKey)
	return &KibamailSender{client: client, from: from}
}

// Send delivers a raw ad-hoc email (no template). Uses Kibamail's inline-HTML
// path — the body is sent as plain text.
func (s *KibamailSender) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("kibamail: empty recipient")
	}
	req := &kibamail.SendEmailRequest{
		From:    s.from,
		To:      msg.To,
		Subject: msg.Subject,
		Text:    msg.Body,
	}
	res, err := s.client.Emails.SendWithContext(ctx, req)
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

	// Convert map[string]string → map[string]interface{} for the SDK.
	vars := make(map[string]interface{}, len(msg.Variables))
	for k, v := range msg.Variables {
		vars[k] = v
	}

	req := &kibamail.SendEmailRequest{
		From: s.from,
		To:   msg.To,
		Template: &kibamail.SendEmailTemplate{
			ID:        string(msg.Slug),
			Variables: vars,
		},
		Metadata: map[string]string{
			"template": string(msg.Slug),
		},
	}

	res, err := s.client.Emails.SendWithContext(ctx, req)
	if err != nil {
		return wrapKibamailErr("send_template", err)
	}
	slog.Debug("kibamail: template queued",
		"to", msg.To, "slug", msg.Slug, "email_id", res.ID,
	)
	return nil
}

// wrapKibamailErr annotates SDK errors with typed fields useful for asynq
// retry decisions. Rate-limit errors and 5xx/transport errors are left
// un-classified here — asynq already retries every non-nil error, and the
// SDK's `*RateLimitError` / `*APIError` are embedded for downstream logging.
func wrapKibamailErr(op string, err error) error {
	var apiErr *kibamail.APIError
	if errors.As(err, &apiErr) {
		return fmt.Errorf("kibamail %s: code=%s status=%d request_id=%s: %w",
			op, apiErr.Code, apiErr.StatusCode, apiErr.RequestID, err)
	}
	var rl *kibamail.RateLimitError
	if errors.As(err, &rl) {
		return fmt.Errorf("kibamail %s: rate limited (retry after %ss): %w",
			op, rl.RetryAfter, err)
	}
	return fmt.Errorf("kibamail %s: %w", op, err)
}
