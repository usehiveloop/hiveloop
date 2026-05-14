package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	coremodel "github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestScanIngestDueReservesQueuedAttempt(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	sourceID := uuid.New()
	refresh := 60
	src := ragmodel.RAGSource{
		ID:                 sourceID,
		OrgIDValue:         uuid.New(),
		KindValue:          ragmodel.RAGSourceKindWebsite,
		Name:               "queued-ingest-reservation",
		Status:             ragmodel.RAGSourceStatusActive,
		Enabled:            true,
		ConfigValue:        coremodel.JSON{},
		AccessType:         ragmodel.AccessTypePublic,
		RefreshFreqSeconds: &refresh,
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	t.Cleanup(func() {
		db.Where("rag_source_id = ?", sourceID).Delete(&ragmodel.RAGIndexAttempt{})
		db.Delete(&ragmodel.RAGSource{}, "id = ?", sourceID)
	})

	enq := &enqueue.MockClient{}
	cfg := Config{
		IngestTick:   15 * time.Second,
		UniqueSlack:  30 * time.Second,
		EnqueueLimit: 10,
	}

	n, err := ScanIngestDue(context.Background(), db, enq, cfg)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("first scan enqueued = %d, want 1", n)
	}
	tasks := enq.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("enqueued tasks = %d, want 1", len(tasks))
	}
	payload, err := ragtasks.UnmarshalIngest(tasks[0].Payload)
	if err != nil {
		t.Fatalf("unmarshal enqueued ingest: %v", err)
	}
	if payload.AttemptID == nil {
		t.Fatal("scheduled ingest payload should include reserved attempt id")
	}

	var attempts []ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ?", sourceID).Find(&attempts).Error; err != nil {
		t.Fatalf("load attempts after first scan: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts after first scan = %d, want 1", len(attempts))
	}
	if attempts[0].Status != ragmodel.IndexingStatusNotStarted {
		t.Fatalf("reserved attempt status = %q, want not_started", attempts[0].Status)
	}
	if attempts[0].ID != *payload.AttemptID {
		t.Fatalf("payload attempt id = %s, want %s", *payload.AttemptID, attempts[0].ID)
	}

	n, err = ScanIngestDue(context.Background(), db, enq, cfg)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if n != 0 {
		t.Fatalf("second scan enqueued = %d, want 0", n)
	}
	if got := len(enq.Tasks()); got != 1 {
		t.Fatalf("enqueued tasks after second scan = %d, want 1", got)
	}
}
