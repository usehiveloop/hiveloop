package db_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	gormpkg "gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

type queryFixture struct {
	DB    *gormpkg.DB
	Org   *model.Org
	User  *model.User
	Integ *model.InIntegration
}

func newQueryFixture(t *testing.T) *queryFixture {
	t.Helper()
	d := testhelpers.ConnectTestDB(t)
	org := testhelpers.NewTestOrg(t, d)
	user := testhelpers.NewTestUser(t, d, org.ID)
	integ := testhelpers.NewTestInIntegration(t, d, "queries-test")
	return &queryFixture{DB: d, Org: org, User: user, Integ: integ}
}

func (f *queryFixture) addSource(t *testing.T, status ragmodel.RAGSourceStatus, kind ragmodel.RAGSourceKind) *ragmodel.RAGSource {
	t.Helper()
	src := &ragmodel.RAGSource{
		OrgIDValue: f.Org.ID,
		KindValue:  kind,
		Name:       "src-" + uuid.New().String()[:8],
		Status:     status,
		Enabled:    true,
		AccessType: ragmodel.AccessTypePrivate,
	}
	if kind == ragmodel.RAGSourceKindIntegration {
		conn := testhelpers.NewTestInConnection(t, f.DB, f.Org.ID, f.User.ID, f.Integ.ID)
		src.InConnectionID = &conn.ID
	}
	if err := f.DB.Create(src).Error; err != nil {
		t.Fatalf("create rag source: %v", err)
	}
	t.Cleanup(func() { f.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	return src
}

func TestListSourcesForOrg_PaginationAndFilters(t *testing.T) {
	f := newQueryFixture(t)

	for i := 0; i < 7; i++ {
		f.addSource(t, ragmodel.RAGSourceStatusActive, ragmodel.RAGSourceKindIntegration)
	}
	for i := 0; i < 3; i++ {
		f.addSource(t, ragmodel.RAGSourceStatusPaused, ragmodel.RAGSourceKindIntegration)
	}

	rows, total, err := db.ListSourcesForOrg(f.DB, f.Org.ID, db.ListOptions{Page: 0, PageSize: 4})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 10 {
		t.Fatalf("expected total=10, got %d", total)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	paused := ragmodel.RAGSourceStatusPaused
	rows, total, err = db.ListSourcesForOrg(f.DB, f.Org.ID, db.ListOptions{StatusFilter: &paused})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected paused total=3, got %d", total)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestGetSourceForOrg_CrossOrgIsolation(t *testing.T) {
	f := newQueryFixture(t)
	src := f.addSource(t, ragmodel.RAGSourceStatusActive, ragmodel.RAGSourceKindIntegration)

	got, err := db.GetSourceForOrg(f.DB, f.Org.ID, src.ID)
	if err != nil {
		t.Fatalf("get same-org: %v", err)
	}
	if got.ID != src.ID {
		t.Fatalf("got wrong row")
	}

	otherOrg := testhelpers.NewTestOrg(t, f.DB)
	if _, err := db.GetSourceForOrg(f.DB, otherOrg.ID, src.ID); err != gormpkg.ErrRecordNotFound {
		t.Fatalf("expected ErrRecordNotFound from cross-org get, got %v", err)
	}
}

func TestListAttemptsForSource_Pagination(t *testing.T) {
	f := newQueryFixture(t)
	src := f.addSource(t, ragmodel.RAGSourceStatusActive, ragmodel.RAGSourceKindIntegration)

	base := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 12; i++ {
		a := &ragmodel.RAGIndexAttempt{
			OrgID:       f.Org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusSuccess,
			TimeCreated: base.Add(time.Duration(i) * time.Minute),
		}
		if err := f.DB.Create(a).Error; err != nil {
			t.Fatalf("create attempt: %v", err)
		}
	}
	t.Cleanup(func() {
		f.DB.Where("rag_source_id = ?", src.ID).Delete(&ragmodel.RAGIndexAttempt{})
	})

	rows, total, err := db.ListAttemptsForSource(f.DB, f.Org.ID, src.ID, 0, 5)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if total != 12 {
		t.Fatalf("expected total=12, got %d", total)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
}

func TestListAttemptErrors_Pagination(t *testing.T) {
	f := newQueryFixture(t)
	src := f.addSource(t, ragmodel.RAGSourceStatusActive, ragmodel.RAGSourceKindIntegration)
	attempt := &ragmodel.RAGIndexAttempt{
		OrgID:       f.Org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusCompletedWithErrors,
	}
	if err := f.DB.Create(attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	t.Cleanup(func() { f.DB.Where("id = ?", attempt.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	for i := 0; i < 3; i++ {
		e := &ragmodel.RAGIndexAttemptError{
			OrgID:          f.Org.ID,
			IndexAttemptID: attempt.ID,
			RAGSourceID:    src.ID,
			FailureMessage: "boom",
		}
		if err := f.DB.Create(e).Error; err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	rows, total, err := db.ListAttemptErrors(f.DB, attempt.ID, 0, 10)
	if err != nil {
		t.Fatalf("list errors: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("expected 3/3, got %d/%d", len(rows), total)
	}
}

func TestListSupportedIntegrations(t *testing.T) {
	d := testhelpers.ConnectTestDB(t)

	supported1 := testhelpers.NewTestInIntegration(t, d, "picker-supported-a")
	supported2 := testhelpers.NewTestInIntegration(t, d, "picker-supported-b")
	unsupported := testhelpers.NewTestInIntegration(t, d, "picker-unsupported")

	if err := d.Model(&model.InIntegration{}).
		Where("id IN ?", []uuid.UUID{supported1.ID, supported2.ID}).
		Update("supports_rag_source", true).Error; err != nil {
		t.Fatalf("flag supported: %v", err)
	}

	rows, err := db.ListSupportedIntegrations(d)
	if err != nil {
		t.Fatalf("list integrations: %v", err)
	}

	ids := map[uuid.UUID]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	if !ids[supported1.ID] || !ids[supported2.ID] {
		t.Fatalf("expected both supported integrations to appear")
	}
	if ids[unsupported.ID] {
		t.Fatalf("unsupported integration should not appear")
	}
}
