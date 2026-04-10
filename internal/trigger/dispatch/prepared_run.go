// Package dispatch decides which agents should run in response to a webhook,
// builds the fully-resolved request blueprints, and returns them. It does not
// execute Nango calls or create conversations — that's the executor's job.
//
// The dispatcher is provider-agnostic. Callers (Nango webhook handler, custom
// per-provider HTTP endpoints) resolve the connection themselves and pass it
// in via DispatchInput.
package dispatch

import (
	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/model"
)

// SandboxStrategy describes how the executor should obtain a sandbox for an
// agent run. Shared agents reuse a pool sandbox; dedicated agents get a fresh
// sandbox per run.
type SandboxStrategy string

const (
	SandboxStrategyReusePool       SandboxStrategy = "reuse_pool"
	SandboxStrategyCreateDedicated SandboxStrategy = "create_dedicated"
)

// RunIntent tells the executor which lifecycle bucket a PreparedRun belongs
// to. Normal runs create-or-continue a conversation and send the incoming
// event. Terminate runs are triggered by an event listed in the parent's
// terminate_on config — they either send a final message and close the
// conversation (graceful) or close silently without sending.
//
// The dispatcher is responsible for distinguishing normal vs terminate based
// on which list the event key appears in; the executor reads RunIntent to
// decide its flow.
type RunIntent string

const (
	RunIntentNormal    RunIntent = "normal"
	RunIntentTerminate RunIntent = "terminate"
)

// DispatchInput is the resolved webhook envelope handed to the dispatcher.
type DispatchInput struct {
	Provider    string            // catalog provider key, e.g. "github"
	EventType   string            // top-level event, e.g. "issues" (X-GitHub-Event header)
	EventAction string            // sub-action, e.g. "opened" (payload.action). Empty for actionless events like push.
	Payload     map[string]any    // raw webhook body, JSON-decoded
	DeliveryID  string            // provider delivery id (e.g. X-GitHub-Delivery), used for tracing/dedup
	OrgID       uuid.UUID         // resolved by the caller
	Connection  *model.Connection // resolved by the caller (already loaded with Integration preloaded)
}

// TriggerKey returns the catalog trigger key for this input.
// For action-bearing events: "<event_type>.<event_action>" (e.g. "issues.opened").
// For actionless events: just "<event_type>" (e.g. "push").
func (di DispatchInput) TriggerKey() string {
	if di.EventAction == "" {
		return di.EventType
	}
	return di.EventType + "." + di.EventAction
}

// PreparedRun is the fully-resolved blueprint for a single agent run.
// The executor takes a PreparedRun and turns it into Nango calls + a Bridge
// conversation. Skipped runs (filtered out by conditions) are still returned
// with SkipReason populated for observability.
type PreparedRun struct {
	OrgID          uuid.UUID
	AgentID        uuid.UUID
	AgentTriggerID uuid.UUID
	ConnectionID   uuid.UUID
	NangoConnID    string // copied from Connection.NangoConnectionID for the executor
	ProviderCfgKey string // {orgID}_{integrationUniqueKey} — Nango provider config key
	Provider       string
	TriggerKey     string // the matched key, e.g. "issues.opened"

	// RunIntent tells the executor which flow to route through:
	//   - RunIntentNormal: create-or-continue conversation, send instructions, run agent
	//   - RunIntentTerminate: if SilentClose, close the existing conversation without
	//     running the agent. Otherwise, create-or-continue conversation, send the final
	//     instructions, run the agent one last time, then close the conversation.
	//
	// Defaults to RunIntentNormal. Set to RunIntentTerminate only when the incoming
	// event matches a TerminateRule on the parent AgentTrigger.
	RunIntent RunIntent

	// SilentClose is meaningful only when RunIntent == RunIntentTerminate. True means
	// "just close the existing conversation and exit" — no context actions, no final
	// message, no LLM call, no Nango proxying. The executor looks up the existing
	// conversation by (AgentID, ConnectionID, ResourceKey) and marks it closed. If no
	// conversation exists, the executor logs and does nothing.
	SilentClose bool

	// ResourceKey is a stable identifier for the subject resource of this event —
	// the thing an agent conversation should be attached to. Events on the same
	// resource share a conversation with the same agent.
	//
	// Empty string means "always create a new conversation" (e.g., push events
	// have no continuation semantics). The executor uses the tuple
	// (AgentID, ConnectionID, ResourceKey) as the lookup key.
	//
	// Computed at dispatch time by substituting $refs.x into the resource's
	// ResourceKeyTemplate from the catalog. Provider-agnostic — nothing here
	// knows which provider it came from.
	ResourceKey string

	// SandboxStrategy decides whether the executor reuses a pool sandbox or
	// provisions a fresh one. ReusePool runs use SandboxID as the target.
	SandboxStrategy SandboxStrategy
	SandboxID       *uuid.UUID

	// Refs is the resolved entity ref map (e.g. owner=octocat, repo=Hello-World, issue_number=1347)
	// extracted from the webhook payload using catalog TriggerDef.Refs.
	Refs map[string]string

	// ContextRequests are the read-only API calls the executor must fire (in order)
	// before starting the conversation. Optional requests may fail without blocking.
	ContextRequests []ContextRequest

	// Instructions is the agent's prompt template with $refs.x already substituted.
	// Any remaining {{$step.x}} placeholders refer to context-action results and are
	// resolved by the executor after each context request returns.
	Instructions string

	// DeferredVars lists every {{$step.x}} placeholder that survived dispatch-time
	// substitution. The executor uses this to know which steps need result-substitution
	// and to detect dangling references.
	DeferredVars []string

	// Skipped runs are still returned for observability. SkipReason is human-readable
	// and identifies which check failed (e.g. "condition 0: sender.login not_one_of failed").
	// Skipped runs are NOT enqueued to the executor.
	SkipReason string
}

// Skipped reports whether this run was filtered out and should not be executed.
func (p PreparedRun) Skipped() bool { return p.SkipReason != "" }

// ContextRequest is one fully-resolved read API call. The executor fires this
// against Nango using the action's catalog Execution config (method, headers).
// Path is already substituted with refs; Query and Body have $refs.x replaced
// but may still contain {{$step.x}} placeholders if they reference earlier
// context steps' results.
type ContextRequest struct {
	As           string            // context bag key, e.g. "issue", "files"
	ActionKey    string            // catalog action key, e.g. "issues_get"
	Method       string            // HTTP method copied from action.Execution.Method
	Path         string            // already substituted with refs (e.g. /repos/octocat/Hello-World/issues/1347)
	Query        map[string]string // resolved query params (may contain {{$step.x}})
	Body         map[string]any    // resolved body params (may contain {{$step.x}})
	Headers      map[string]string // copied from action.Execution.Headers
	Optional     bool              // failure does not block the run
	DeferredVars []string          // {{$step.x}} placeholders found in this request's params
}
