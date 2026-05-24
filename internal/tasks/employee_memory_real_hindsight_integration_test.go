package tasks

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func TestRealHindsightEmployeeMemoryCheckpointFlow(t *testing.T) {
	if os.Getenv("HIVY_HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HIVY_HINDSIGHT_INTEGRATION=1 and HIVY_HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HIVY_HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	sessionID := "slack-C123-" + uuid.NewString()
	marker := "marker-" + uuid.NewString()

	if err := db.Create(&model.Org{ID: orgID, Name: "real-memory-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "real-hindsight-sb", BridgeURL: "http://bridge", EncryptedBridgeAPIKey: []byte("x"), Status: "running"}).Error; err != nil {
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
		EmployeeID: agentID, SandboxID: sandboxID, SessionID: sessionID, SourceEvent: "agent.message.sent",
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
		EmployeeID:  agentID,
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
		Where("employee_id = ? AND sandbox_id = ? AND session_id = ? AND retained_at IS NOT NULL", agentID, sandboxID, sessionID).
		Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained events: %v", err)
	}
	if retainedCount != int64(len(seededEvents)) {
		t.Fatalf("retained count = %d, want %d", retainedCount, len(seededEvents))
	}

	resp := waitForEmployeeMemoryRecall(t, ctx, client, orgID, marker)
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

func TestRealHindsightEmployeeMemoryProductionWorkload(t *testing.T) {
	if os.Getenv("HIVY_HINDSIGHT_INTEGRATION") != "1" {
		t.Skip("set HIVY_HINDSIGHT_INTEGRATION=1 and HIVY_HINDSIGHT_API_URL to run against a real Hindsight service")
	}
	baseURL := os.Getenv("HIVY_HINDSIGHT_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db := openTasksMemoryTestDB(t)
	orgID := uuid.New()
	agentID := uuid.New()
	sandboxID := uuid.New()
	if err := db.Create(&model.Org{ID: orgID, Name: "prod-memory-" + uuid.NewString()[:8], Active: true}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Employee{ID: agentID, OrgID: &orgID, Name: "Aria", IsEmployee: true, Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Sandbox{ID: sandboxID, OrgID: &orgID, EmployeeID: &agentID, ExternalID: "production-workload-sb", BridgeURL: "http://bridge", EncryptedBridgeAPIKey: []byte("x"), Status: "running"}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	sessions := productionMemorySessions()
	var inserted int
	var expectedRetained int
	for i, session := range sessions {
		sessionID := "slack-C123-prod-" + uuid.NewString()
		events := session.toEvents(t, orgID, agentID, sandboxID, sessionID, i)
		if _, ok := buildEmployeeRetainItem(&agent, EmployeeMemoryRetainPayload{
			EmployeeID: agentID, SandboxID: sandboxID, SessionID: sessionID, SourceEvent: "agent.message.sent",
		}, events); ok {
			expectedRetained += len(events)
		}
		for _, event := range events {
			if err := db.Create(&event).Error; err != nil {
				t.Fatalf("create event: %v", err)
			}
			inserted++
		}
	}
	if inserted < 100 {
		t.Fatalf("seeded %d events, want at least 100", inserted)
	}

	client := hindsight.NewClient(baseURL)
	if err := client.ConfigureBank(ctx, hindsight.OrgBankID(orgID), hindsight.DefaultMemoryConfig().ToBankConfigUpdate()); err != nil {
		t.Fatalf("configure bank: %v", err)
	}
	handler := NewEmployeeMemoryRetainHandler(db, client, nil)
	for _, sessionID := range distinctProductionSessionIDs(t, db, agentID, sandboxID) {
		task, err := NewEmployeeMemoryRetainTask(EmployeeMemoryRetainPayload{
			EmployeeID:  agentID,
			SandboxID:   sandboxID,
			SessionID:   sessionID,
			Reason:      "production_workload_test",
			SourceEvent: "agent.message.sent",
		})
		if err != nil {
			t.Fatalf("task: %v", err)
		}
		if err := handler.Handle(ctx, task); err != nil {
			t.Fatalf("retain session %s: %v", sessionID, err)
		}
	}
	var retainedCount int64
	if err := db.Model(&model.EmployeeMemoryEvent{}).
		Where("employee_id = ? AND sandbox_id = ? AND retained_at IS NOT NULL", agentID, sandboxID).
		Count(&retainedCount).Error; err != nil {
		t.Fatalf("count retained: %v", err)
	}
	if retainedCount != int64(expectedRetained) {
		t.Fatalf("retained %d events, want %d", retainedCount, expectedRetained)
	}
	if expectedRetained >= inserted {
		t.Fatalf("expected some non-work events to remain unretained; inserted=%d expected_retained=%d", inserted, expectedRetained)
	}

	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "deployment-policy",
			query: "What does the Platform team require before production deploys?",
			want:  []string{"rollback", "production deploy"},
		},
		{
			name:  "testing-policy",
			query: "What testing rules or quality gates does the Platform team follow?",
			want:  []string{"integration", "test"},
		},
		{
			name:  "communication-feedback",
			query: "What feedback has the Platform team given Aria about communication style?",
			want:  []string{"short", "direct"},
		},
		{
			name:  "ownership",
			query: "Who owns billing incidents or invoice failures?",
			want:  []string{"nora", "billing"},
		},
		{
			name:  "technical-context",
			query: "What durable technical context should Aria remember about the backend?",
			want:  []string{"postgres", "idempotency"},
		},
	}
	for _, tc := range cases {
		resp := waitForProductionRecall(t, ctx, client, orgID, tc.query, tc.want)
		t.Logf("production recall %s:\n%s", tc.name, mustJSONForLog(resp.Results))
	}

	for _, query := range []string{
		"What jokes did teammates tell Aria?",
		"What lunch, coffee, playlist, or meme chatter should Aria remember?",
	} {
		resp, err := client.Recall(ctx, hindsight.OrgBankID(orgID), &hindsight.RecallRequest{
			Query:  query,
			Budget: "high",
			TagGroups: []any{map[string]any{
				"tags":  []string{"company:" + orgID.String()},
				"match": "all_strict",
			}},
		})
		if err != nil {
			t.Fatalf("non-work recall %q: %v", query, err)
		}
		resultJSON := strings.ToLower(mustJSONForLog(resp.Results))
		t.Logf("non-work recall %q:\n%s", query, mustJSONForLog(resp.Results))
		for _, forbidden := range []string{"sandwich", "playlist", "meme", "rubber duck", "pirate", "coffee"} {
			if strings.Contains(resultJSON, forbidden) {
				t.Fatalf("non-work chatter leaked into memory for query %q: %s", query, resultJSON)
			}
		}
	}
}
