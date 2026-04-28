package tasks

// Task type constants for all Asynq tasks.
const (
	// On-demand tasks (enqueued by HTTP handlers / middleware)
	TypeWebhookForward            = "webhook:forward"
	TypeAuditWrite                = "audit:write"
	TypeGenerationWrite           = "generation:write"
	TypeAPIKeyUpdate              = "apikey:update_last_used"
	TypeAdminAuditWrite           = "admin_audit:write"
	TypeEmailSend                 = "email:send"
	TypeEmailSendTemplate         = "email:send_template"
	TypeAgentCleanup              = "agent:cleanup"
	TypeBillingTokenSpend         = "billing:token_spend"
	TypeSandboxTemplateBuild      = "sandbox_template:build"
	TypeSandboxTemplateRetryBuild = "sandbox_template:retry"
	TypeSkillHydrate              = "skill:hydrate"
	TypeTriggerDispatch           = "trigger:dispatch"
	TypeRouterDispatch            = "router:dispatch"
	TypeAgentConversationCreate   = "agent:conversation_create"
	TypeConversationName          = "conversation:name"
	TypeSubscriptionDispatch      = "subscription:dispatch"
	TypeCronTriggerDispatch       = "cron_trigger:dispatch"

	// Periodic tasks (scheduled by the worker)
	TypeTokenCleanup             = "periodic:token_cleanup"
	TypeStreamCleanup            = "periodic:stream_cleanup"
	TypeSandboxHealthCheck       = "periodic:sandbox_health_check"
	TypeSandboxResourceCheck     = "periodic:sandbox_resource_check"
	TypeCronTriggerPoll          = "periodic:cron_trigger_poll"
	TypeSandboxLifecycle         = "periodic:sandbox_lifecycle"
	TypeCreditsExpire            = "periodic:credits_expire"
	TypeBillingRenewSweep        = "periodic:billing_renew_sweep"

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
