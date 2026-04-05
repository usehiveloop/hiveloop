package tasks

// Task type constants for all Asynq tasks.
const (
	// On-demand tasks (enqueued by HTTP handlers / middleware)
	TypeForgeRun        = "forge:run"
	TypeWebhookForward  = "webhook:forward"
	TypeAuditWrite      = "audit:write"
	TypeGenerationWrite = "generation:write"
	TypeAPIKeyUpdate    = "apikey:update_last_used"
	TypeAdminAuditWrite = "admin_audit:write"
	TypeEmailSend       = "email:send"
	TypeSystemAgentSeed = "system_agent:seed"
	TypeAgentCleanup    = "agent:cleanup"

	// Periodic tasks (scheduled by the worker)
	TypeTokenCleanup          = "periodic:token_cleanup"
	TypeStreamCleanup         = "periodic:stream_cleanup"
	TypeSandboxHealthCheck    = "periodic:sandbox_health_check"
	TypeSandboxResourceCheck  = "periodic:sandbox_resource_check"
)

// Queue names with priority weights.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueBulk     = "bulk"
	QueuePeriodic = "periodic"
)
