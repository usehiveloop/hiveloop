package handler

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

func TestEnsureSlackKnowledgeSourceCreatesOneSourceAndEnqueuesIngest(t *testing.T) {
	db := connectSlackKnowledgeTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "Slack Knowledge Test"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "Aria", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := model.AgentProfile{
		ID:         uuid.New(),
		OrgID:      org.ID,
		AgentID:    agent.ID,
		Provider:   "slack",
		ExternalID: "T123",
		Label:      "Acme",
		Config:     model.JSON{},
		Status:     "active",
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}

	enq := &recordingEnqueuer{}
	h := &AgentProfileHandler{db: db, enq: enq}
	h.ensureSlackKnowledgeSource(context.Background(), agent, &profile)
	h.ensureSlackKnowledgeSource(context.Background(), agent, &profile)

	var sources []ragmodel.RAGSource
	if err := db.Where("org_id = ? AND kind = ?", org.ID, ragmodel.RAGSourceKindSlackBotProfile).Find(&sources).Error; err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected exactly one source, got %d", len(sources))
	}
	if sources[0].ConfigValue["agent_profile_id"] != profile.ID.String() {
		t.Fatalf("source config = %#v", sources[0].ConfigValue)
	}
	if sources[0].IndexingStart == nil {
		t.Fatalf("expected indexing_start to be set")
	}
	var reloaded model.AgentProfile
	if err := db.First(&reloaded, "id = ?", profile.ID).Error; err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	if reloaded.Config["knowledge_source_id"] != sources[0].ID.String() {
		t.Fatalf("profile config = %#v", reloaded.Config)
	}
	if len(enq.tasks) != 2 {
		t.Fatalf("expected one enqueue attempt per ensure call, got %d", len(enq.tasks))
	}
	payload, err := ragtasks.UnmarshalIngest(enq.tasks[0].Payload())
	if err != nil {
		t.Fatalf("decode task payload: %v", err)
	}
	if payload.RAGSourceID != sources[0].ID {
		t.Fatalf("task source id = %s, want %s", payload.RAGSourceID, sources[0].ID)
	}
}

type recordingEnqueuer struct {
	tasks []*asynq.Task
}

func (r *recordingEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.tasks = append(r.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (r *recordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return r.Enqueue(task)
}

func (r *recordingEnqueuer) Close() error { return nil }

func connectSlackKnowledgeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hiveloop:localdev@localhost:5433/hiveloop_test?sslmode=disable"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.AutoMigrate(&ragmodel.RAGSource{}); err != nil {
		t.Fatalf("rag automigrate: %v", err)
	}
	t.Cleanup(func() {
		db.Where("kind = ?", ragmodel.RAGSourceKindSlackBotProfile).Delete(&ragmodel.RAGSource{})
		db.Where("provider = ?", "slack").Delete(&model.AgentProfile{})
		db.Where("name = ?", "Aria").Delete(&model.Agent{})
		db.Where("name = ?", "Slack Knowledge Test").Delete(&model.Org{})
		sqlDB.Close()
	})
	return db
}
