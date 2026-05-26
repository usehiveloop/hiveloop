package email

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	resend "github.com/resend/resend-go/v3"
)

type ResendSender struct {
	client *resend.Client
	from   string
}

func NewResendSender(apiKey, from string) *ResendSender {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	client := resend.NewCustomClient(httpClient, apiKey)
	return &ResendSender{client: client, from: from}
}

func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("resend: empty recipient")
	}
	if msg.Subject == "" {
		return errors.New("resend: empty subject")
	}

	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Text:    msg.Body,
	}
	if _, err := s.client.Emails.SendWithOptions(ctx, params, sendOptions(msg.IdempotencyKey)); err != nil {
		return fmt.Errorf("resend send: %w", err)
	}
	return nil
}

func (s *ResendSender) SendTemplate(ctx context.Context, msg TemplateMessage) error {
	if msg.To == "" {
		return errors.New("resend: empty recipient")
	}
	if msg.Slug == "" {
		return errors.New("resend: empty template slug")
	}
	if missing := Validate(msg.Slug, msg.Variables); len(missing) > 0 {
		return fmt.Errorf("resend: template %s missing variables: %v", msg.Slug, missing)
	}

	vars := make(map[string]any, len(msg.Variables))
	for k, v := range msg.Variables {
		vars[k] = v
	}

	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{msg.To},
		Subject: Subject(msg.Slug, msg.Variables),
		Template: &resend.EmailTemplate{
			Id:        string(msg.Slug),
			Variables: vars,
		},
		Tags: []resend.Tag{
			{Name: "template", Value: string(msg.Slug)},
		},
	}
	if _, err := s.client.Emails.SendWithOptions(ctx, params, sendOptions(msg.IdempotencyKey)); err != nil {
		return fmt.Errorf("resend send_template: %w", err)
	}
	return nil
}

func sendOptions(idempotencyKey string) *resend.SendEmailOptions {
	if idempotencyKey == "" {
		return nil
	}
	return &resend.SendEmailOptions{IdempotencyKey: idempotencyKey}
}
