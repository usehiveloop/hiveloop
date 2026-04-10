// Package execute is the second half of the trigger pipeline. Where the
// dispatcher decides "who should run and with what blueprint," the executor
// turns that blueprint into an actual agent run: it fires context actions
// against Nango, threads results through {{$step.x}} placeholders, and
// assembles the final opening-message prompt that would be sent to the LLM.
//
// The executor is deliberately narrow. It does NOT touch Bridge, sandboxes,
// agent_conversations rows, or anything that has to persist state across
// runs. Its one job is: given a PreparedRun and a NangoProxy, produce a
// fully-resolved instruction string. The surrounding plumbing (conversation
// creation, sandbox provisioning, asynq wiring) lives outside this package
// and calls Executor.Execute as its last step before handing the assembled
// prompt to Bridge.
//
// Keeping the executor pure-ish makes it testable end-to-end with the real
// dispatcher and the real catalog; only Nango is mocked.
package execute

import (
	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
)

// ExecutedRun is the output of Executor.Execute — the result of turning a
// dispatch.PreparedRun into real context-action calls and a final prompt.
//
// For production, the caller takes FinalInstructions, creates a Bridge
// conversation (or continues an existing one keyed by PreparedRun.ResourceKey),
// and sends FinalInstructions as the opening message.
//
// For tests, the caller just asserts on FinalInstructions. No Bridge, no
// sandboxes, no database writes — everything the test cares about lives on
// this struct.
type ExecutedRun struct {
	// Source is the PreparedRun this execution was built from. Carried
	// through so the caller has all the identity/routing info (AgentID,
	// ResourceKey, SandboxStrategy, etc.) in one place.
	Source dispatch.PreparedRun

	// FinalInstructions is the trigger's Instructions template with every
	// placeholder resolved — both $refs.x (already done by the dispatcher)
	// and {{$step.x.y}} (done here by the executor after context actions
	// return). This is the string the LLM will see as the opening user
	// message of the conversation.
	//
	// Empty when Skipped or SilentClose is true.
	FinalInstructions string

	// ContextResults holds every successful context-action response, keyed
	// by the ContextAction.As name. Preserves the raw JSON-decoded shape
	// (map[string]any / []any / scalars) exactly as returned by Nango.
	// Available for downstream consumers that want to inspect individual
	// fields beyond what the instructions template pulls.
	ContextResults map[string]any

	// ContextErrors records failures per context-action step, keyed by As.
	// A non-nil entry means the action's Nango call returned an error. When
	// the action was Optional, the entry is still recorded here but the
	// overall run continues; ContextResults[as] will be nil. When the
	// action was not optional, Execute returns the error directly and
	// ContextErrors contains the details before bail-out.
	ContextErrors map[string]error

	// Skipped mirrors the dispatcher's PreparedRun.Skipped(). If true, the
	// executor did no work — it returned immediately with an empty result.
	// Callers should check this before enqueueing downstream work.
	Skipped bool

	// SkipReason is copied from PreparedRun.SkipReason for convenience.
	SkipReason string

	// SilentClose is true when the source run is a terminate with
	// SilentClose=true. The executor short-circuits these runs entirely —
	// no context actions fire, no instructions are built. The downstream
	// caller uses Source.ResourceKey to find and close the existing
	// conversation without running the agent.
	SilentClose bool
}

// IsExecutable reports whether this run should produce a downstream agent
// conversation. False when Skipped or SilentClose; true otherwise. Callers
// use this as their enqueue filter.
func (r *ExecutedRun) IsExecutable() bool {
	if r == nil {
		return false
	}
	if r.Skipped {
		return false
	}
	if r.SilentClose {
		return false
	}
	return true
}
