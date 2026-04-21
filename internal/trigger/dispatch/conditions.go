package dispatch

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// evaluateConditions runs every condition against the payload and combines the
// results according to TriggerMatch.Mode ("all" = AND, "any" = OR).
//
// On a passing condition set, returns ("", true).
// On a failing condition set, returns a human-readable failure reason and false.
// The reason names the first failing condition (for "all") or the last evaluated
// condition (for "any") so debugging from logs is easy.
//
// nil match (no conditions) always passes.
func evaluateConditions(match *model.TriggerMatch, payload map[string]any) (reason string, passed bool) {
	if match == nil || len(match.Conditions) == 0 {
		return "", true
	}
	mode := match.Mode
	if mode == "" {
		mode = "all"
	}
	switch mode {
	case "all":
		for index, cond := range match.Conditions {
			if !evaluateCondition(cond, payload) {
				return fmt.Sprintf("condition %d (%s %s) failed", index, cond.Path, cond.Operator), false
			}
		}
		return "", true
	case "any":
		var lastReason string
		for index, cond := range match.Conditions {
			if evaluateCondition(cond, payload) {
				return "", true
			}
			lastReason = fmt.Sprintf("condition %d (%s %s) failed", index, cond.Path, cond.Operator)
		}
		return "no conditions matched (any mode); " + lastReason, false
	default:
		return fmt.Sprintf("invalid match mode %q", mode), false
	}
}

// evaluateCondition runs one condition against the payload. Path lookup is
// dot-notation (same as ref extraction). Operator semantics:
//
//   - equals / not_equals       : string comparison after stringifying both sides
//   - one_of / not_one_of       : value is in / not in a list of strings
//   - contains / not_contains   : substring (path value contains the literal)
//   - matches                   : regex match (path value matches the regex)
//   - exists / not_exists       : path resolves to a non-nil value
//
// All operators tolerate missing paths gracefully:
//   - exists returns false
//   - not_exists returns true
//   - everything else returns false (the condition fails)
func evaluateCondition(cond model.TriggerCondition, payload map[string]any) bool {
	rawValue, found := lookupPath(payload, cond.Path)

	switch cond.Operator {
	case "exists":
		return found && rawValue != nil
	case "not_exists":
		return !found || rawValue == nil
	}

	if !found {
		return false
	}
	pathValue := stringifyScalar(rawValue)

	switch cond.Operator {
	case "equals":
		return pathValue == toScalarString(cond.Value)
	case "not_equals":
		return pathValue != toScalarString(cond.Value)
	case "one_of":
		for _, item := range toStringSlice(cond.Value) {
			if pathValue == item {
				return true
			}
		}
		return false
	case "not_one_of":
		for _, item := range toStringSlice(cond.Value) {
			if pathValue == item {
				return false
			}
		}
		return true
	case "contains":
		return strings.Contains(pathValue, toScalarString(cond.Value))
	case "not_contains":
		return !strings.Contains(pathValue, toScalarString(cond.Value))
	case "matches":
		pattern, err := regexp.Compile(toScalarString(cond.Value))
		if err != nil {
			return false
		}
		return pattern.MatchString(pathValue)
	default:
		return false
	}
}

// toScalarString stringifies a condition.value for non-list operators. JSON
// decoding gives us interface{} so we have to handle string/number/bool here
// the same way refs do.
func toScalarString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
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
	}
	return stringifyScalar(value)
}

// toStringSlice converts a condition.value into []string for one_of/not_one_of.
// Accepts []string, []any (with mixed types), or a single scalar (treated as a
// one-element list — convenient for "in [single value]" patterns).
func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, toScalarString(item))
		}
		return out
	default:
		return []string{toScalarString(value)}
	}
}
