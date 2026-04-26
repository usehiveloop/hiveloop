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

type schedulerFixture struct {
	DB   *gorm.DB
	Org  *model.Org
	Conn *model.InConnection
	Cfg  scheduler.Config
	Enq  *enqueueClient
	Insp *asynq.Inspector
}

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

func (f *schedulerFixture) makeSource(t *testing.T, opts ...sourceOpt) *ragmodel.RAGSource {
	t.Helper()
	// Each call gets a fresh InConnection so sources don't collide on
	// uq_rag_sources_in_connection.
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
	defaultRefresh := 60
	src.RefreshFreqSeconds = &defaultRefresh
	for _, o := range opts {
		o(src)
	}
	// Non-INTEGRATION kinds must have a null in_connection_id (CHECK).
	if src.KindValue != ragmodel.RAGSourceKindIntegration {
		src.InConnectionID = nil
	}
	wantEnabled := src.Enabled
	if err := f.DB.Create(src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	// gorm's "not null;default:true" skips Enabled=false from the
	// INSERT, so the column default wins; force-update post-insert.
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

func (f *schedulerFixture) queueDepth(t *testing.T) int {
	t.Helper()
	info, err := f.Insp.GetQueueInfo(ragtasks.QueueRagWork)
	if err != nil {
		return 0
	}
	return info.Pending + info.Active + info.Scheduled
}

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

func minutesAgo(n int) time.Time { return time.Now().Add(-time.Duration(n) * time.Minute) }

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

func ctxBg() context.Context { return context.Background() }
