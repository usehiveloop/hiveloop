package tasks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/trigger/dispatch"
)

type compiledTriggerMessage struct {
	Text           string
	ResourceKey    string
	ConversationID string
	Raw            map[string]any
}

func (h *EmployeeTriggerDispatchHandler) compileMessage(payload EmployeeTriggerDispatchPayload, trigger model.AgentTrigger, webhookPayload map[string]any) compiledTriggerMessage {
	triggerType := trigger.TriggerType
	if triggerType == "" {
		triggerType = "webhook"
	}
	resourceKey := payload.DeliveryID
	refs := map[string]string{}
	summaryRefs := map[string]string{}
	displayName := triggerType
	event := eventKey(payload.EventType, payload.EventAction)
	if triggerType == "webhook" {
		if def, ok := lookupTriggerDef(h.catalog, payload.Provider, payload.EventType, payload.EventAction); ok {
			displayName = def.DisplayName
			refs, _ = dispatch.ExtractRefs(webhookPayload, def.Refs)
			summaryRefs, _ = dispatch.ExtractRefs(webhookPayload, def.SummaryRefs)
			if def.ResourceKeyTemplate != "" {
				if key, ok := substituteTemplate(def.ResourceKeyTemplate, refs); ok {
					resourceKey = key
				}
			}
		}
	} else {
		displayName = "HTTP trigger"
	}
	conversationID := stableTriggerConversationID(trigger.ID, resourceKey)

	var b strings.Builder
	b.WriteString("Trigger fired.\n\n")
	if strings.TrimSpace(trigger.Instructions) != "" {
		b.WriteString("Instructions:\n")
		b.WriteString(strings.TrimSpace(trigger.Instructions))
		b.WriteString("\n\n")
	}
	b.WriteString("Event:\n")
	writeKV(&b, "type", triggerType)
	writeKV(&b, "name", displayName)
	writeKV(&b, "provider", payload.Provider)
	writeKV(&b, "event_key", event)
	writeKV(&b, "delivery_id", payload.DeliveryID)
	writeKV(&b, "resource_key", resourceKey)

	if len(refs) > 0 {
		b.WriteString("\nRefs:\n")
		writeMap(&b, refs)
	}
	if len(summaryRefs) > 0 {
		b.WriteString("\nSummary:\n")
		writeMap(&b, summaryRefs)
	}
	if triggerType == "http" && len(webhookPayload) > 0 {
		b.WriteString("\nHTTP payload:\n")
		encoded, _ := json.MarshalIndent(webhookPayload, "", "  ")
		b.Write(encoded)
		b.WriteByte('\n')
	}

	return compiledTriggerMessage{
		Text:           b.String(),
		ResourceKey:    resourceKey,
		ConversationID: conversationID,
		Raw: map[string]any{
			"source":       "trigger",
			"trigger_id":   trigger.ID.String(),
			"trigger_type": triggerType,
			"provider":     payload.Provider,
			"event_key":    event,
			"delivery_id":  payload.DeliveryID,
			"resource_key": resourceKey,
			"refs":         refs,
			"summary_refs": summaryRefs,
			"received_at":  time.Now().UTC().Format(time.RFC3339),
		},
	}
}

func eventKey(eventType, eventAction string) string {
	if eventAction == "" {
		return eventType
	}
	return eventType + "." + eventAction
}

func triggerConditionsMatch(trigger model.AgentTrigger, payload map[string]any) (bool, string) {
	if len(trigger.Conditions) == 0 {
		return true, ""
	}
	var match model.TriggerMatch
	if err := json.Unmarshal(trigger.Conditions, &match); err != nil {
		return false, "invalid condition json"
	}
	reason, ok := dispatch.MatchConditions(&match, payload)
	return ok, reason
}

func lookupTriggerDef(cat *catalog.Catalog, provider, eventType, eventAction string) (*catalog.TriggerDef, bool) {
	key := eventKey(eventType, eventAction)
	if def, ok := cat.GetTrigger(provider, key); ok {
		return def, true
	}
	if pt, ok := cat.GetProviderTriggersForVariant(provider); ok {
		if def, exists := pt.Triggers[key]; exists {
			return &def, true
		}
		if eventAction != "" {
			if def, exists := pt.Triggers[eventType]; exists {
				return &def, true
			}
		}
	}
	return nil, false
}

func substituteTemplate(template string, refs map[string]string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(template); {
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

func stableTriggerConversationID(triggerID uuid.UUID, resourceKey string) string {
	sum := sha256.Sum256([]byte(triggerID.String() + ":" + resourceKey))
	return "trigger-" + hex.EncodeToString(sum[:])[:32]
}

func writeKV(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString("- ")
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

func writeMap(b *strings.Builder, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeKV(b, key, values[key])
	}
}
