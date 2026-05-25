package tasks

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

var memorySecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

func employeeSessionEventsContainSecret(events []model.EmployeeSessionEvent) bool {
	for _, event := range events {
		payload := employeeMemoryPayload(event)
		for _, key := range []string{"text", "message", "result_summary"} {
			if value := firstPayloadString(payload, key); value != "" && memorySecretPattern.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func shouldIncludeEmployeeMemoryMessage(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" || memorySecretPattern.MatchString(text) {
		return false
	}
	return true
}

func meaningfulEmployeeMemoryTranscript(transcript string, events []model.EmployeeSessionEvent) bool {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" || memorySecretPattern.MatchString(transcript) {
		return false
	}
	hasUser := false
	hasCheckpoint := false
	for _, event := range events {
		if event.EventType == "user.message.received" {
			hasUser = true
		}
		if event.EventType == "agent.message.sent" || event.EventType == "session.completed" {
			hasCheckpoint = true
		}
	}
	return hasUser && hasCheckpoint
}

func employeeMemoryTags(agent *model.Employee, source string) []string {
	tags := []string{
		"company:" + agent.OrgID.String(),
		"source:" + sanitizeMemoryTagValue(source),
		"visibility:company",
		"memory_type:company_context",
	}
	return tags
}

func dominantEmployeeMemorySource(events []model.EmployeeSessionEvent) string {
	counts := map[string]int{}
	for _, event := range events {
		source := strings.TrimSpace(event.Source)
		if source == "" {
			source = "manual"
		}
		counts[source]++
	}
	type pair struct {
		source string
		count  int
	}
	pairs := make([]pair, 0, len(counts))
	for source, count := range counts {
		pairs = append(pairs, pair{source: source, count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].source < pairs[j].source
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) == 0 {
		return "manual"
	}
	return pairs[0].source
}

func employeeMemoryPayload(event model.EmployeeSessionEvent) map[string]any {
	var payload map[string]any
	if len(event.Payload) > 0 {
		_ = json.Unmarshal(event.Payload, &payload)
	}
	if payload == nil {
		return map[string]any{}
	}
	return payload
}

func sanitizeMemoryTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "manual"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '/':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return "manual"
	}
	return out
}
