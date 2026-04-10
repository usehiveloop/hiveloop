package execute

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// stepBag holds the results of every context action that has already run,
// keyed by ContextAction.As. Later steps and the final instructions template
// look up values here via {{$step.field.nested}} placeholders.
//
// The bag stores raw JSON-decoded values (map[string]any, []any, scalars) —
// nothing is stringified until it's used in a template substitution. This
// lets dot-path lookups walk nested structures naturally.
type stepBag struct {
	results map[string]any
}

func newStepBag() *stepBag {
	return &stepBag{results: make(map[string]any)}
}

// set stores the result of a context action. Nil values are still stored
// (empty step) so later lookups distinguish "step did run but returned
// nothing" from "step doesn't exist."
func (b *stepBag) set(stepName string, value any) {
	b.results[stepName] = value
}

// get returns the raw value for a step name, or nil + false if the step
// hasn't run yet or was never in the context list.
func (b *stepBag) get(stepName string) (any, bool) {
	value, ok := b.results[stepName]
	return value, ok
}

// snapshot returns a shallow copy of the results map. Used to populate
// ExecutedRun.ContextResults without exposing the internal map for
// mutation.
func (b *stepBag) snapshot() map[string]any {
	out := make(map[string]any, len(b.results))
	for key, value := range b.results {
		out[key] = value
	}
	return out
}

// Placeholder syntax recognized by the executor:
//
//   {{$step_name}}            — whole step result, JSON-stringified if object/array
//   {{$step_name.field}}      — dot-path into a nested field
//   {{$step_name.a.b.c}}      — arbitrarily deep dot-path
//
// Step names must start with a letter or underscore and contain only word
// characters. This matches the syntax the dispatcher uses when recording
// DeferredVars — both layers agree on what a step placeholder looks like.
//
// The `$refs` prefix is NOT matched here — that was handled at dispatch
// time and the result string passed to the executor has no $refs.x left.
var stepPlaceholderRegex = regexp.MustCompile(`\{\{\s*\$([A-Za-z_][\w]*)(?:\.([\w.]+))?\s*\}\}`)

// substituteStepPlaceholders replaces every {{$step.x.y}} occurrence in the
// input string with the resolved value from the step bag. The resolution
// rules:
//
//   - Scalar values (string, number, bool) render as their string form
//   - Objects and arrays JSON-stringify with 2-space indent (LLM-readable)
//   - Missing steps render as "[missing: step_name]"
//   - Present step but missing nested field renders as "[missing: step.a.b]"
//   - nil step result renders as empty string (treat as "ran but returned
//     nothing")
//
// The function scans the input once and returns a new string; it does NOT
// mutate the bag. Used both for between-step param resolution and for the
// final instructions assembly.
func (b *stepBag) substituteStepPlaceholders(input string) string {
	if input == "" {
		return input
	}
	return stepPlaceholderRegex.ReplaceAllStringFunc(input, func(match string) string {
		groups := stepPlaceholderRegex.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		stepName := groups[1]
		dotPath := ""
		if len(groups) >= 3 {
			dotPath = groups[2]
		}

		value, found := b.get(stepName)
		if !found {
			return fmt.Sprintf("[missing: %s]", stepName)
		}
		if value == nil {
			return ""
		}

		if dotPath == "" {
			return stringifyForPrompt(value)
		}

		resolved, ok := walkDotPath(value, dotPath)
		if !ok {
			return fmt.Sprintf("[missing: %s.%s]", stepName, dotPath)
		}
		if resolved == nil {
			return ""
		}
		return stringifyForPrompt(resolved)
	})
}

// substituteInParams walks a params map (one context action's resolved
// query + body) and replaces step placeholders in every string value.
// Non-string values pass through unchanged. Used during context-action
// chaining when a later action's params reference an earlier action's
// result.
//
// Only the top level is walked — param values are rarely nested maps and
// when they are (e.g., JSON array body), the executor calls this on each
// string field the body_mapping wrote.
func (b *stepBag) substituteInParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for key, value := range params {
		if str, isString := value.(string); isString {
			out[key] = b.substituteStepPlaceholders(str)
			continue
		}
		out[key] = value
	}
	return out
}

// substituteInStringMap is the same as substituteInParams but for
// map[string]string (used for query params which are always string-typed).
func (b *stepBag) substituteInStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = b.substituteStepPlaceholders(value)
	}
	return out
}

// walkDotPath navigates a dot-separated path through a nested value.
// Returns (value, true) on success; (nil, false) if any intermediate
// segment is missing or isn't walkable (i.e., not a map).
//
// Array indexing with numeric keys is NOT supported — if you have a list
// and need to reach into a specific element, that indicates the template
// should use the whole step ({{$step}}) and let the LLM interpret the
// array, not pluck individual elements via dot-path. This keeps the
// template semantics simple and matches the dispatcher's own ref-lookup
// behavior.
func walkDotPath(value any, dotPath string) (any, bool) {
	if dotPath == "" {
		return value, true
	}
	segments := strings.Split(dotPath, ".")
	current := value
	for _, segment := range segments {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := obj[segment]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

// stringifyForPrompt converts a JSON-decoded value to a string form
// suitable for embedding in an LLM prompt:
//
//   - Scalars (string, bool, int, float) render via fmt.Sprintf
//   - Integers from JSON (float64 that round to int) render without
//     trailing decimals so issue_number 1347 doesn't become "1347.000000"
//   - Objects and arrays are JSON-marshaled with 2-space indent for
//     readability — the LLM reads indented JSON much more reliably than
//     compact JSON
//   - nil renders as empty string (caller handles this earlier but it's
//     defensive here too)
func stringifyForPrompt(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	case int, int32, int64:
		return fmt.Sprintf("%d", typed)
	case map[string]any, []any:
		pretty, err := json.MarshalIndent(typed, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(pretty)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
