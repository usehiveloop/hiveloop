package tasks

import (
	"github.com/hibiken/asynq"
	polargo "github.com/polarsource/polar-go"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/skills"
	"github.com/ziraloop/ziraloop/internal/streaming"
)

// WorkerDeps holds the dependencies needed by task handlers.
type WorkerDeps struct {
	DB               *gorm.DB
	Cleanup          *streaming.Cleanup
	Orchestrator     *sandbox.Orchestrator // nil if sandbox not configured
	Pusher           *sandbox.Pusher       // nil if sandbox not configured
	EncKey           *crypto.SymmetricKey  // nil if not configured
	ForgeExecute     ForgeExecuteFunc      // nil if forge not configured
	ForgeDesignEvals ForgeDesignEvalsFunc  // nil if forge not configured
	ForgeEvalJudge   ForgeEvalJudgeFunc    // nil if forge not configured
	EmailSend        EmailSenderFunc       // nil if email not configured
	PolarClient      *polargo.Polar        // nil if billing not configured
	EventBus         *streaming.EventBus   // nil if streaming not configured
	SkillFetcher     *skills.GitFetcher    // nil disables git skill hydration
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

	// Forge orchestration
	if deps.ForgeExecute != nil {
		mux.HandleFunc(TypeForgeRun, NewForgeRunHandler(deps.ForgeExecute).Handle)
	}
	if deps.ForgeDesignEvals != nil {
		mux.HandleFunc(TypeForgeDesignEvals, NewForgeDesignEvalsHandler(deps.ForgeDesignEvals).Handle)
	}
	if deps.ForgeEvalJudge != nil {
		mux.HandleFunc(TypeForgeEvalJudge, NewForgeEvalJudgeHandler(deps.ForgeEvalJudge).Handle)
	}

	// Email sending
	if deps.EmailSend != nil {
		mux.HandleFunc(TypeEmailSend, NewEmailSendHandler(deps.EmailSend).Handle)
	}

	// Periodic task handlers
	mux.HandleFunc(TypeTokenCleanup, NewTokenCleanupHandler(deps.DB).Handle)
	mux.HandleFunc(TypeStreamCleanup, NewStreamCleanupHandler(deps.Cleanup).Handle)

	if deps.Orchestrator != nil {
		mux.HandleFunc(TypeSandboxHealthCheck, NewSandboxHealthCheckHandler(deps.Orchestrator).Handle)
		mux.HandleFunc(TypeSandboxResourceCheck, NewSandboxResourceCheckHandler(deps.Orchestrator).Handle)
	}

	if deps.Orchestrator != nil && deps.Pusher != nil {
		mux.HandleFunc(TypeSystemAgentSync, NewSystemAgentSyncHandler(deps.Orchestrator, deps.Pusher).Handle)
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

	// Trigger dispatch — runs the dispatcher for an incoming webhook,
	// produces PreparedRun blueprints, and (in the next PR) enqueues
	// per-agent run tasks.
	mux.HandleFunc(TypeTriggerDispatch, NewTriggerDispatchHandler(deps.DB).Handle)

	return mux
}
