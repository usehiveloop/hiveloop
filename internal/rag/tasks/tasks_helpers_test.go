package tasks_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	embedder "github.com/usehiveloop/hiveloop/internal/rag/embedder"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/ragpb"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

type taskFixture struct {
	DB     *gorm.DB
	Org    *model.Org
	Conn   *model.InConnection
	Engine *testhelpers.RagEngineInstance
	Deps   *ragtasks.Deps
}

// nextStubKind returns a unique kind because the global connector
// registry rejects duplicate Register calls within a test binary.
func nextStubKind() string {
	return "stub-" + uuid.New().String()[:8]
}

func setupTask(t *testing.T) *taskFixture {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "tasks-test")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	inst := testhelpers.StartRagEngineInTestMode(t, testhelpers.RagEngineConfig{})
	const dim = uint32(2560)
	dataset := datasetName()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := inst.Client.CreateDataset(ctx, &ragpb.CreateDatasetRequest{
		DatasetName:        dataset,
		VectorDim:          dim,
		EmbeddingPrecision: "float32",
		IdempotencyKey:     "tasks-" + t.Name(),
	}); err != nil {
		t.Fatalf("create dataset: %v", err)
	}

	deps := &ragtasks.Deps{
		DB:                db,
		RagClient:         inst.Client,
		HeartbeatTick:     150 * time.Millisecond,
		BatchSize:         5,
		DatasetName:       dataset,
		DeclaredVectorDim: dim,
	}

	if err := embedder.SeedRegistry(db); err != nil {
		t.Fatalf("seed embedding-model registry: %v", err)
	}

	return &taskFixture{
		DB:     db,
		Org:    org,
		Conn:   conn,
		Engine: inst,
		Deps:   deps,
	}
}

func datasetName() string { return "rag_chunks__fake__2560" }

func (f *taskFixture) makeSource(t *testing.T, kind string) *ragmodel.RAGSource {
	t.Helper()
	connID := f.Conn.ID
	src := &ragmodel.RAGSource{
		OrgIDValue:     f.Org.ID,
		KindValue:      ragmodel.RAGSourceKind(kind),
		Name:           "src-" + uuid.New().String()[:8],
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
	}
	// Non-INTEGRATION kinds must have a null in_connection_id (CHECK).
	if !ragmodel.RAGSourceKind(kind).IsValid() ||
		ragmodel.RAGSourceKind(kind) != ragmodel.RAGSourceKindIntegration {
		src.InConnectionID = nil
	}
	if err := f.DB.Create(src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	t.Cleanup(func() {
		f.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})
	return src
}

func (f *taskFixture) runIngestNow(ctx context.Context, t *testing.T, sourceID uuid.UUID) error {
	t.Helper()
	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: sourceID})
	if err != nil {
		t.Fatalf("build ingest task: %v", err)
	}
	return f.Deps.HandleIngest(ctx, task)
}

func reloadAttempt(t *testing.T, db *gorm.DB, sourceID uuid.UUID) *ragmodel.RAGIndexAttempt {
	t.Helper()
	var a ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ?", sourceID).
		Order("time_created DESC").
		First(&a).Error; err != nil {
		t.Fatalf("reload attempt: %v", err)
	}
	return &a
}

func reloadSource(t *testing.T, db *gorm.DB, id uuid.UUID) *ragmodel.RAGSource {
	t.Helper()
	var s ragmodel.RAGSource
	if err := db.First(&s, "id = ?", id).Error; err != nil {
		t.Fatalf("reload source: %v", err)
	}
	return &s
}
