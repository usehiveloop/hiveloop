package tasks_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

const testDBURL = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"

func connectDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

func createAgent(t *testing.T, db *gorm.DB, deleted bool) model.Agent {
	t.Helper()
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "cleanup-test-" + uuid.New().String()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	agent := model.Agent{
		OrgID:        &orgID,
		Name:         "cleanup-agent-" + uuid.New().String()[:8],
		SystemPrompt: "test",
		Model:        "test-model",
		Status:       "active",
	}
	if deleted {
		now := time.Now()
		agent.DeletedAt = &now
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	t.Cleanup(func() {
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
		db.Where("id = ?", orgID).Delete(&model.Org{})
	})

	return agent
}

func makeTask(t *testing.T, agentID uuid.UUID) *asynq.Task {
	t.Helper()
	task, err := tasks.NewAgentCleanupTask(agentID)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func TestAgentCleanup_HardDeletesSoftDeletedAgent(t *testing.T) {
	db := connectDB(t)
	agent := createAgent(t, db, true)

	handler := tasks.NewAgentCleanupHandler(db, nil, nil)
	task := makeTask(t, agent.ID)

	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var count int64
	db.Model(&model.Agent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Fatal("agent should be hard-deleted from DB")
	}
}

func TestAgentCleanup_HardDeletesActiveAgent(t *testing.T) {
	db := connectDB(t)
	agent := createAgent(t, db, false)

	handler := tasks.NewAgentCleanupHandler(db, nil, nil)
	task := makeTask(t, agent.ID)

	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle: %v", err)
	}

	var count int64
	db.Model(&model.Agent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Fatal("agent should be hard-deleted from DB")
	}
}

func TestAgentCleanup_AlreadyDeletedIsIdempotent(t *testing.T) {
	db := connectDB(t)

	handler := tasks.NewAgentCleanupHandler(db, nil, nil)
	task := makeTask(t, uuid.New())

	if err := handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle should not error for missing agent: %v", err)
	}
}

func TestAgentCleanup_NilOrchestratorHandledGracefully(t *testing.T) {
	db := connectDB(t)

	agent := createAgent(t, db, true)
	handler := tasks.NewAgentCleanupHandler(db, nil, nil)

	if err := handler.Handle(context.Background(), makeTask(t, agent.ID)); err != nil {
		t.Fatalf("cleanup with nil orchestrator should not error: %v", err)
	}

	var count int64
	db.Model(&model.Agent{}).Where("id = ?", agent.ID).Count(&count)
	if count != 0 {
		t.Fatal("agent should still be hard-deleted even without orchestrator")
	}
}

func TestAgentCleanup_PayloadRoundTrip(t *testing.T) {
	agentID := uuid.New()
	task, err := tasks.NewAgentCleanupTask(agentID)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if task.Type() != tasks.TypeAgentCleanup {
		t.Fatalf("expected type %q, got %q", tasks.TypeAgentCleanup, task.Type())
	}

	var payload tasks.AgentCleanupPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.AgentID != agentID {
		t.Fatalf("expected agent ID %s, got %s", agentID, payload.AgentID)
	}
}
