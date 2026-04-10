package execute

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/trigger/dispatch"
)

// Executor turns a dispatch.PreparedRun into an ExecutedRun with fully
// resolved context and a final LLM instruction. It's the second half of
// the trigger pipeline: the dispatcher decides what should run, the
// executor actually runs it.
//
// See the package doc on executed_run.go for the overall philosophy.
type Executor struct {
	Nango   NangoProxy
	Catalog *catalog.Catalog
	Logger  *slog.Logger
}

// New constructs an Executor with the given dependencies. Logger defaults
// to slog.Default if nil; the other two are required.
func New(nango NangoProxy, cat *catalog.Catalog, logger *slog.Logger) (*Executor, error) {
	if nango == nil {
		return nil, ErrNilNango
	}
	if cat == nil {
		return nil, ErrNilCatalog
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Executor{
		Nango:   nango,
		Catalog: cat,
		Logger:  logger,
	}, nil
}

// Execute is the single entry point. It walks the PreparedRun's context
// requests in order, fires each through the NangoProxy, threads the results
// into subsequent requests (via {{$step.x}} substitution), and finally
// resolves the run's Instructions template into the prompt that would be
// sent to the LLM as the opening user message.
//
// Behaviors in order:
//
//  1. If the run is Skipped (dispatcher set SkipReason), return an
//     ExecutedRun with Skipped=true and no further work.
//  2. If the run is a silent terminate, return with SilentClose=true and
//     no further work — the caller uses ResourceKey to close the
//     existing conversation.
//  3. Otherwise, iterate ContextRequests:
//     a. Substitute {{$step.x}} placeholders in Query, Body, and Path
//        from the step bag (filled by prior iterations).
//     b. Call Nango.Proxy with the resolved request.
//     c. On success, store the response in the step bag keyed by As.
//     d. On error, if the action is Optional, record the error in
//        ContextErrors, set an empty step, and continue. Otherwise,
//        return ErrRequiredContextFailed wrapped with the original error.
//  4. After all context actions complete, substitute {{$step.x}} in the
//     run's Instructions using the fully-populated step bag. This is the
//     FinalInstructions — the load-bearing output.
//
// Execute does NOT touch Bridge, sandboxes, or any database state. The
// caller takes the ExecutedRun and decides what to do with it.
func (e *Executor) Execute(ctx context.Context, run dispatch.PreparedRun) (*ExecutedRun, error) {
	logger := e.Logger.With(
		"trigger_key", run.TriggerKey,
		"agent_trigger_id", run.AgentTriggerID,
		"agent_id", run.AgentID,
		"resource_key", run.ResourceKey,
	)

	// Skipped runs pass through untouched.
	if run.Skipped() {
		logger.Info("execute: run was skipped by dispatcher", "skip_reason", run.SkipReason)
		return &ExecutedRun{
			Source:     run,
			Skipped:    true,
			SkipReason: run.SkipReason,
		}, nil
	}

	// Silent terminate runs short-circuit. The caller handles the
	// conversation close using the ResourceKey.
	if run.SilentClose {
		logger.Info("execute: silent close, no context/instructions work")
		return &ExecutedRun{
			Source:      run,
			SilentClose: true,
		}, nil
	}

	result := &ExecutedRun{
		Source:         run,
		ContextResults: make(map[string]any),
		ContextErrors:  make(map[string]error),
	}

	bag := newStepBag()

	for index, contextReq := range run.ContextRequests {
		stepLogger := logger.With(
			"step", contextReq.As,
			"action", contextReq.ActionKey,
			"index", index,
		)

		// Resolve {{$step.x}} placeholders in path, query, and body using
		// results from earlier steps. The dispatcher already substituted
		// $refs.x before producing this request; step placeholders are
		// the only thing that survives to execution time.
		proxyReq := ProxyRequest{
			Method:         contextReq.Method,
			ProviderCfgKey: run.ProviderCfgKey,
			NangoConnID:    run.NangoConnID,
			Path:           bag.substituteStepPlaceholders(contextReq.Path),
			Query:          bag.substituteInStringMap(contextReq.Query),
			Body:           bag.substituteInParams(contextReq.Body),
			Headers:        contextReq.Headers,
		}

		stepLogger.Info("execute: firing context action",
			"method", proxyReq.Method,
			"path", proxyReq.Path,
			"query_keys", mapKeys(proxyReq.Query),
			"body_keys", mapKeysAny(proxyReq.Body),
		)

		response, err := e.Nango.Proxy(ctx, proxyReq)
		if err != nil {
			result.ContextErrors[contextReq.As] = err
			if contextReq.Optional {
				stepLogger.Warn("execute: optional context action failed, continuing with empty result", "error", err)
				bag.set(contextReq.As, nil)
				continue
			}
			stepLogger.Error("execute: required context action failed", "error", err)
			return result, fmt.Errorf("%w: %s: %v", ErrRequiredContextFailed, contextReq.As, err)
		}

		bag.set(contextReq.As, response)
		stepLogger.Info("execute: context action succeeded",
			"response_shape", describeResponseShape(response),
		)
	}

	// Final instructions = trigger instructions with every remaining
	// {{$step.x}} placeholder resolved from the now-complete step bag.
	// $refs.x was already substituted by the dispatcher.
	result.FinalInstructions = bag.substituteStepPlaceholders(run.Instructions)
	result.ContextResults = bag.snapshot()

	logger.Info("execute: run complete",
		"final_instructions_len", len(result.FinalInstructions),
		"context_results_count", len(result.ContextResults),
		"context_errors_count", len(result.ContextErrors),
	)

	return result, nil
}

// mapKeys returns the keys of a string map for logging. Not performance
// critical; log construction only happens when the log level is enabled.
func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func mapKeysAny(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// describeResponseShape returns a short log-friendly summary of a Nango
// response value. Objects are described by their top-level keys; arrays
// by element count; scalars by their type.
func describeResponseShape(value any) string {
	switch typed := value.(type) {
	case nil:
		return "nil"
	case map[string]any:
		return fmt.Sprintf("object(%d keys)", len(typed))
	case []any:
		return fmt.Sprintf("array(%d elements)", len(typed))
	case string:
		return "string"
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	default:
		return fmt.Sprintf("%T", typed)
	}
}
