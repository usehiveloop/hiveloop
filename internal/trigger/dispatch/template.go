package dispatch

import (
	"regexp"
	"strings"
)

// Template syntax recognized by the dispatcher:
//
//   $refs.x          — bare ref reference (used in plain text and YAML scalars)
//   {{$refs.x}}      — moustache ref reference (mustache style for instructions)
//   {{$step.field}}  — context-action result reference (DEFERRED to executor)
//
// Dispatch-time substitution resolves both ref forms ($refs.x and {{$refs.x}})
// using the resolved refs map. {{$step.x}} placeholders are left intact for the
// executor and recorded in DeferredVars so we can detect dangling references.
//
// The two ref forms exist because users write context-action paths in YAML
// scalars (where bare $refs.x reads naturally) and instruction prompts in
// markdown (where {{$refs.x}} reads more like a typical template).

// stepRefRegex matches {{$step_name.field}} (deferred — left in place).
// step_name must start with a letter and contain word chars; field is a dot-path.
var stepRefRegex = regexp.MustCompile(`\{\{\s*\$([A-Za-z][\w]*)(?:\.([\w.]+))?\s*\}\}`)

// mustacheRefRegex matches {{$refs.x}} or {{ $refs.x }} (with optional whitespace).
var mustacheRefRegex = regexp.MustCompile(`\{\{\s*\$refs\.([\w]+)\s*\}\}`)

// bareRefRegex matches $refs.x not inside {{ ... }}. Used as a second pass
// after mustache substitution. The trailing lookahead avoids matching the
// inside of {{$refs.x}} which has already been handled.
var bareRefRegex = regexp.MustCompile(`\$refs\.([\w]+)`)

// substituteRefs replaces every $refs.x and {{$refs.x}} occurrence in the input
// with the corresponding value from the refs map. References to keys not in
// the map are left in place (so log/debug output can show what's missing).
//
// {{$step.field}} placeholders are NOT touched here — they survive for the
// executor.
func substituteRefs(input string, refs map[string]string) string {
	if input == "" || len(refs) == 0 {
		return input
	}
	output := mustacheRefRegex.ReplaceAllStringFunc(input, func(match string) string {
		matches := mustacheRefRegex.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		if value, ok := refs[matches[1]]; ok {
			return value
		}
		return match
	})
	output = bareRefRegex.ReplaceAllStringFunc(output, func(match string) string {
		matches := bareRefRegex.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		if value, ok := refs[matches[1]]; ok {
			return value
		}
		return match
	})
	return output
}

// findStepReferences returns the unique set of step names that appear as
// {{$step_name.field}} placeholders in the input. The dispatcher uses this to
// populate ContextRequest.DeferredVars and to validate that referenced steps
// exist as earlier ContextActions in the trigger config.
//
// "refs" is recognized as a step name by the regex but excluded — it's already
// substituted by substituteRefs.
func findStepReferences(input string) []string {
	if input == "" {
		return nil
	}
	matches := stepRefRegex.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		stepName := match[1]
		if stepName == "refs" {
			continue
		}
		if seen[stepName] {
			continue
		}
		seen[stepName] = true
		out = append(out, stepName)
	}
	return out
}

// substitutePathParams fills {param} placeholders in an action's path template
// from a values map. This is a separate, much simpler substitution from the
// ref/step machinery — paths like /repos/{owner}/{repo}/issues/{issue_number}
// use single-brace placeholders that come straight from the catalog.
func substitutePathParams(pathTemplate string, values map[string]string) string {
	if pathTemplate == "" || !strings.Contains(pathTemplate, "{") {
		return pathTemplate
	}
	result := pathTemplate
	for key, value := range values {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}
