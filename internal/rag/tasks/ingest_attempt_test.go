package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	coremodel "github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/rag/testhelpers"
)

func TestOpenAttemptClaimsReservedAttempt(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	sourceID := uuid.New()
	src := ragmodel.RAGSource{
		ID:          sourceID,
		OrgIDValue:  uuid.New(),
		KindValue:   ragmodel.RAGSourceKindWebsite,
		Name:        "claim-reserved-attempt",
		Status:      ragmodel.RAGSourceStatusActive,
		Enabled:     true,
		ConfigValue: coremodel.JSON{},
		AccessType:  ragmodel.AccessTypePublic,
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}
	t.Cleanup(func() {
		db.Where("rag_source_id = ?", sourceID).Delete(&ragmodel.RAGIndexAttempt{})
		db.Delete(&ragmodel.RAGSource{}, "id = ?", sourceID)
	})

	reserved := ragmodel.RAGIndexAttempt{
		OrgID:       src.OrgIDValue,
		RAGSourceID: sourceID,
		Status:      ragmodel.IndexingStatusNotStarted,
	}
	if err := db.Create(&reserved).Error; err != nil {
		t.Fatalf("create reservation: %v", err)
	}

	attempt, err := openAttempt(context.Background(), db, &src, false, &reserved.ID)
	if err != nil {
		t.Fatalf("claim reserved attempt: %v", err)
	}
	if attempt.ID != reserved.ID {
		t.Fatalf("claimed attempt id = %s, want %s", attempt.ID, reserved.ID)
	}
	if attempt.Status != ragmodel.IndexingStatusInProgress {
		t.Fatalf("claimed attempt status = %q, want in_progress", attempt.Status)
	}
	if attempt.TimeStarted == nil {
		t.Fatal("claimed attempt should record time_started")
	}
	if attempt.LastProgressTime == nil {
		t.Fatal("claimed attempt should record last_progress_time")
	}

	if _, err := openAttempt(context.Background(), db, &src, false, &reserved.ID); !errors.Is(err, errIngestAttemptAlreadyClaimed) {
		t.Fatalf("second claim error = %v, want errIngestAttemptAlreadyClaimed", err)
	}
}
