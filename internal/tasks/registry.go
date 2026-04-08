package tasks

import (
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/streaming"
)

// WorkerDeps holds the dependencies needed by task handlers.
// This avoids importing the bootstrap package (which would create an import cycle).
type WorkerDeps struct {
	DB           *gorm.DB
	Cleanup      *streaming.Cleanup
	Orchestrator *sandbox.Orchestrator // nil if sandbox not configured
	Pusher       *sandbox.Pusher      // nil if sandbox not configured
	EncKey       *crypto.SymmetricKey  // nil if not configured
	ForgeExecute     ForgeExecuteFunc      // nil if forge not configured
	ForgeDesignEvals ForgeDesignEvalsFunc   // nil if forge not configured
	EmailSend        EmailSenderFunc       // nil if email not configured
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

	// Agent cleanup (works with or without orchestrator/pusher — handles nil gracefully)
	mux.HandleFunc(TypeAgentCleanup, NewAgentCleanupHandler(deps.DB, deps.Orchestrator, deps.Pusher).Handle)

	return mux
}
