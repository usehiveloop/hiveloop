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

// ForgeEvalJudgePayload is the payload for TypeForgeEvalJudge tasks.
// Each task runs ONE eval case end-to-end: eval target → judge → save score.
type ForgeEvalJudgePayload struct {
	RunID          uuid.UUID `json:"run_id"`
	IterationID    uuid.UUID `json:"iteration_id"`
	EvalResultID   uuid.UUID `json:"eval_result_id"`
	EvalCaseID     uuid.UUID `json:"eval_case_id"`
	MaxConcurrency int       `json:"max_concurrency"`
}

// NewForgeEvalJudgeTask creates a task that runs one eval case and judges it.
func NewForgeEvalJudgeTask(payload ForgeEvalJudgePayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal forge eval judge payload: %w", err)
	}
	return asynq.NewTask(
		TypeForgeEvalJudge,
		data,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(2),
		asynq.Timeout(5*time.Minute),
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
// billing:usage_event
// ---------------------------------------------------------------------------

// BillingUsageEventPayload is the payload for TypeBillingUsageEvent tasks.
type BillingUsageEventPayload struct {
	OrgID       uuid.UUID `json:"org_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	SandboxType string    `json:"sandbox_type"` // "shared" or "dedicated"
	RunID       uuid.UUID `json:"run_id"`
}

// NewBillingUsageEventTask creates a task that sends a usage event to Polar.
func NewBillingUsageEventTask(orgID, agentID, runID uuid.UUID, sandboxType string) (*asynq.Task, error) {
	payload, err := json.Marshal(BillingUsageEventPayload{
		OrgID:       orgID,
		AgentID:     agentID,
		SandboxType: sandboxType,
		RunID:       runID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal billing usage event payload: %w", err)
	}
	return asynq.NewTask(
		TypeBillingUsageEvent,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(15*time.Second),
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

// ---------------------------------------------------------------------------
// sandbox_template:build
// ---------------------------------------------------------------------------

// SandboxTemplateBuildPayload is the payload for TypeSandboxTemplateBuild tasks.
type SandboxTemplateBuildPayload struct {
	TemplateID uuid.UUID `json:"template_id"`
}

// NewSandboxTemplateBuildTask creates a task that builds a sandbox template snapshot.
func NewSandboxTemplateBuildTask(templateID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(SandboxTemplateBuildPayload{TemplateID: templateID})
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox template build payload: %w", err)
	}
	return asynq.NewTask(
		TypeSandboxTemplateBuild,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
	), nil
}

// ---------------------------------------------------------------------------
// sandbox_template:retry
// ---------------------------------------------------------------------------

// SandboxTemplateRetryBuildPayload is the payload for retry tasks.
type SandboxTemplateRetryBuildPayload struct {
	TemplateID    uuid.UUID `json:"template_id"`
	BuildCommands []string  `json:"build_commands,omitempty"`
}

// NewSandboxTemplateRetryBuildTask creates a task that retries building a sandbox template.
func NewSandboxTemplateRetryBuildTask(templateID uuid.UUID, buildCommands []string) (*asynq.Task, error) {
	payload := SandboxTemplateRetryBuildPayload{
		TemplateID:    templateID,
		BuildCommands: buildCommands,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sandbox template retry payload: %w", err)
	}
	return asynq.NewTask(
		TypeSandboxTemplateRetryBuild,
		data,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(2),
		asynq.Timeout(30*time.Minute),
	), nil
}

// ---------------------------------------------------------------------------
// skill:hydrate
// ---------------------------------------------------------------------------

// SkillHydratePayload is the payload for TypeSkillHydrate tasks.
type SkillHydratePayload struct {
	SkillID uuid.UUID `json:"skill_id"`
}

// NewSkillHydrateTask creates a task that pulls a git-sourced skill at its
// tracked ref and writes a new SkillVersion.
func NewSkillHydrateTask(skillID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(SkillHydratePayload{SkillID: skillID})
	if err != nil {
		return nil, fmt.Errorf("marshal skill hydrate payload: %w", err)
	}
	return asynq.NewTask(
		TypeSkillHydrate,
		payload,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2*time.Minute),
	), nil
}

// ---------------------------------------------------------------------------
// trigger:dispatch
// ---------------------------------------------------------------------------

// TriggerDispatchPayload carries everything the dispatcher needs to decide
// which agents should run for an incoming webhook. The connection is encoded
// by ID — the worker reloads it from the DB so we don't carry secrets across
// the queue boundary.
//
// PayloadJSON is the raw webhook body as bytes (not parsed) so the worker can
// log/replay it verbatim. The dispatcher decodes it on demand.
type TriggerDispatchPayload struct {
	Provider     string    `json:"provider"`
	EventType    string    `json:"event_type"`
	EventAction  string    `json:"event_action"`
	DeliveryID   string    `json:"delivery_id"`
	OrgID        uuid.UUID `json:"org_id"`
	ConnectionID uuid.UUID `json:"connection_id"`
	PayloadJSON  []byte    `json:"payload"`
}

// NewTriggerDispatchTask creates a task that runs the dispatcher for a webhook.
//
// MaxRetry is intentionally low (3) — dispatch is fast and any error here is
// either a transient DB issue (worth a retry) or a programmer error (more
// retries don't help). Long timeouts are unnecessary; the dispatch step is
// pure CPU + one DB query, so 30s is generous.
func NewTriggerDispatchTask(payload TriggerDispatchPayload) (*asynq.Task, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger dispatch payload: %w", err)
	}
	return asynq.NewTask(
		TypeTriggerDispatch,
		encoded,
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Second),
	), nil
}
