package dispatch

import (
	"encoding/json"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// buildContextRequests turns []ContextAction → []ContextRequest, fully resolving
// refs and path templates. Returns a slice of requests in the same order as the
// input actions, plus a list of validation errors (one per action that failed).
//
// Validation errors include: action not in catalog, ref names a non-existent
// resource type, {{$step.x}} references a step that doesn't appear earlier in
// the action list. The dispatcher logs errors but still returns the partial
// list — the executor will skip any request whose ActionKey is empty.
//
// Path parameter resolution order (highest priority wins):
//  1. Explicit params from ContextAction.Params (with $refs.x substituted)
//  2. Resource ref_bindings (resolved from catalog ResourceDef.RefBindings)
//  3. Refs map (auto-fills any leftover {param} placeholders that match a ref name)
//
// Query and body params come from action.Execution.QueryMapping / BodyMapping.
func buildContextRequests(
	cat *catalog.Catalog,
	provider string,
	actions []model.ContextAction,
	refs map[string]string,
	triggerKey string,
) (requests []ContextRequest, errs []string) {
	requests = make([]ContextRequest, 0, len(actions))
	earlierSteps := make(map[string]bool, len(actions))

	for index, contextAction := range actions {
		// only_when filter — skip when current trigger key is not in the list.
		if len(contextAction.OnlyWhen) > 0 && !containsString(contextAction.OnlyWhen, triggerKey) {
			continue
		}

		actionDef, ok := cat.GetAction(provider, contextAction.Action)
		if !ok {
			errs = append(errs, fmt.Sprintf("context_actions[%d] %q: action not in catalog", index, contextAction.Action))
			earlierSteps[contextAction.As] = true
			continue
		}
		if actionDef.Execution == nil {
			errs = append(errs, fmt.Sprintf("context_actions[%d] %q: action has no execution config", index, contextAction.Action))
			earlierSteps[contextAction.As] = true
			continue
		}

		// 1. Start with resource ref_bindings (resolves "ref: issue" → owner/repo/issue_number).
		params := make(map[string]string)
		if contextAction.Ref != "" {
			resourceDef, found := cat.GetResourceDef(provider, contextAction.Ref)
			if !found {
				errs = append(errs, fmt.Sprintf("context_actions[%d] %q: ref %q is not a resource of provider %q", index, contextAction.Action, contextAction.Ref, provider))
			} else {
				for paramName, refExpr := range resourceDef.RefBindings {
					// refExpr is "$refs.x" — substitute against refs map.
					params[paramName] = substituteRefs(refExpr, refs)
				}
			}
		}

		// 2. Layer explicit params on top (caller overrides). Substitute $refs.x;
		//    leave {{$step.x}} placeholders for the executor.
		for paramName, paramValue := range contextAction.Params {
			scalar := stringifyScalar(paramValue)
			params[paramName] = substituteRefs(scalar, refs)
		}

		// 3. Build the request.
		execution := actionDef.Execution

		// Path substitution: prefer the explicit/binding params, then fall back
		// to the refs map for any path placeholder that matches a ref name.
		// This catches paths like /repos/{owner}/{repo} when the action has no
		// resource ref (some search endpoints).
		pathValues := make(map[string]string, len(params)+len(refs))
		for refName, refValue := range refs {
			pathValues[refName] = refValue
		}
		for paramName, paramValue := range params {
			pathValues[paramName] = paramValue
		}
		path := substitutePathParams(execution.Path, pathValues)

		// Split params into query and body using the action's mapping config.
		// Anything not mapped is path-only and was consumed above.
		query := make(map[string]string)
		body := make(map[string]any)
		for paramName, paramValue := range params {
			if execution.QueryMapping != nil {
				if queryKey, ok := execution.QueryMapping[paramName]; ok {
					query[queryKey] = paramValue
					continue
				}
			}
			if execution.BodyMapping != nil {
				if bodyKey, ok := execution.BodyMapping[paramName]; ok {
					body[bodyKey] = paramValue
					continue
				}
			}
		}

		// Detect deferred {{$step.x}} placeholders across path, query, body, and
		// validate that each referenced step appears earlier in the action list.
		var deferredVars []string
		for _, value := range query {
			for _, step := range findStepReferences(value) {
				if !earlierSteps[step] {
					errs = append(errs, fmt.Sprintf("context_actions[%d] %q: references step %q that does not appear earlier", index, contextAction.As, step))
				}
				if !containsString(deferredVars, step) {
					deferredVars = append(deferredVars, step)
				}
			}
		}
		for _, raw := range body {
			value, ok := raw.(string)
			if !ok {
				continue
			}
			for _, step := range findStepReferences(value) {
				if !earlierSteps[step] {
					errs = append(errs, fmt.Sprintf("context_actions[%d] %q: references step %q that does not appear earlier", index, contextAction.As, step))
				}
				if !containsString(deferredVars, step) {
					deferredVars = append(deferredVars, step)
				}
			}
		}

		var headers map[string]string
		if len(execution.Headers) > 0 {
			headers = make(map[string]string, len(execution.Headers))
			for key, value := range execution.Headers {
				headers[key] = value
			}
		}

		requests = append(requests, ContextRequest{
			As:           contextAction.As,
			ActionKey:    contextAction.Action,
			Method:       execution.Method,
			Path:         path,
			Query:        query,
			Body:         body,
			Headers:      headers,
			Optional:     contextAction.Optional,
			DeferredVars: deferredVars,
		})

		earlierSteps[contextAction.As] = true
	}

	return requests, errs
}

// parseConditions unmarshals the AgentTrigger.Conditions JSONB blob into a
// TriggerMatch. Empty conditions return nil.
func parseConditions(raw model.RawJSON) (*model.TriggerMatch, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var match model.TriggerMatch
	if err := json.Unmarshal(raw, &match); err != nil {
		return nil, err
	}
	return &match, nil
}

// parseContextActions unmarshals the AgentTrigger.ContextActions JSONB blob.
func parseContextActions(raw model.RawJSON) ([]model.ContextAction, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var actions []model.ContextAction
	if err := json.Unmarshal(raw, &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

func containsString(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
