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

// taskFixture bundles per-test infrastructure for the per-source
// handler tests: a real DB, a real rag-engine binary, and a freshly
// inserted org / in-connection / source / dataset.
type taskFixture struct {
	DB     *gorm.DB
	Org    *model.Org
	Conn   *model.InConnection
	Engine *testhelpers.RagEngineInstance
	Deps   *ragtasks.Deps
}

// nextStubKind returns a unique kind string per call because the
// global connector registry rejects duplicate Register calls within
// the test binary lifetime.
func nextStubKind() string {
	return "stub-" + uuid.New().String()[:8]
}

// setupTask spins up the full per-source handler stack:
// real Postgres + real rag-engine + a configured Deps bundle.
// The dataset is created server-side so IngestBatch / UpdateACL /
// Prune calls succeed.
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

	// Make sure the embedding-model registry is migrated. Already
	// done by ConnectTestDB through rag.AutoMigrate, but we depend
	// on it directly so a missing migration fails loudly.
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

// datasetName returns the canonical fake-embedder dataset.
func datasetName() string { return "rag_chunks__fake__2560" }

// makeSource inserts a RAGSource with the given kind, returning the row.
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
	// Per the integration<->in_connection check constraint, only
	// INTEGRATION-kind rows may carry an in_connection_id. Our stub
	// kinds (e.g. "stub-…") aren't INTEGRATION at the SQL level, so
	// we drop the FK to make the insert legal.
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

// runIngestNow synthesises an asynq.Task for TypeRagIngest and invokes
// the handler synchronously. Returns the handler's error, if any.
func (f *taskFixture) runIngestNow(ctx context.Context, t *testing.T, sourceID uuid.UUID) error {
	t.Helper()
	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{RAGSourceID: sourceID})
	if err != nil {
		t.Fatalf("build ingest task: %v", err)
	}
	return f.Deps.HandleIngest(ctx, task)
}

// reloadAttempt fetches the most recent rag_index_attempts row for
// the given source, ordered by time_created DESC.
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

// reloadSource fetches the rag_sources row by ID.
func reloadSource(t *testing.T, db *gorm.DB, id uuid.UUID) *ragmodel.RAGSource {
	t.Helper()
	var s ragmodel.RAGSource
	if err := db.First(&s, "id = ?", id).Error; err != nil {
		t.Fatalf("reload source: %v", err)
	}
	return &s
}
