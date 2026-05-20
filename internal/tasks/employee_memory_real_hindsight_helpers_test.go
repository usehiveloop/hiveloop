package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
)

func waitForEmployeeMemoryRecall(t *testing.T, ctx context.Context, client *hindsight.Client, orgID uuid.UUID, marker string) *hindsight.RecallResponse {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last *hindsight.RecallResponse
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Recall(ctx, hindsight.OrgBankID(orgID), &hindsight.RecallRequest{
			Query:  "What does the Platform team require before production deploys? " + marker,
			Budget: "mid",
			TagGroups: []any{map[string]any{
				"tags":  []string{"company:" + orgID.String()},
				"match": "all_strict",
			}},
		})
		if err != nil {
			lastErr = err
		} else {
			last = resp
			resultJSON := strings.ToLower(mustJSONForLog(resp.Results))
			if strings.Contains(resultJSON, "rollback") && strings.Contains(resultJSON, "production deploy") {
				return resp
			}
		}
		time.Sleep(3 * time.Second)
	}
	if lastErr != nil {
		t.Fatalf("recall never succeeded: %v", lastErr)
	}
	if last == nil {
		t.Fatalf("recall never returned a response")
	}
	return last
}

func waitForProductionRecall(t *testing.T, ctx context.Context, client *hindsight.Client, orgID uuid.UUID, query string, want []string) *hindsight.RecallResponse {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	var last *hindsight.RecallResponse
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Recall(ctx, hindsight.OrgBankID(orgID), &hindsight.RecallRequest{
			Query:  query,
			Budget: "high",
			TagGroups: []any{map[string]any{
				"tags":  []string{"company:" + orgID.String()},
				"match": "all_strict",
			}},
		})
		if err != nil {
			lastErr = err
		} else {
			last = resp
			resultJSON := strings.ToLower(mustJSONForLog(resp.Results))
			if containsAll(resultJSON, want) {
				return resp
			}
		}
		time.Sleep(4 * time.Second)
	}
	if lastErr != nil {
		t.Fatalf("recall %q never succeeded: %v", query, lastErr)
	}
	if last == nil {
		t.Fatalf("recall %q never returned", query)
	}
	t.Fatalf("recall %q missing %v in %s", query, want, mustJSONForLog(last.Results))
	return last
}

func containsAll(value string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(value, strings.ToLower(word)) {
			return false
		}
	}
	return true
}

func mustJSONForLog(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
