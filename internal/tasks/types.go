package tasks

// Task type constants for all Asynq tasks.
const (
	// On-demand tasks (enqueued by HTTP handlers / middleware)
	TypeWebhookForward            = "webhook:forward"
	TypeAuditWrite                = "audit:write"
	TypeGenerationWrite           = "generation:write"
	TypeAPIKeyUpdate              = "apikey:update_last_used" // #nosec G101 -- task type identifier, not a credential
	TypeAdminAuditWrite           = "admin_audit:write"
	TypeEmailSend                 = "email:send"
	TypeEmailSendTemplate         = "email:send_template"
	TypeAgentCleanup              = "agent:cleanup"
	TypeAgentProfileNangoCleanup  = "agent_profile:nango_cleanup"
	TypeSandboxTemplateBuild      = "sandbox_template:build"
	TypeSandboxTemplateRetryBuild = "sandbox_template:retry"
	TypeSkillHydrate              = "skill:hydrate"
	TypeEmployeeTriggerDispatch   = "employee_trigger:dispatch"
	TypeConversationName          = "conversation:name"
	TypeEmployeeMemoryRetain      = "employee:memory_retain"
	TypeEmployeeMemoryRefresh     = "employee:memory_refresh"
	TypeEmployeeSandboxUpgrade    = "employee:sandbox_upgrade"
	TypeEmployeeSandboxRetire     = "employee:sandbox_retire"

	// Periodic tasks (scheduled by the worker)
	TypeTokenCleanup         = "periodic:token_cleanup"
	TypeStreamCleanup        = "periodic:stream_cleanup"
	TypeSandboxHealthCheck   = "periodic:sandbox_health_check"
	TypeSandboxResourceCheck = "periodic:sandbox_resource_check"
	TypeSandboxLifecycle     = "periodic:sandbox_lifecycle"
	TypeCreditsExpire        = "periodic:credits_expire"
	TypeBillingBatchProcess  = "periodic:billing_batch_process"
	TypeBillingRenewSweep    = "periodic:billing_renew_sweep"

	// On-demand task enqueued by the sweep for each due subscription.
	TypeBillingRenewSubscription = "billing:renew_subscription"
)

// Queue names with priority weights.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueBulk     = "bulk"
	QueuePeriodic = "periodic"
)
