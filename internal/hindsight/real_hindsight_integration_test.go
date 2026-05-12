package hindsight

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRealHindsightRetainRecallOrgTeam(t *testing.T) {
	if os.Getenv("HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HINDSIGHT_INTEGRATION=1 and HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := NewClient(baseURL)
	orgID := uuid.New()
	teamID := uuid.New()
	bankID := OrgBankID(orgID)
	if err := client.ConfigureBank(ctx, bankID, DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("ConfigureBank: %v", err)
	}

	fact := "The Platform team requires rollback notes in every deployment plan. marker=" + uuid.NewString()
	_, err := client.Retain(ctx, bankID, &RetainRequest{
		Items: []RetainItem{{
			Content:    fact,
			Context:    "Integration test durable team policy",
			DocumentID: "integration-test:" + uuid.NewString(),
			Tags: []string{
				"company:" + orgID.String(),
				"team:" + teamID.String(),
				"source:manual",
				"visibility:team",
				"memory_type:policy",
			},
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Metadata:  map[string]string{"test": "real-hindsight"},
			ObservationScopes: [][]string{
				{"company:" + orgID.String()},
				{"company:" + orgID.String(), "team:" + teamID.String()},
			},
		}},
		Async: false,
	})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}

	resp, err := client.Recall(ctx, bankID, &RecallRequest{
		Query:  "What deployment policy does the Platform team follow?",
		Budget: "mid",
		TagGroups: []any{map[string]any{
			"tags":  []string{"company:" + orgID.String(), "team:" + teamID.String()},
			"match": "all_strict",
		}},
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatalf("expected at least one recalled memory")
	}
	if !strings.Contains(strings.ToLower(toJSONForTest(resp.Results)), "rollback") {
		t.Fatalf("recall results did not include retained policy: %#v", resp.Results)
	}
}

func toJSONForTest(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
