package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// WebhookForwardPayload is the payload for TypeWebhookForward tasks.
type WebhookForwardPayload struct {
	WebhookURL      string `json:"webhook_url"`
	EncryptedSecret []byte `json:"encrypted_secret"`
	Body            []byte `json:"body"`
}

// NewWebhookForwardTask creates a task that forwards a webhook to an org's endpoint.
func NewWebhookForwardTask(webhookURL string, encryptedSecret []byte, body []byte) (*asynq.Task, error) {
	payload, err := json.Marshal(WebhookForwardPayload{
		WebhookURL:      webhookURL,
		EncryptedSecret: encryptedSecret,
		Body:            body,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal webhook forward payload: %w", err)
	}
	return asynq.NewTask(
		TypeWebhookForward,
		payload,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(30*time.Second),
	), nil
}

// EmailSendPayload is the payload for TypeEmailSend tasks.
type EmailSendPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// NewEmailSendTask creates a task that sends an email.
func NewEmailSendTask(to, subject, body string) (*asynq.Task, error) {
	payload, err := json.Marshal(EmailSendPayload{To: to, Subject: subject, Body: body})
	if err != nil {
		return nil, fmt.Errorf("marshal email send payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmailSend,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(30*time.Second),
	), nil
}

// EmailSendTemplatePayload is the payload for TypeEmailSendTemplate tasks.
// Variables is a flat string map — Kibamail templates use Handlebars
// {{key}} substitution and every value is already stringified when enqueued.
type EmailSendTemplatePayload struct {
	To        string            `json:"to"`
	Slug      string            `json:"slug"`
	Variables map[string]string `json:"variables,omitempty"`
}

// NewEmailSendTemplateTask creates a task that sends an email via a
// published Kibamail transactional template (resolved by slug).
func NewEmailSendTemplateTask(to, slug string, variables map[string]string) (*asynq.Task, error) {
	payload, err := json.Marshal(EmailSendTemplatePayload{
		To:        to,
		Slug:      slug,
		Variables: variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal email template payload: %w", err)
	}
	return asynq.NewTask(
		TypeEmailSendTemplate,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(30*time.Second),
	), nil
}

// APIKeyUpdatePayload is the payload for TypeAPIKeyUpdate tasks.
type APIKeyUpdatePayload struct {
	KeyID uuid.UUID `json:"key_id"`
}

// NewAPIKeyUpdateTask creates a task that updates an API key's last_used_at.
func NewAPIKeyUpdateTask(keyID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(APIKeyUpdatePayload{KeyID: keyID})
	if err != nil {
		return nil, fmt.Errorf("marshal apikey update payload: %w", err)
	}
	return asynq.NewTask(
		TypeAPIKeyUpdate,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}
