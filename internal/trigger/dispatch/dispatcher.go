package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/model"
)

// Dispatcher decides which agents should run for an incoming webhook and
// builds the fully-resolved blueprints. It does not execute Nango calls or
// create conversations — that's the executor's job.
//
// A Dispatcher is safe for concurrent use. All state lives on the input
// (the DispatchInput passed to Run) or in the injected dependencies.
type Dispatcher struct {
	Triggers AgentTriggerStore
	Catalog  *catalog.Catalog
	Logger   *slog.Logger
}

// New constructs a Dispatcher with the given dependencies. Logger is required
// because every dispatch decision is logged for production debugging.
func New(triggers AgentTriggerStore, cat *catalog.Catalog, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		Triggers: triggers,
		Catalog:  cat,
		Logger:   logger,
	}
}

// Run is the dispatch entry point. It receives a resolved webhook envelope and
// returns a slice of PreparedRun blueprints — one per matched agent trigger,
// including skipped runs (with SkipReason populated) for observability.
//
// The slice is in deterministic order (by AgentTrigger.ID). The executor is
// expected to filter out Skipped() runs before enqueueing them.
//
// Errors from the trigger store (DB issues) bubble up. Validation errors per
// individual run (catalog miss, dangling step ref, ambiguous key) are
// recorded as SkipReason on the affected run, not as a top-level error.
func (d *Dispatcher) Run(ctx context.Context, input DispatchInput) ([]PreparedRun, error) {
	if input.Connection == nil {
		return nil, ErrNilConnection
	}

	triggerKey := input.TriggerKey()
	logger := d.Logger.With(
		"delivery_id", input.DeliveryID,
		"provider", input.Provider,
		"trigger_key", triggerKey,
		"org_id", input.OrgID,
		"connection_id", input.Connection.ID,
	)
	logger.Info("dispatch: webhook received")

	triggerCatalog := d.lookupProviderTriggers(input.Provider)
	if triggerCatalog == nil {
		logger.Warn("dispatch: provider has no triggers in catalog, ignoring")
		return nil, ErrUnknownProvider
	}

	triggerDef, hasTrigger := triggerCatalog.Triggers[triggerKey]
	if !hasTrigger {
		logger.Info("dispatch: trigger key not in catalog, ignoring", "available_count", len(triggerCatalog.Triggers))
		return nil, nil
	}

	// Extract refs + resolve the resource key once per webhook. Every run
	// produced for this webhook shares the same refs map and resource key.
	refs, missing := extractRefs(input.Payload, triggerDef.Refs)
	if len(missing) > 0 {
		logger.Warn("dispatch: some refs missing from payload", "missing", missing)
	}
	resourceKey := resolveResourceKey(d.Catalog, input.Provider, triggerDef.ResourceType, refs)
	logger.Info("dispatch: refs resolved", "refs", refs, "resource_key", resourceKey, "resource_type", triggerDef.ResourceType)

	// Find all enabled agent triggers on this connection where the event key
	// appears in EITHER trigger_keys (normal) or terminate_event_keys (terminate).
	matches, err := d.Triggers.FindMatching(ctx, input.OrgID, input.Connection.ID, []string{triggerKey})
	if err != nil {
		logger.Error("dispatch: failed to load matching agent triggers", "error", err)
		return nil, fmt.Errorf("loading agent triggers: %w", err)
	}
	logger.Info("dispatch: agent triggers matched", "count", len(matches))

	if len(matches) == 0 {
		return nil, nil
	}

	providerCfgKey := input.OrgID.String() + "_" + input.Connection.Integration.UniqueKey

	runs := make([]PreparedRun, 0, len(matches))
	for _, match := range matches {
		isNormal := containsString(match.Trigger.TriggerKeys, triggerKey)
		terminateRule, hasTerminate := findMatchingTerminateRule(match.Trigger, triggerKey, input.Payload, logger)

		// Ambiguous config: event key is listed in BOTH trigger_keys and
		// terminate_on. We reject at save time in the handler, but double-check
		// here as a safety net so drift (direct DB edits, migrations) doesn't
		// produce silent wrong behavior.
		if isNormal && hasTerminate {
			logger.Error("dispatch: ambiguous event key, listed in both trigger_keys and terminate_on",
				"agent_trigger_id", match.Trigger.ID,
				"trigger_key", triggerKey,
			)
			run := baseRun(input, triggerKey, refs, resourceKey, match, providerCfgKey)
			run.SkipReason = "ambiguous: event key is in both trigger_keys and terminate_on"
			runs = append(runs, run)
			continue
		}

		switch {
		case isNormal:
			runs = append(runs, d.buildNormalRun(ctx, input, triggerKey, refs, resourceKey, match, providerCfgKey, logger))
		case hasTerminate:
			runs = append(runs, d.buildTerminateRun(ctx, input, triggerKey, refs, resourceKey, match, *terminateRule, providerCfgKey, logger))
		default:
			// Store returned this trigger because of array overlap on the
			// terminate column, but no rule actually matched the key + conditions.
			// This is fine — skip silently.
			logger.Debug("dispatch: trigger matched by store but no terminate rule fired",
				"agent_trigger_id", match.Trigger.ID,
			)
		}
	}

	d.Logger.Info("dispatch: complete",
		"delivery_id", input.DeliveryID,
		"trigger_key", triggerKey,
		"runs_total", len(runs),
		"runs_skipped", countSkipped(runs),
	)

	return runs, nil
}

// lookupProviderTriggers handles the variant fallback. The actions catalog has
// per-variant entries (github, github-app, github-app-oauth, github-pat) but
// triggers live only under the base provider name. We strip dash-suffixes
// progressively until a match is found.
func (d *Dispatcher) lookupProviderTriggers(provider string) *catalog.ProviderTriggers {
	if pt, ok := d.Catalog.GetProviderTriggers(provider); ok {
		return pt
	}
	if pt, ok := d.Catalog.GetProviderTriggersForVariant(provider); ok {
		return pt
	}
	return nil
}

// baseRun builds the shared identity fields every PreparedRun carries. Used
// by both the normal and terminate paths so they start from the same place.
func baseRun(
	input DispatchInput,
	triggerKey string,
	refs map[string]string,
	resourceKey string,
	match TriggerWithAgent,
	providerCfgKey string,
) PreparedRun {
	run := PreparedRun{
		OrgID:          input.OrgID,
		AgentID:        match.Agent.ID,
		AgentTriggerID: match.Trigger.ID,
		ConnectionID:   input.Connection.ID,
		NangoConnID:    input.Connection.NangoConnectionID,
		ProviderCfgKey: providerCfgKey,
		Provider:       input.Provider,
		TriggerKey:     triggerKey,
		RunIntent:      RunIntentNormal,
		ResourceKey:    resourceKey,
		Refs:           refs,
	}
	switch match.Agent.SandboxType {
	case "dedicated":
		run.SandboxStrategy = SandboxStrategyCreateDedicated
	default:
		run.SandboxStrategy = SandboxStrategyReusePool
		run.SandboxID = match.Agent.SandboxID
	}
	return run
}

// buildNormalRun builds a PreparedRun for an event listed in the trigger's
// TriggerKeys (the normal create-or-continue flow). It evaluates the trigger's
// own conditions, builds context requests from its own context_actions, and
// substitutes refs into its instructions.
func (d *Dispatcher) buildNormalRun(
	ctx context.Context,
	input DispatchInput,
	triggerKey string,
	refs map[string]string,
	resourceKey string,
	match TriggerWithAgent,
	providerCfgKey string,
	parentLogger *slog.Logger,
) PreparedRun {
	_ = ctx

	logger := parentLogger.With(
		"agent_trigger_id", match.Trigger.ID,
		"agent_id", match.Agent.ID,
		"intent", RunIntentNormal,
	)

	run := baseRun(input, triggerKey, refs, resourceKey, match, providerCfgKey)

	conditions, err := parseConditions(match.Trigger.Conditions)
	if err != nil {
		run.SkipReason = "invalid conditions JSON: " + err.Error()
		logger.Warn("dispatch: skipped, invalid conditions", "error", err)
		return run
	}
	if reason, passed := evaluateConditions(conditions, input.Payload); !passed {
		run.SkipReason = reason
		logger.Info("dispatch: skipped, conditions did not match", "reason", reason)
	}

	contextActions, err := parseContextActions(match.Trigger.ContextActions)
	if err != nil {
		if run.SkipReason == "" {
			run.SkipReason = "invalid context_actions JSON: " + err.Error()
		}
		logger.Warn("dispatch: invalid context_actions", "error", err)
		return run
	}

	requests, buildErrs := buildContextRequests(d.Catalog, input.Provider, contextActions, refs, triggerKey)
	if len(buildErrs) > 0 {
		for _, errMsg := range buildErrs {
			logger.Warn("dispatch: context request build error", "error", errMsg)
		}
		if run.SkipReason == "" {
			run.SkipReason = "context_actions build error: " + buildErrs[0]
		}
	}
	run.ContextRequests = requests
	run.Instructions = substituteRefs(match.Trigger.Instructions, refs)
	run.DeferredVars = collectDeferredVars(requests)
	for _, stepName := range findStepReferences(run.Instructions) {
		if !containsString(run.DeferredVars, stepName) {
			run.DeferredVars = append(run.DeferredVars, stepName)
		}
	}

	logger.Info("dispatch: normal run built",
		"context_requests", len(run.ContextRequests),
		"deferred_vars", run.DeferredVars,
		"skip_reason", run.SkipReason,
		"sandbox_strategy", run.SandboxStrategy,
	)
	return run
}

// buildTerminateRun builds a PreparedRun for an event listed in a TerminateRule.
// Conditions are inherited from the parent trigger by default (so "skip drafts"
// on the parent also skips draft PRs on close), unless the rule sets
// ignore_parent_conditions. The rule's own conditions/context/instructions are
// applied on top.
//
// If the rule is Silent, the returned run has SilentClose=true and no context
// requests or instructions. The executor looks up the existing conversation by
// ResourceKey and closes it without running the agent. If no resource key
// resolved, the terminate run is skipped — there's nothing to close without a
// lookup key.
func (d *Dispatcher) buildTerminateRun(
	ctx context.Context,
	input DispatchInput,
	triggerKey string,
	refs map[string]string,
	resourceKey string,
	match TriggerWithAgent,
	rule model.TerminateRule,
	providerCfgKey string,
	parentLogger *slog.Logger,
) PreparedRun {
	_ = ctx

	logger := parentLogger.With(
		"agent_trigger_id", match.Trigger.ID,
		"agent_id", match.Agent.ID,
		"intent", RunIntentTerminate,
	)

	run := baseRun(input, triggerKey, refs, resourceKey, match, providerCfgKey)
	run.RunIntent = RunIntentTerminate
	run.SilentClose = rule.Silent

	// A terminate run needs a resource key to find the conversation to close —
	// without one, there's no way to correlate this event with an existing run.
	// Skip loudly so a missing catalog template shows up in logs.
	if resourceKey == "" {
		run.SkipReason = "terminate requires a resource key, but none resolved for this resource type"
		logger.Warn("dispatch: terminate skipped, no resource key",
			"resource_type", match.Trigger.ID, // keep for debugging — agent trigger id
		)
		return run
	}

	// Inherit parent conditions unless the rule opts out.
	if !rule.IgnoreParentConditions {
		parentConditions, err := parseConditions(match.Trigger.Conditions)
		if err != nil {
			run.SkipReason = "invalid parent conditions JSON: " + err.Error()
			logger.Warn("dispatch: terminate skipped, invalid parent conditions", "error", err)
			return run
		}
		if reason, passed := evaluateConditions(parentConditions, input.Payload); !passed {
			run.SkipReason = "parent " + reason
			logger.Info("dispatch: terminate skipped, parent conditions did not match", "reason", reason)
			// Don't return yet — we still populate the run for observability,
			// but the executor will skip it based on SkipReason.
		}
	}

	// Apply rule's own conditions on top (only if parent passed).
	if run.SkipReason == "" && rule.Conditions != nil {
		if reason, passed := evaluateConditions(rule.Conditions, input.Payload); !passed {
			run.SkipReason = "terminate rule " + reason
			logger.Info("dispatch: terminate skipped, rule conditions did not match", "reason", reason)
		}
	}

	// Silent closes carry nothing else: no context actions, no instructions.
	// The executor's entire job is "close the conversation by ResourceKey".
	if rule.Silent {
		logger.Info("dispatch: silent terminate run built", "skip_reason", run.SkipReason)
		return run
	}

	// Build context requests from the rule's own context_actions.
	requests, buildErrs := buildContextRequests(d.Catalog, input.Provider, rule.ContextActions, refs, triggerKey)
	if len(buildErrs) > 0 {
		for _, errMsg := range buildErrs {
			logger.Warn("dispatch: terminate context request build error", "error", errMsg)
		}
		if run.SkipReason == "" {
			run.SkipReason = "terminate context_actions build error: " + buildErrs[0]
		}
	}
	run.ContextRequests = requests
	run.Instructions = substituteRefs(rule.Instructions, refs)
	run.DeferredVars = collectDeferredVars(requests)
	for _, stepName := range findStepReferences(run.Instructions) {
		if !containsString(run.DeferredVars, stepName) {
			run.DeferredVars = append(run.DeferredVars, stepName)
		}
	}

	logger.Info("dispatch: terminate run built",
		"context_requests", len(run.ContextRequests),
		"deferred_vars", run.DeferredVars,
		"skip_reason", run.SkipReason,
		"silent", rule.Silent,
		"sandbox_strategy", run.SandboxStrategy,
	)
	return run
}

// findMatchingTerminateRule parses TerminateOn and returns the first rule
// whose TriggerKeys contains the event key AND whose own conditions pass
// against the payload. Rules are evaluated in order, first-pass wins — this
// supports patterns like [{merged:true}, {merged:false, silent:true}] where
// the "merged" rule takes precedence when applicable.
//
// Returns (nil, false) when TerminateOn is empty/invalid or no rule matches.
// Malformed JSON is logged and treated as "no terminate rule" — a dispatch
// concern, not a caller error.
func findMatchingTerminateRule(
	trigger model.AgentTrigger,
	triggerKey string,
	payload map[string]any,
	logger *slog.Logger,
) (*model.TerminateRule, bool) {
	if len(trigger.TerminateOn) == 0 {
		return nil, false
	}
	var rules []model.TerminateRule
	if err := json.Unmarshal(trigger.TerminateOn, &rules); err != nil {
		logger.Warn("dispatch: invalid terminate_on JSON, ignoring", "error", err, "agent_trigger_id", trigger.ID)
		return nil, false
	}
	for index := range rules {
		rule := rules[index]
		if !containsString(rule.TriggerKeys, triggerKey) {
			continue
		}
		// Evaluate the rule's OWN conditions here to implement first-pass-wins.
		// Parent conditions are applied later in buildTerminateRun so they can
		// populate SkipReason for observability on the skipped run.
		if rule.Conditions != nil {
			if _, passed := evaluateConditions(rule.Conditions, payload); !passed {
				continue
			}
		}
		return &rule, true
	}
	return nil, false
}

// resolveResourceKey substitutes $refs.x into the resource's template. Returns
// empty string when the resource has no template, the resource type is empty,
// or substitution left any $refs.x unresolved (partial resolution would silently
// merge unrelated resources, so we fail closed).
func resolveResourceKey(cat *catalog.Catalog, provider, resourceType string, refs map[string]string) string {
	if resourceType == "" {
		return ""
	}
	resourceDef, ok := cat.GetResourceDef(provider, resourceType)
	if !ok {
		// Fall back to variant lookup (github-app → github) for providers where
		// resources are defined under the base name.
		if baseResourceDef, found := tryVariantResource(cat, provider, resourceType); found {
			resourceDef = baseResourceDef
		} else {
			return ""
		}
	}
	if resourceDef.ResourceKeyTemplate == "" {
		return ""
	}
	resolved := substituteRefs(resourceDef.ResourceKeyTemplate, refs)
	if strings.Contains(resolved, "$refs.") {
		return ""
	}
	return resolved
}

// tryVariantResource walks up the provider name (github-app → github) looking
// for a resource definition. The trigger catalog already does this for
// ProviderTriggers; we do the same for resources because some providers define
// actions per-variant but resources only at the base name.
func tryVariantResource(cat *catalog.Catalog, provider, resourceType string) (*catalog.ResourceDef, bool) {
	name := provider
	for {
		idx := strings.LastIndex(name, "-")
		if idx <= 0 {
			return nil, false
		}
		name = name[:idx]
		if rd, ok := cat.GetResourceDef(name, resourceType); ok {
			return rd, true
		}
	}
}

func collectDeferredVars(requests []ContextRequest) []string {
	seen := make(map[string]bool)
	var out []string
	for _, request := range requests {
		for _, deferred := range request.DeferredVars {
			if seen[deferred] {
				continue
			}
			seen[deferred] = true
			out = append(out, deferred)
		}
	}
	return out
}

func countSkipped(runs []PreparedRun) int {
	count := 0
	for _, run := range runs {
		if run.Skipped() {
			count++
		}
	}
	return count
}

// Compile-time interface assertion: AgentTrigger and Agent must remain in the
// model package and the dispatcher must keep its dependency surface tiny so
// the test fakes are easy to write.
var _ = model.AgentTrigger{}
