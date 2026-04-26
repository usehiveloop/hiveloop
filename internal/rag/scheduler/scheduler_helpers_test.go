package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/scheduler"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// schedulerFixture bundles the per-test infrastructure: a real DB
// connection, a real Asynq client, and a real Inspector. The org and
// in-connection rows are set up so the source can FK-resolve.
type schedulerFixture struct {
	DB    *gorm.DB
	Org   *model.Org
	Conn  *model.InConnection
	Cfg   scheduler.Config
	Enq   *enqueueClient
	Insp  *asynq.Inspector
}

// enqueueClient is a thin wrapper used by tests so we can both feed
// the scheduler an enqueue.TaskEnqueuer and reach into the underlying
// asynq Inspector for assertions.
type enqueueClient struct {
	*asynq.Client
}

func (c *enqueueClient) Enqueue(t *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return c.Client.Enqueue(t, opts...)
}
func (c *enqueueClient) Close() error { return c.Client.Close() }

func setupScheduler(t *testing.T) *schedulerFixture {
	t.Helper()
	db := testhelpers.ConnectTestDB(t)
	testhelpers.ConnectTestRedis(t)

	org := testhelpers.NewTestOrg(t, db)
	user := testhelpers.NewTestUser(t, db, org.ID)
	integ := testhelpers.NewTestInIntegration(t, db, "scheduler-test")
	conn := testhelpers.NewTestInConnection(t, db, org.ID, user.ID, integ.ID)

	cli := asynq.NewClient(testhelpers.AsynqRedisOpt())
	t.Cleanup(func() { _ = cli.Close() })
	insp := testhelpers.NewTestAsynqInspector(t)

	return &schedulerFixture{
		DB:   db,
		Org:  org,
		Conn: conn,
		Cfg: scheduler.Config{
			IngestTick:      15 * time.Second,
			PermSyncTick:    30 * time.Second,
			PruneTick:       60 * time.Second,
			WatchdogTick:    60 * time.Second,
			WatchdogTimeout: 30 * time.Minute,
			UniqueSlack:     30 * time.Second,
			EnqueueLimit:    100,
		},
		Enq:  &enqueueClient{Client: cli},
		Insp: insp,
	}
}

// makeSource inserts a RAGSource with the supplied overrides applied.
// Returns the row so tests can refer back to its ID. Cleanup is hooked
// to the org cleanup via the cascade.
func (f *schedulerFixture) makeSource(t *testing.T, opts ...sourceOpt) *ragmodel.RAGSource {
	t.Helper()
	// Each call gets a fresh InConnection so sources don't collide on
	// the partial unique index uq_rag_sources_in_connection.
	conn := freshInConnection(t, f.DB, f.Org.ID, f.Conn.UserID, f.Conn.InIntegrationID)
	connID := conn.ID
	src := &ragmodel.RAGSource{
		OrgIDValue:     f.Org.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "src-" + uuid.New().String()[:8],
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		InConnectionID: &connID,
		AccessType:     ragmodel.AccessTypePrivate,
	}
	// Default: refresh every 60s; null disables ingest. Tests override
	// where needed.
	defaultRefresh := 60
	src.RefreshFreqSeconds = &defaultRefresh
	for _, o := range opts {
		o(src)
	}
	// Enforce the integration<->in_connection check constraint: a
	// non-INTEGRATION kind must have a null in_connection_id.
	if src.KindValue != ragmodel.RAGSourceKindIntegration {
		src.InConnectionID = nil
	}
	wantEnabled := src.Enabled
	if err := f.DB.Create(src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	// gorm's "not null;default:true" causes Enabled=false to be skipped
	// from the INSERT, so the column default wins. Force the desired
	// value with an explicit UPDATE post-insert.
	if !wantEnabled {
		if err := f.DB.Model(&ragmodel.RAGSource{}).
			Where("id = ?", src.ID).
			Update("enabled", false).Error; err != nil {
			t.Fatalf("force-disable source: %v", err)
		}
		src.Enabled = false
	}
	t.Cleanup(func() {
		f.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
	})
	return src
}

type sourceOpt func(*ragmodel.RAGSource)

func withRefresh(secs *int) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.RefreshFreqSeconds = secs }
}
func withLastIndex(t time.Time) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.LastSuccessfulIndexTime = &t }
}
func withStatus(st ragmodel.RAGSourceStatus) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.Status = st }
}
func withEnabled(b bool) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.Enabled = b }
}
func withKind(k string) sourceOpt {
	// The model column is a typed RAGSourceKind enum. For tests where
	// we want to associate a stub-connector kind, we override the
	// in-DB string directly via a hook below.
	return func(s *ragmodel.RAGSource) { s.KindValue = ragmodel.RAGSourceKind(k) }
}
func withAccessType(a ragmodel.AccessType) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.AccessType = a }
}
func withPermSyncFreq(secs *int) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.PermSyncFreqSeconds = secs }
}
func withLastPermSync(t time.Time) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.LastTimePermSync = &t }
}
func withPruneFreq(secs *int) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.PruneFreqSeconds = secs }
}
func withLastPruned(t time.Time) sourceOpt {
	return func(s *ragmodel.RAGSource) { s.LastPruned = &t }
}

// queueDepth returns Pending + Active for the rag work queue. Asynq's
// Unique blocks on Pending only, so a duplicate scan tick that lands
// on a still-Pending task is what these tests exercise.
func (f *schedulerFixture) queueDepth(t *testing.T) int {
	t.Helper()
	info, err := f.Insp.GetQueueInfo(ragtasks.QueueRagWork)
	if err != nil {
		// Empty queue is reported as not-found; treat as 0.
		return 0
	}
	return info.Pending + info.Active + info.Scheduled
}

// pendingTaskTypes returns the type strings of every pending task in
// the rag work queue.
func (f *schedulerFixture) pendingTaskTypes(t *testing.T) []string {
	t.Helper()
	tasks, err := f.Insp.ListPendingTasks(ragtasks.QueueRagWork)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(tasks))
	for _, ti := range tasks {
		out = append(out, ti.Type)
	}
	return out
}

// minutesAgo is a tiny helper for "now - N minutes".
func minutesAgo(n int) time.Time { return time.Now().Add(-time.Duration(n) * time.Minute) }

// freshInConnection creates a new InConnection for the given org/user/integration
// so each makeSource call gets a unique target for the partial unique
// index uq_rag_sources_in_connection.
func freshInConnection(t *testing.T, db *gorm.DB, orgID, userID, integID uuid.UUID) *model.InConnection {
	t.Helper()
	suffix := uuid.New().String()[:8]
	conn := &model.InConnection{
		OrgID:             orgID,
		UserID:            userID,
		InIntegrationID:   integID,
		NangoConnectionID: "nango-fake-" + suffix,
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatalf("create in_connection: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", conn.ID).Delete(&model.InConnection{}) })
	return conn
}

// ctxBg is a one-liner for tests that don't care about cancellation.
func ctxBg() context.Context { return context.Background() }
