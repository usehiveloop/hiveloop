package dispatch

import (
	"regexp"
)

// Template syntax recognized by the dispatcher:
//
//   $refs.x          — bare ref reference (used in plain text and YAML scalars)
//   {{$refs.x}}      — moustache ref reference (mustache style for instructions)
//
// Dispatch-time substitution resolves both ref forms ($refs.x and {{$refs.x}})
// using the resolved refs map.
//
// The two ref forms exist because users write context-action paths in YAML
// scalars (where bare $refs.x reads naturally) and instruction prompts in
// markdown (where {{$refs.x}} reads more like a typical template).

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

// SubstituteRefs is the exported alias for substituteRefs — callers outside
// this package (executor, task handlers) need to apply $refs.x substitution
// to trigger instruction templates before building conversation messages.
func SubstituteRefs(input string, refs map[string]string) string {
	return substituteRefs(input, refs)
}
