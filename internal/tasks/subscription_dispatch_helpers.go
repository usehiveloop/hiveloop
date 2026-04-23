package tasks

import (
	"fmt"
	"sort"
	"strings"
)

func partitionSummaryFields(summaryRefs map[string]string) (inline, block []string) {
	for name, value := range summaryRefs {
		if isInlineSummaryValue(value) {
			inline = append(inline, name)
		} else {
			block = append(block, name)
		}
	}
	sort.Strings(inline)
	sort.Strings(block)
	return inline, block
}

func isInlineSummaryValue(value string) bool {
	return len(value) <= summaryInlineMaxBytes && !strings.ContainsRune(value, '\n')
}

func fenceFor(value string) string {
	fence := "```"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

const payloadPathsMaxDepth = 6

func renderPayloadPaths(payload map[string]any) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	paths := map[string]string{}
	walkPayloadPaths(payload, "", 0, paths)
	return paths
}

func walkPayloadPaths(value any, prefix string, depth int, out map[string]string) {
	if depth > payloadPathsMaxDepth {
		out[prefix] = "…(truncated, max depth)"
		return
	}
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			out[prefix] = "object{}"
			return
		}
		for key, child := range v {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			walkPayloadPaths(child, next, depth+1, out)
		}
	case []any:
		if len(v) == 0 {
			out[prefix] = "array[0]"
			return
		}
		if obj, ok := v[0].(map[string]any); ok {
			out[prefix] = fmt.Sprintf("array[%d] of object", len(v))
			for key, child := range obj {
				walkPayloadPaths(child, prefix+"[*]."+key, depth+1, out)
			}
			return
		}
		out[prefix] = fmt.Sprintf("array[%d] of %s", len(v), jsonScalarType(v[0]))
	case string:
		if len(v) > 100 {
			out[prefix] = fmt.Sprintf("string (%d bytes)", len(v))
		} else {
			out[prefix] = "string"
		}
	case float64:
		out[prefix] = "number"
	case bool:
		out[prefix] = "bool"
	case nil:
		out[prefix] = "null"
	default:
		out[prefix] = "?"
	}
}

func jsonScalarType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "?"
	}
}

func truncateFieldValue(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "…(truncated)"
}

func topLevelKeys(payload map[string]any) []string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	return keys
}

func previewString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("…(+%d bytes)", len(s)-limit)
}
