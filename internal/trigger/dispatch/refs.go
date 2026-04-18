package dispatch

import (
	"fmt"
	"strconv"
	"strings"
)

// extractRefs walks the catalog TriggerDef.Refs map (ref_name → dot-path) and
// resolves each path against the webhook payload. Returns a map of refs as
// strings (everything is stringified for substitution into URLs/templates).
//
// Each entry's value may be either a single dot-path (e.g. `"event.channel"`)
// or a fallback list separated by `||` (e.g. `"event.thread_ts || event.ts"`).
// Fallback lists resolve to the first path that returns a non-nil, non-empty
// value — this is how the catalog coalesces across sibling fields that exist
// only in certain event variants (e.g. Slack's `thread_ts` which is present
// only on thread replies, falling back to `ts` for top-level messages so both
// produce the same thread identifier).
//
// Missing or non-scalar paths are reported in missing []string for logging.
// The dispatcher does NOT fail on missing refs — they're left out of the map
// and any template using them will surface the issue downstream.
func extractRefs(payload map[string]any, defs map[string]string) (refs map[string]string, missing []string) {
	refs = make(map[string]string, len(defs))
	for refName, rawPath := range defs {
		value, ok := resolveRefPath(payload, rawPath)
		if !ok {
			missing = append(missing, refName+"="+rawPath)
			continue
		}
		refs[refName] = stringifyScalar(value)
	}
	return refs, missing
}

// resolveRefPath tries each path in a fallback-list expression against the
// payload and returns the first one that resolves to a non-nil, non-empty
// value. Single-path expressions (no `||`) behave exactly as lookupPath.
//
// "Empty" means nil, the empty string, or missing — all three are treated as
// "try the next fallback" because the coalescing use case is "use field A when
// it's present, otherwise B." A zero number or false boolean is NOT empty;
// those resolve successfully (the coalescing semantics are about presence,
// not truthiness).
func resolveRefPath(payload map[string]any, rawPath string) (any, bool) {
	paths := splitFallbackPaths(rawPath)
	for _, path := range paths {
		// Literal support: "'some string'" resolves to the literal "some string".
		// Use case: last-resort fallback in a `||` chain, e.g.
		// `issue.pull_request.url || 'fallback'`.
		if literal, ok := parseLiteral(path); ok {
			return literal, true
		}

		// Conditional literal: "?path:if_present:if_absent" — resolves to the
		// literal `if_present` when `path` exists (non-nil, non-empty),
		// otherwise `if_absent`. This is how the catalog produces a
		// discriminator like "pull" vs "issue" for shared event types (e.g.
		// issue_comment.created fires on both Issues and PRs — the payload's
		// `issue.pull_request` presence tells us which).
		if cond, ok := parseConditionalLiteral(path); ok {
			if _, present := lookupPath(payload, cond.path); present {
				if cond.ifPresent != "" {
					return cond.ifPresent, true
				}
				continue
			}
			if cond.ifAbsent != "" {
				return cond.ifAbsent, true
			}
			continue
		}

		value, found := lookupPath(payload, path)
		if !found {
			continue
		}
		if value == nil {
			continue
		}
		// Treat empty strings as "not present" for coalescing purposes. The
		// common case is a field that exists in the payload envelope but is
		// blank for this particular event variant — the catalog author almost
		// certainly wants to fall through to the next option.
		if str, isString := value.(string); isString && str == "" {
			continue
		}
		return value, true
	}
	return nil, false
}

// parseLiteral recognizes single-quoted literals like "'pull'" and returns
// the unquoted body. Quotes must balance; content may not itself contain a
// single quote. Returns (value, true) on success; (_, false) means the
// argument is not a literal expression and should be treated as a path.
func parseLiteral(expr string) (string, bool) {
	if len(expr) < 2 {
		return "", false
	}
	if expr[0] != '\'' || expr[len(expr)-1] != '\'' {
		return "", false
	}
	body := expr[1 : len(expr)-1]
	if strings.Contains(body, "'") {
		return "", false
	}
	return body, true
}

// conditionalLiteral is the parsed form of "?path:if_present:if_absent".
type conditionalLiteral struct {
	path      string
	ifPresent string
	ifAbsent  string
}

// parseConditionalLiteral recognizes the "?path:if_present:if_absent" form.
// All three segments are required; missing colons or segments return !ok.
// The literals are NOT single-quoted (to keep the syntax compact) and may
// not themselves contain ':'.
func parseConditionalLiteral(expr string) (conditionalLiteral, bool) {
	if len(expr) < 2 || expr[0] != '?' {
		return conditionalLiteral{}, false
	}
	rest := expr[1:]
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return conditionalLiteral{}, false
	}
	path := strings.TrimSpace(parts[0])
	if path == "" {
		return conditionalLiteral{}, false
	}
	return conditionalLiteral{
		path:      path,
		ifPresent: strings.TrimSpace(parts[1]),
		ifAbsent:  strings.TrimSpace(parts[2]),
	}, true
}

// splitFallbackPaths parses a fallback-list expression like
// "event.thread_ts || event.ts" into the list of individual dot-paths.
// Whitespace around `||` and around each path is stripped. Paths that are
// empty after trimming are dropped. Single-path expressions (no `||`) return
// a one-element slice. All-empty results return nil (not an empty slice) so
// callers can rely on `if len(paths) == 0` consistently.
func splitFallbackPaths(rawPath string) []string {
	if !strings.Contains(rawPath, "||") {
		trimmed := strings.TrimSpace(rawPath)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
	parts := strings.Split(rawPath, "||")
	var out []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// lookupPath walks a dot-separated path through nested maps. Returns (value, true)
// on success or (nil, false) if any segment is missing, out of range, or
// traverses a non-container.
//
// Edge cases handled explicitly because real GitHub payloads have them:
//   - "issue.number" — number lives in a nested object → recurse
//   - "ref" — top-level scalar → single segment
//   - "issue.pull_request" — may be missing entirely (issue events vs PR events)
//   - "pull_requests.0.number" — numeric segments index into arrays. GitHub
//     check_run/check_suite/workflow_run payloads expose `pull_requests[]` as
//     the canonical commit→PR link, so refs need to reach into slot 0 to pull
//     the PR number out for resource-key resolution.
func lookupPath(payload map[string]any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	var current any = payload
	for _, segment := range segments {
		switch container := current.(type) {
		case map[string]any:
			next, exists := container[segment]
			if !exists {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(container) {
				return nil, false
			}
			current = container[index]
		default:
			return nil, false
		}
	}
	return current, true
}

// ExtractRefs is the exported alias for extractRefs — callers outside this
// package (subscription dispatch, future webhook routers) need the same
// catalog-ref extraction logic and there's no reason to duplicate it.
func ExtractRefs(payload map[string]any, defs map[string]string) (refs map[string]string, missing []string) {
	return extractRefs(payload, defs)
}

// stringifyScalar converts a JSON-decoded value to its string form for templates
// and URLs. JSON numbers come back from encoding/json as float64; we render
// integers without the trailing ".0" so /repos/foo/bar/issues/1347 looks right
// instead of /repos/foo/bar/issues/1347.000000.
func stringifyScalar(value any) string {
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
		// JSON numbers are always float64. Render integers cleanly.
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
