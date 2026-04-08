package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/ziraloop/ziraloop/internal/model"
)

// ---------------------------------------------------------------------------
// webhook:forward
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// forge:run
// ---------------------------------------------------------------------------

// ForgeRunPayload is the payload for TypeForgeRun tasks.
type ForgeRunPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

// NewForgeRunTask creates a task that executes a forge run.
func NewForgeRunTask(runID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(ForgeRunPayload{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("marshal forge run payload: %w", err)
	}
	return asynq.NewTask(
		TypeForgeRun,
		payload,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
		asynq.Unique(24*time.Hour),
	), nil
}

// ---------------------------------------------------------------------------
// forge:design_evals
// ---------------------------------------------------------------------------

// ForgeDesignEvalsPayload is the payload for TypeForgeDesignEvals tasks.
type ForgeDesignEvalsPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

// NewForgeDesignEvalsTask creates a task that generates eval cases for a forge run.
func NewForgeDesignEvalsTask(runID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(ForgeDesignEvalsPayload{RunID: runID})
	if err != nil {
		return nil, fmt.Errorf("marshal forge design evals payload: %w", err)
	}
	return asynq.NewTask(
		TypeForgeDesignEvals,
		payload,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(10*time.Minute),
		asynq.Unique(24*time.Hour),
	), nil
}

// ---------------------------------------------------------------------------
// email:send
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// apikey:update_last_used
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// admin_audit:write
// ---------------------------------------------------------------------------

// AdminAuditWritePayload is the payload for TypeAdminAuditWrite tasks.
type AdminAuditWritePayload struct {
	Entry model.AdminAuditEntry `json:"entry"`
}

// NewAdminAuditWriteTask creates a task that writes an admin audit log entry.
func NewAdminAuditWriteTask(entry model.AdminAuditEntry) (*asynq.Task, error) {
	payload, err := json.Marshal(AdminAuditWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal admin audit payload: %w", err)
	}
	return asynq.NewTask(
		TypeAdminAuditWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}

// ---------------------------------------------------------------------------
// audit:write
// ---------------------------------------------------------------------------

// AuditWritePayload is the payload for TypeAuditWrite tasks.
type AuditWritePayload struct {
	Entry model.AuditEntry `json:"entry"`
}

// NewAuditWriteTask creates a task that writes an audit log entry.
func NewAuditWriteTask(entry model.AuditEntry) (*asynq.Task, error) {
	payload, err := json.Marshal(AuditWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}
	return asynq.NewTask(
		TypeAuditWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}

// ---------------------------------------------------------------------------
// generation:write
// ---------------------------------------------------------------------------

// GenerationWritePayload is the payload for TypeGenerationWrite tasks.
type GenerationWritePayload struct {
	Entry model.Generation `json:"entry"`
}

// NewGenerationWriteTask creates a task that writes a generation record.
func NewGenerationWriteTask(entry model.Generation) (*asynq.Task, error) {
	payload, err := json.Marshal(GenerationWritePayload{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("marshal generation payload: %w", err)
	}
	return asynq.NewTask(
		TypeGenerationWrite,
		payload,
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10*time.Second),
	), nil
}

// ---------------------------------------------------------------------------
// agent:cleanup
// ---------------------------------------------------------------------------

// AgentCleanupPayload is the payload for TypeAgentCleanup tasks.
type AgentCleanupPayload struct {
	AgentID uuid.UUID `json:"agent_id"`
}

// NewAgentCleanupTask creates a task that cleans up an agent's sandboxes and then hard-deletes it.
func NewAgentCleanupTask(agentID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(AgentCleanupPayload{AgentID: agentID})
	if err != nil {
		return nil, fmt.Errorf("marshal agent cleanup payload: %w", err)
	}
	return asynq.NewTask(
		TypeAgentCleanup,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}
