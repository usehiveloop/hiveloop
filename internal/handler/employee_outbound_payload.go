package handler

import (
	"regexp"
	"strings"
)

var obviousSecretPattern = regexp.MustCompile(`(?i)(ptok_|xox[baprs]-|sk-[a-z0-9]|api[_-]?key|secret|token|password)\s*[:=]\s*\S+`)

func stringValueDefault(payload map[string]any, key, fallback string) string {
	if value := stringValue(payload, key); value != "" {
		return value
	}
	return fallback
}

func employeeEventSource(payload map[string]any) string {
	source := sanitizeTagValue(stringValue(payload, "source"))
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "gateway"))
	}
	if source == "" {
		source = sanitizeTagValue(stringValue(payload, "platform"))
	}
	if source == "" {
		return "manual"
	}
	return source
}

func sanitizeTagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
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
	return strings.Trim(b.String(), "-_")
}

func payloadLooksSensitive(payload map[string]any) bool {
	for _, key := range []string{"text", "result_summary", "message", "error"} {
		if obviousSecretPattern.MatchString(stringValue(payload, key)) {
			return true
		}
	}
	return false
}

func stringValue(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
