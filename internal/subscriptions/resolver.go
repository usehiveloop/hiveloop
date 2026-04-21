package subscriptions

import (
	"log/slog"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

// ResolveEventResourceKey computes the canonical resource_key for an incoming
// webhook payload by:
//
//  1. Looking up the event's trigger definition in the catalog
//     (via GetTrigger or GetProviderTriggersForVariant for variants like
//     "github-app" that fall back to the base "github" catalog).
//  2. Extracting refs from the payload using the trigger def's Refs map.
//  3. Substituting {name} placeholders in the trigger def's ResourceKeyTemplate
//     with the extracted refs.
//
// Returns the canonical key (e.g. "github/hiveloop/hiveloop/pull/99") and
// true on success. Returns "" and false when:
//   - No trigger def exists for (provider, eventKey)
//   - The trigger has no resource_key_template
//   - One or more refs needed by the template are missing from the payload
//
// This is the bridge between inbound events and conversation_subscriptions:
// the key this function produces is the same shape an agent's
// subscribe_to_events tool call produces from an id_example, so lookups
// match.
//
// logger is required — every resolution step is logged (catalog lookup, refs
// extracted, missing refs, final key) because routing failures are the single
// most common reason a subscription "should have" matched but didn't, and we
// need full visibility in production to debug agent flows.
//
// No DB access — caller passes the payload map and the catalog.
func ResolveEventResourceKey(
	logger *slog.Logger,
	cat *catalog.Catalog,
	provider, eventType, eventAction string,
	payload map[string]any,
) (string, bool) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(
		"component", "subscriptions.resolver",
		"provider", provider,
		"event_type", eventType,
		"event_action", eventAction,
	)

	logger.Info("resolver: lookup starting")

	def, ok := lookupTriggerDef(cat, provider, eventType, eventAction)
	if !ok {
		logger.Info("resolver: no trigger def found in catalog, dropping",
			"event_key_tried_1", eventKey(eventType, eventAction),
			"event_key_tried_2", eventType,
		)
		return "", false
	}
	logger.Info("resolver: trigger def found",
		"display_name", def.DisplayName,
		"resource_type", def.ResourceType,
		"resource_key_template", def.ResourceKeyTemplate,
		"ref_count", len(def.Refs),
	)

	if def.ResourceKeyTemplate == "" {
		logger.Info("resolver: trigger def has no resource_key_template, dropping")
		return "", false
	}

	refs, missing := dispatch.ExtractRefs(payload, def.Refs)
	logger.Info("resolver: refs extracted",
		"refs_found", len(refs),
		"refs_missing", len(missing),
		"refs", refs,
		"missing", missing,
	)

	key, ok := substituteTemplate(def.ResourceKeyTemplate, refs)
	if !ok {
		logger.Warn("resolver: template substitution failed, dropping",
			"template", def.ResourceKeyTemplate,
			"available_refs", refs,
		)
		return "", false
	}

	logger.Info("resolver: resource_key resolved", "resource_key", key)
	return key, true
}

// ResolveEventSummaryRefs extracts the curated summary field values from the
// payload for an incoming webhook event, driven by the trigger's
// `summary_refs` catalog entry.
//
// Returns the trigger def (for display context), the resolved map of
// display_name → string_value, and true on catalog lookup success.
// A catalog miss returns (nil, nil, false). A hit with no summary_refs
// defined returns (def, nil, true) — callers should fall back to a minimal
// envelope in that case.
//
// Values that resolve to missing/null/empty are omitted from the returned
// map so the agent only sees fields that are actually present in the payload.
func ResolveEventSummaryRefs(
	cat *catalog.Catalog,
	provider, eventType, eventAction string,
	payload map[string]any,
) (*catalog.TriggerDef, map[string]string, bool) {
	def, ok := lookupTriggerDef(cat, provider, eventType, eventAction)
	if !ok {
		return nil, nil, false
	}
	if len(def.SummaryRefs) == 0 {
		return def, nil, true
	}
	refs, _ := dispatch.ExtractRefs(payload, def.SummaryRefs)
	return def, refs, true
}

// eventKey returns the "{eventType}.{eventAction}" composite or bare eventType
// when the action is empty. Matches the catalog's trigger-map keying.
func eventKey(eventType, eventAction string) string {
	if eventAction == "" {
		return eventType
	}
	return eventType + "." + eventAction
}

// lookupTriggerDef resolves a trigger definition, honoring variant fallback
// so events from "github-app" find templates declared in "github.triggers.json".
// eventKey is "{eventType}.{eventAction}" when action is set, bare eventType
// otherwise — the catalog keys its triggers the same way (e.g. "issues.opened",
// "push"). For events where action is empty but the catalog stores
// "issues" as the sole key, we try the bare eventType as a second attempt.
func lookupTriggerDef(cat *catalog.Catalog, provider, eventType, eventAction string) (*catalog.TriggerDef, bool) {
	key := eventKey(eventType, eventAction)

	if def, ok := cat.GetTrigger(provider, key); ok {
		return def, true
	}

	if pt, ok := cat.GetProviderTriggersForVariant(provider); ok {
		if def, exists := pt.Triggers[key]; exists {
			return &def, true
		}
		// Fall through to bare eventType (for actionless events like push).
		if eventAction != "" {
			if def, exists := pt.Triggers[eventType]; exists {
				return &def, true
			}
		}
	}

	return nil, false
}

// substituteTemplate replaces {name} placeholders in template with the
// corresponding ref value. Returns false if any placeholder can't be
// resolved — callers should drop the event rather than deliver a malformed
// key (better to log than to write an unroutable subscription match).
func substituteTemplate(template string, refs map[string]string) (string, bool) {
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
			out.WriteString(template[i:])
			break
		}
		close += open

		out.WriteString(template[i:open])
		name := template[open+1 : close]
		value, ok := refs[name]
		if !ok || value == "" {
			return "", false
		}
		out.WriteString(value)
		i = close + 1
	}

	return out.String(), true
}
