// Package subscriptions implements the subscribe_to_events flow: catalog lookup,
// provider-integration enforcement, resource_id parsing, canonical-key
// construction, and the idempotent upsert into conversation_subscriptions.
//
// The package is intentionally independent of the MCP server so it can be
// unit-tested without spinning up the MCP runtime. The MCP tool registration
// lives in internal/mcpserver/ and delegates to SubscribeToEvents.
package subscriptions

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
)

// compiledPatternCache memoizes compiled regexes keyed by the raw pattern
// string so we don't recompile on every call. The catalog is immutable at
// runtime (loaded once from embedded files), so the cache never invalidates.
var (
	compiledPatternCache   = make(map[string]*regexp.Regexp)
	compiledPatternCacheMu sync.Mutex
)

func compilePattern(pattern string) (*regexp.Regexp, error) {
	compiledPatternCacheMu.Lock()
	defer compiledPatternCacheMu.Unlock()
	if re, ok := compiledPatternCache[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	compiledPatternCache[pattern] = re
	return re, nil
}

// ParseResult is the outcome of parsing a user-supplied resource_id against a
// subscribable resource definition. CanonicalKey is what we write into
// conversation_subscriptions.ResourceKey; Parts is the captured group map
// (owner, repo, number, etc.) in case callers want to persist it separately.
type ParseResult struct {
	CanonicalKey string
	Parts        map[string]string
}

// ParseResourceID validates resourceID against the resource's IDPattern and
// substitutes the captured groups into CanonicalTemplate. The returned error
// is designed to be surfaced directly to the agent — it names the expected
// format and an example so she can self-correct.
func ParseResourceID(def catalog.SubscribableResource, resourceID string) (*ParseResult, error) {
	if resourceID == "" {
		return nil, fmt.Errorf("resource_id is required (expected format like %q)", def.IDExample)
	}

	re, err := compilePattern(def.IDPattern)
	if err != nil {
		// This is a catalog authoring bug, not a user error, but we return it
		// rather than panic because the catalog is loaded from JSON — a
		// malformed pattern shouldn't crash the whole server.
		return nil, fmt.Errorf("catalog: invalid id_pattern for resource: %w", err)
	}

	match := re.FindStringSubmatch(resourceID)
	if match == nil {
		return nil, fmt.Errorf(
			"resource_id %q does not match the expected format for this resource_type (expected format like %q)",
			resourceID, def.IDExample,
		)
	}

	parts := make(map[string]string, len(re.SubexpNames()))
	for index, name := range re.SubexpNames() {
		if index == 0 || name == "" {
			continue
		}
		parts[name] = match[index]
	}

	canonical, err := substituteCanonical(def.CanonicalTemplate, parts)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		CanonicalKey: canonical,
		Parts:        parts,
	}, nil
}

// substituteCanonical replaces {name} placeholders in template with the
// corresponding value from parts. A missing placeholder is a catalog bug
// (the regex must define every name referenced in the template), so we
// return an error identifying the offender.
func substituteCanonical(template string, parts map[string]string) (string, error) {
	var out strings.Builder
	out.Grow(len(template))

	i := 0
	for i < len(template) {
		open := strings.IndexByte(template[i:], '{')
		if open < 0 {
			out.WriteString(template[i:])
			break
		}
		open += i
		close := strings.IndexByte(template[open:], '}')
		if close < 0 {
			// Unclosed brace — treat the rest literally.
			out.WriteString(template[i:])
			break
		}
		close += open

		out.WriteString(template[i:open])
		name := template[open+1 : close]

		value, ok := parts[name]
		if !ok {
			return "", fmt.Errorf(
				"catalog: canonical_template references {%s} but id_pattern captures no such named group",
				name,
			)
		}
		out.WriteString(value)
		i = close + 1
	}

	return out.String(), nil
}
