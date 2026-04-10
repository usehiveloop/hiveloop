package dispatch

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// Test 12: Real-Postgres GORM store test.
//
// This is the "no copping out" guardrail. The 11 in-memory dispatcher tests
// validate the logic, but they only catch logic bugs — not bugs in the actual
// SQL query. The PostgreSQL `&&` array overlap operator and pq.StringArray
// driver behavior need to round-trip through a real DB to be trustworthy.
//
// What this test catches that in-memory tests can't:
//   - typos in the WHERE clause column names
//   - misuse of pq.StringArray vs []string in the gorm WHERE arg
//   - wrong array operator (e.g. @> instead of &&)
//   - schema drift between the model and the production migration
//
// Skipped automatically when no Postgres is reachable. Tests run when
// DATABASE_URL is set, mirroring the convention in internal/handler/.
func TestGormAgentTriggerStore_FindMatching(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Postgres not reachable, skipping real-DB test: %v", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("Postgres not reachable: %v", err)
	}

	// AutoMigrate gives us the production schema (including the Instructions
	// column we added). This is the same migration the server runs at boot.
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Use unique IDs per test run so this test can be re-run without cleanup.
	orgID := uuid.New()
	connectionID := uuid.New()
	otherConnectionID := uuid.New()
	agentID := uuid.New()
	otherAgentID := uuid.New()

	// Seed two agents — one we expect to match, one we expect to filter out.
	defer cleanupAgentTriggers(t, db, orgID)
	defer cleanupAgents(t, db, orgID)

	if err := db.Create(&model.Agent{
		ID:           agentID,
		OrgID:        &orgID,
		Name:         "match-agent",
		SandboxType:  "shared",
		SystemPrompt: "test",
		Model:        "claude-opus-4-6",
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := db.Create(&model.Agent{
		ID:           otherAgentID,
		OrgID:        &orgID,
		Name:         "non-match-agent",
		SandboxType:  "shared",
		SystemPrompt: "test",
		Model:        "claude-opus-4-6",
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Trigger 1: matches our event (issues.opened) on the right connection — SHOULD return.
	wantTriggerID := uuid.New()
	mustCreateTrigger(t, db, model.AgentTrigger{
		ID:           wantTriggerID,
		OrgID:        orgID,
		AgentID:      agentID,
		ConnectionID: connectionID,
		TriggerKeys:  pq.StringArray{"issues.opened", "issues.closed"},
		Enabled:      true,
		Instructions: "match me",
	})

	// Trigger 2: same connection, same key, but DISABLED — should be excluded.
	mustCreateTrigger(t, db, model.AgentTrigger{
		ID:           uuid.New(),
		OrgID:        orgID,
		AgentID:      otherAgentID,
		ConnectionID: connectionID,
		TriggerKeys:  pq.StringArray{"issues.opened"},
		Enabled:      false,
	})

	// Trigger 3: enabled, listens for the right key, but DIFFERENT connection — should be excluded.
	mustCreateTrigger(t, db, model.AgentTrigger{
		ID:           uuid.New(),
		OrgID:        orgID,
		AgentID:      otherAgentID,
		ConnectionID: otherConnectionID,
		TriggerKeys:  pq.StringArray{"issues.opened"},
		Enabled:      true,
	})

	// Trigger 4: enabled, same connection, but listens for a DIFFERENT key only.
	mustCreateTrigger(t, db, model.AgentTrigger{
		ID:           uuid.New(),
		OrgID:        orgID,
		AgentID:      otherAgentID,
		ConnectionID: connectionID,
		TriggerKeys:  pq.StringArray{"pull_request.opened"},
		Enabled:      true,
	})

	store := NewGormAgentTriggerStore(db)
	results, err := store.FindMatching(context.Background(), orgID, connectionID, []string{"issues.opened"})
	if err != nil {
		t.Fatalf("FindMatching: %v", err)
	}

	if len(results) != 1 {
		ids := make([]string, 0, len(results))
		for _, result := range results {
			ids = append(ids, result.Trigger.ID.String())
		}
		t.Fatalf("expected 1 matching trigger, got %d (ids: %v)", len(results), ids)
	}
	if results[0].Trigger.ID != wantTriggerID {
		t.Errorf("returned trigger ID = %v, want %v", results[0].Trigger.ID, wantTriggerID)
	}
	if results[0].Trigger.Instructions != "match me" {
		t.Errorf("trigger Instructions = %q, want 'match me'", results[0].Trigger.Instructions)
	}
	if results[0].Agent.ID != agentID {
		t.Errorf("loaded agent ID = %v, want %v", results[0].Agent.ID, agentID)
	}

	// Multi-key array overlap: querying with a key that matches the SECOND
	// element of trigger 1's TriggerKeys should still find it. This validates
	// the && (overlap) operator vs the @> (contains) operator.
	results, err = store.FindMatching(context.Background(), orgID, connectionID, []string{"issues.closed"})
	if err != nil {
		t.Fatalf("FindMatching for issues.closed: %v", err)
	}
	if len(results) != 1 || results[0].Trigger.ID != wantTriggerID {
		t.Errorf("multi-key array overlap broken: got %d results", len(results))
	}

	// No-match query: a key that no trigger listens for should return nothing.
	results, err = store.FindMatching(context.Background(), orgID, connectionID, []string{"deployment_status.created"})
	if err != nil {
		t.Fatalf("FindMatching no-match: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unmatched key, got %d", len(results))
	}
}

// mustCreateTrigger inserts a trigger and bails the test on failure.
// We use raw Create + explicit Update for Enabled because GORM treats bool
// false as a zero value and skips it in Create — same workaround the handler
// uses at internal/handler/agent_triggers.go:301.
func mustCreateTrigger(t *testing.T, db *gorm.DB, trigger model.AgentTrigger) {
	t.Helper()
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger %s: %v", trigger.ID, err)
	}
	if !trigger.Enabled {
		if err := db.Model(&model.AgentTrigger{}).Where("id = ?", trigger.ID).Update("enabled", false).Error; err != nil {
			t.Fatalf("update trigger enabled=false: %v", err)
		}
	}
}

func cleanupAgentTriggers(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.AgentTrigger{})
}

func cleanupAgents(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Unscoped().Where("org_id = ?", orgID).Delete(&model.Agent{})
}
