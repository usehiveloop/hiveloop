package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func buildEmployeeRetainItem(agent *model.Employee, payload EmployeeMemoryRetainPayload, events []model.EmployeeSessionEvent) (hindsight.RetainItem, bool) {
	if agent == nil || agent.OrgID == nil || len(events) == 0 {
		return hindsight.RetainItem{}, false
	}
	if employeeSessionEventsContainSecret(events) {
		return hindsight.RetainItem{}, false
	}
	if !employeeSessionEventsHaveWorkSignal(events) {
		return hindsight.RetainItem{}, false
	}
	digest := employeeMemoryRetentionDigest(agent.Name, events)
	if !meaningfulEmployeeMemoryTranscript(digest, events) {
		return hindsight.RetainItem{}, false
	}
	source := dominantEmployeeMemorySource(events)
	tags := employeeMemoryTags(agent, source)
	channel := firstEmployeePayloadString(events, "channel")
	if channel != "" {
		tags = append(tags, "channel:"+sanitizeMemoryTagValue(channel))
	}
	observationScopes := [][]string{{"company:" + agent.OrgID.String()}}
	return hindsight.RetainItem{
		Content:           digest,
		Context:           fmt.Sprintf("Filtered employee memory digest from %s source. It intentionally omits routine tool use and transient task chatter. Retain durable people facts, including teammate names and stable channel user IDs or mention handles when present, plus company facts, decisions, preferences, ownership, policies, recurring workflows, and reusable technical context. Do not retain active conversation framing or temporary task status as durable facts.", source),
		DocumentID:        "employee-session:" + payload.SandboxID.String() + ":" + payload.SessionID,
		Tags:              tags,
		Timestamp:         events[0].EventAt.UTC().Format(time.RFC3339),
		Metadata:          employeeMemoryRetainMetadata(agent, payload, events),
		ObservationScopes: observationScopes,
	}, true
}

func employeeMemoryRetentionDigest(agentName string, events []model.EmployeeSessionEvent) string {
	var lines []string
	for _, event := range events {
		payload := employeeMemoryPayload(event)
		switch event.EventType {
		case "user.message.received":
			speaker := employeeMemorySpeaker(payload)
			if speaker == "" {
				speaker = "teammate"
			}
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeEmployeeMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Teammate %s: %s", speaker, text))
			}
		case "agent.message.sent":
			text := firstPayloadString(payload, "text", "message")
			if shouldIncludeEmployeeMemoryMessage(text) {
				lines = append(lines, fmt.Sprintf("Employee %s: %s", agentName, text))
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	var buf strings.Builder
	buf.WriteString("Durable memory extraction input. This omits raw tool calls, internal commands, and execution trace.\n")
	buf.WriteString("Retain durable people facts, including teammate names, stable channel user IDs or mention handles, roles, ownership, preferences, decisions, policies, recurring workflows, business/customer/team/project facts, and stable technical/company context.\n")
	buf.WriteString("Do not retain active-conversation framing as facts: who is currently talking to the employee, who asked in this thread, temporary task progress, status chatter, one-off execution state, or ordinary completion messages.\n")
	buf.WriteString("Use speaker names and channel IDs as attribution context and as durable people identity only when they identify real teammates, roles, ownership, or preferences.\n\n")
	for _, line := range lines {
		buf.WriteString("- ")
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	return strings.TrimSpace(buf.String())
}

func employeeMemorySpeaker(payload map[string]any) string {
	name := firstPayloadString(payload, "user_display_name")
	userID := firstPayloadString(payload, "user")
	mention := employeeMemorySlackMention(userID)
	switch {
	case name != "" && mention != "":
		return fmt.Sprintf("%s (%s)", name, mention)
	case name != "":
		return name
	case mention != "":
		return mention
	default:
		return userID
	}
}

func employeeMemorySlackMention(userID string) string {
	userID = strings.TrimSpace(userID)
	if strings.HasPrefix(userID, "U") || strings.HasPrefix(userID, "W") {
		return "<@" + userID + ">"
	}
	return ""
}

func employeeMemoryRetainMetadata(agent *model.Employee, payload EmployeeMemoryRetainPayload, events []model.EmployeeSessionEvent) map[string]string {
	meta := map[string]string{
		"employee_id":  agent.ID.String(),
		"sandbox_id":   payload.SandboxID.String(),
		"session_id":   payload.SessionID,
		"event_count":  fmt.Sprintf("%d", len(events)),
		"source_event": payload.SourceEvent,
	}
	for _, key := range []string{"source", "channel", "thread_ts", "user", "user_display_name", "tool"} {
		if value := firstEmployeePayloadString(events, key); value != "" {
			meta[key] = value
		}
	}
	return meta
}

func firstEmployeePayloadString(events []model.EmployeeSessionEvent, key string) string {
	for _, event := range events {
		if value := firstPayloadString(employeeMemoryPayload(event), key); value != "" {
			return value
		}
	}
	return ""
}

func firstPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func employeeSessionEventIDs(events []model.EmployeeSessionEvent) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}
