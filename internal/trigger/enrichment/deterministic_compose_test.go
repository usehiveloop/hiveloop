package enrichment

import (
	"fmt"
	"testing"
)

func TestComposeEnrichedMessage_AllSuccessful(t *testing.T) {
	input := DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs: map[string]string{
			"service_name": "web.hiveloop.com",
			"branch":       "main",
		},
	}
	results := []enrichmentResult{
		{As: "build_logs", Action: "build_logs", Data: map[string]any{"data": []any{"line1", "line2"}}},
		{As: "service_details", Action: "service", Data: map[string]any{"name": "web", "status": "FAILED"}},
	}

	msg := composeEnrichedMessage(input, results)

	assertContains(t, msg, "## Deployment.failed", "event header")
	assertContains(t, msg, "service_name", "refs key")
	assertContains(t, msg, "web.hiveloop.com", "refs value")
	assertContains(t, msg, "### build_logs", "build_logs section")
	assertContains(t, msg, "### service_details", "service_details section")
	assertContains(t, msg, "```json", "JSON code block")
}

func TestComposeEnrichedMessage_PartialFailure(t *testing.T) {
	input := DeterministicEnrichInput{
		Provider:  "railway",
		EventType: "Deployment.failed",
		Refs:      map[string]string{"branch": "main"},
	}
	results := []enrichmentResult{
		{As: "build_logs", Action: "build_logs", Data: map[string]any{"log": "ok"}},
		{As: "runtime_logs", Action: "deployment_logs", Err: fmt.Errorf("nango proxy: 404 not found")},
	}

	msg := composeEnrichedMessage(input, results)

	assertContains(t, msg, "### build_logs", "successful section")
	assertContains(t, msg, "### runtime_logs", "failed section header")
	assertContains(t, msg, "> **Error:**", "error annotation")
	assertContains(t, msg, "404 not found", "error detail")
}
