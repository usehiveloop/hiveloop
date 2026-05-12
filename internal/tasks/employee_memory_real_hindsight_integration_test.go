package tasks

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/hindsight"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestRealHindsightEmployeeMemoryCheckpointFlow(t *testing.T) {
	if os.Getenv("HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HINDSIGHT_INTEGRATION=1 and HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	teamID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sessionID := "slack-C123-" + uuid.NewString()
	marker := "marker-" + uuid.NewString()

	if err := db.Create(&model.Org{ID: orgID, Name: "real-memory-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := db.Create(&model.Team{ID: teamID, OrgID: orgID, Name: "Platform"}).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{ID: agentID, OrgID: &orgID, TeamID: &teamID, Name: "Aria", IsEmployee: true, Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, AgentID: &agentID, ExternalID: "real-hindsight-sb", BridgeURL: "http://bridge", EncryptedBridgeAPIKey: []byte("x"), Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	seededEvents := []model.EmployeeMemoryEvent{
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "user.message.received", map[string]any{
			"source": "slack", "channel": "C123", "thread_ts": "1770000000.000001", "user_display_name": "Kim",
			"text": "For every production deploy, the Platform team must include rollback notes before merging. " + marker,
		}),
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "tool.invoked", map[string]any{
			"source": "slack", "tool": "read_file", "result_summary": "Confirmed the deploy checklist currently lacks rollback-note enforcement.",
		}),
		memoryEvent(t, orgID, agentID, sandboxID, sessionID, "agent.message.sent", map[string]any{
			"source": "slack", "text": "Done. I will treat rollback notes as required before production deploys.",
		}),
	}
	for _, event := range seededEvents {
		if err := db.Create(&event).Error; err != nil {
			t.Fatalf("create memory event: %v", err)
		}
	}

	itemPreview, ok := buildEmployeeRetainItem(&agent, EmployeeMemoryRetainPayload{
		AgentID: agentID, SandboxID: sandboxID, SessionID: sessionID, SourceEvent: "agent.message.sent",
	}, seededEvents)
	if !ok {
		t.Fatal("expected retain item preview")
	}
	t.Logf("retained content:\n%s", itemPreview.Content)
	t.Logf("retained tags: %v", itemPreview.Tags)
	t.Logf("retained metadata: %s", mustJSONForLog(itemPreview.Metadata))

	client := hindsight.NewClient(baseURL)
	if err := client.ConfigureBank(ctx, hindsight.OrgBankID(orgID), hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("configure bank: %v", err)
	}
	handler := NewEmployeeMemoryRetainHandler(db, client, nil)
	task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{
		AgentID:     agentID,
		SandboxID:   sandboxID,
		SessionID:   sessionID,
		Reason:      "real_hindsight_test",
		SourceEvent: "agent.message.sent",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := handler.Handle(ctx, task); err != nil {
		t.Fatalf("retain handler: %v", err)
	}

	var retainedCount int64
	if err := db.Model(&model.EmployeeMemoryEvent{}).
		Where("agent_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NOT NULL", agentID, sandboxID, sessionID).
		Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained events: %v", err)
	}
	if retainedCount != int64(len(seededEvents)) {
		t.Fatalf("retained count = %d, want %d", retainedCount, len(seededEvents))
	}

	resp := waitForEmployeeMemoryRecall(t, ctx, client, orgID, teamID, marker)
	recallJSON := mustJSONForLog(resp.Results)
	t.Logf("recall returned data:\n%s", recallJSON)
	if len(resp.Results) == 0 {
		t.Fatalf("expected recall results")
	}
	lowerRecall := strings.ToLower(recallJSON)
	if !strings.Contains(lowerRecall, "rollback") ||
		!strings.Contains(lowerRecall, "production deploy") ||
		!strings.Contains(lowerRecall, strings.ToLower("employee-session:"+sandboxID.String()+":"+sessionID)) {
		t.Fatalf("recall did not include retained deployment policy and source document; results=%s", recallJSON)
	}
}

func waitForEmployeeMemoryRecall(t *testing.T, ctx context.Context, client *hindsight.Client, orgID, teamID uuid.UUID, marker string) *hindsight.RecallResponse {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	var last *hindsight.RecallResponse
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Recall(ctx, hindsight.OrgBankID(orgID), &hindsight.RecallRequest{
			Query:  "What does the Platform team require before production deploys? " + marker,
			Budget: "mid",
			TagGroups: []any{map[string]any{
				"tags":  []string{"company:" + orgID.String(), "team:" + teamID.String()},
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

func mustJSONForLog(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
