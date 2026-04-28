package handler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/system"
)

type parsedDoneFrame struct {
	Done   bool         `json:"done"`
	Usage  system.Usage `json:"usage"`
	Cached bool         `json:"cached"`
}

// parseHiveloopSSE walks the SSE body emitted by the system-task handler
// and extracts the per-chunk deltas plus the final done frame.
func parseHiveloopSSE(t *testing.T, body string) ([]string, parsedDoneFrame) {
	t.Helper()
	var deltas []string
	var done parsedDoneFrame
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var d struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &d); err == nil && d.Delta != "" {
			deltas = append(deltas, d.Delta)
			continue
		}
		_ = json.Unmarshal([]byte(payload), &done)
	}
	return deltas, done
}

// escapeJSON escapes inner JSON for embedding in another JSON string field.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
