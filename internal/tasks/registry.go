package tasks

import (
	"github.com/hibiken/asynq"
	polargo "github.com/polarsource/polar-go"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/nango"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
	"github.com/usehiveloop/hiveloop/internal/skills"
	"github.com/usehiveloop/hiveloop/internal/streaming"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
	"github.com/usehiveloop/hiveloop/internal/trigger/enrichment"
)

// WorkerDeps holds the dependencies needed by task handlers.
type WorkerDeps struct {
	DB               *gorm.DB
	Cleanup          *streaming.Cleanup
	Orchestrator     *sandbox.Orchestrator // nil if sandbox not configured
	Pusher           *sandbox.Pusher       // nil if sandbox not configured
	EncKey           *crypto.SymmetricKey  // nil if not configured
	EmailSend         EmailSenderFunc         // nil if email not configured
	EmailSendTemplate EmailTemplateSenderFunc // nil if template email not configured
	PolarClient      *polargo.Polar        // nil if billing not configured
	EventBus         *streaming.EventBus   // nil if streaming not configured
	SkillFetcher     *skills.GitFetcher    // nil disables git skill hydration
	NangoClient      *nango.Client         // nil disables deterministic enrichment
	CacheManager     *cache.Manager        // nil disables tasks that need credential decryption
	Enqueuer         enqueue.TaskEnqueuer  // required for enqueuing sub-tasks
}

// NewServeMux creates an Asynq ServeMux with all task handlers registered.
func NewServeMux(deps *WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()

	// On-demand write handlers
	mux.HandleFunc(TypeAPIKeyUpdate, NewAPIKeyHandler(deps.DB).Handle)
	mux.HandleFunc(TypeAdminAuditWrite, NewAdminAuditHandler(deps.DB).Handle)
	mux.HandleFunc(TypeAuditWrite, NewAuditHandler(deps.DB).Handle)
	mux.HandleFunc(TypeGenerationWrite, NewGenerationHandler(deps.DB).Handle)

	// Webhook forwarding
	mux.HandleFunc(TypeWebhookForward, NewWebhookForwardHandler(deps.EncKey).Handle)

	// Email sending
	if deps.EmailSend != nil {
		mux.HandleFunc(TypeEmailSend, NewEmailSendHandler(deps.EmailSend).Handle)
	}
	if deps.EmailSendTemplate != nil {
		mux.HandleFunc(TypeEmailSendTemplate, NewEmailSendTemplateHandler(deps.EmailSendTemplate).Handle)
	}

	// Periodic task handlers
	mux.HandleFunc(TypeTokenCleanup, NewTokenCleanupHandler(deps.DB).Handle)
	mux.HandleFunc(TypeStreamCleanup, NewStreamCleanupHandler(deps.Cleanup).Handle)

	if deps.Orchestrator != nil {
		mux.HandleFunc(TypeSandboxHealthCheck, NewSandboxHealthCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxResourceCheck, NewSandboxResourceCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxLifecycle, NewSandboxLifecycleHandler(deps.Orchestrator).Handle)
	}

	// Agent cleanup (works with or without orchestrator/pusher — handles nil gracefully)
	mux.HandleFunc(TypeAgentCleanup, NewAgentCleanupHandler(deps.DB, deps.Orchestrator, deps.Pusher).Handle)

	// Sandbox template build
	if deps.Orchestrator != nil {
		handler := NewSandboxTemplateBuildHandler(deps.DB, deps.Orchestrator)
		mux.HandleFunc(TypeSandboxTemplateBuild, handler.Handle)
		mux.HandleFunc(TypeSandboxTemplateRetryBuild, handler.HandleRetry)
	}

	// Billing usage event
	if deps.PolarClient != nil {
		mux.HandleFunc(TypeBillingUsageEvent, NewBillingUsageEventHandler(deps.DB, deps.PolarClient).Handle)
	}

	// Skill hydration from git repos
	if deps.SkillFetcher != nil {
		mux.HandleFunc(TypeSkillHydrate, NewSkillHydrateHandler(deps.DB, deps.SkillFetcher).Handle)
	}

	// Conversation naming (async title generation from the first message).
	// Requires the cache manager for credential decryption.
	if deps.CacheManager != nil {
		if handler := NewConversationNameHandler(deps.DB, deps.CacheManager); handler != nil {
			mux.HandleFunc(TypeConversationName, handler.Handle)
		}
	}

	// Router dispatch (Zira routing system).
	// Only registered when orchestrator + pusher are available (sandbox configured).
	if deps.Orchestrator != nil && deps.Pusher != nil {
		routerDispatcher := dispatch.NewRouterDispatcher(
			dispatch.NewGormRouterTriggerStore(deps.DB, catalog.Global()),
			catalog.Global(),
			nil, // RouterAgent wired separately when credential picker is ready
			nil, // logger — defaults to slog.Default()
		)
		routerHandler := NewRouterDispatchHandler(routerDispatcher, deps.Enqueuer)
		if deps.NangoClient != nil {
			routerHandler.SetDeterministicEnrichment(
				enrichment.NewDeterministicEnricher(deps.NangoClient, catalog.Global(), deps.DB),
			)
		}
		mux.HandleFunc(TypeRouterDispatch, routerHandler.Handle)

		// Cron trigger dispatch (scheduled agent conversations).
		mux.HandleFunc(TypeCronTriggerDispatch,
			NewCronTriggerDispatchHandler(routerDispatcher, deps.Enqueuer).Handle)

		// Agent conversation creation (sandbox provisioning + Bridge push + first message).
		mux.HandleFunc(TypeAgentConversationCreate,
			NewAgentConversationCreateHandler(deps.DB, deps.Orchestrator, deps.Pusher).Handle)

		// Subscription dispatch (fans webhook events into subscribed conversations).
		mux.HandleFunc(TypeSubscriptionDispatch,
			NewSubscriptionDispatchHandler(deps.DB, deps.Orchestrator, catalog.Global()).Handle)
	}

	// Cron trigger poller (runs even without orchestrator — the dispatch tasks
	// will fail gracefully if sandbox isn't configured).
	mux.HandleFunc(TypeCronTriggerPoll,
		NewCronTriggerPollHandler(deps.DB, deps.Enqueuer).Handle)

	return mux
}
