package tasks

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestHandleConnectorBuildError_DisablesSlackSourceWithMissingProfile(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	orgID := uuid.New()
	profileID := uuid.New()
	source := ragmodel.RAGSource{
		ID:                 uuid.New(),
		OrgIDValue:         orgID,
		KindValue:          ragmodel.RAGSourceKindSlackBotProfile,
		Name:               "Slack bot profile",
		Status:             ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:            true,
		ConfigValue:        map[string]any{"agent_profile_id": profileID.String()},
		AccessType:         ragmodel.AccessTypePublic,
		RefreshFreqSeconds: intPtr(900),
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	handled, err := handleConnectorBuildError(context.Background(), db, &source, fmt.Errorf("ingest: build connector: load slack profile: %w", gorm.ErrRecordNotFound))
	if err != nil {
		t.Fatalf("handle connector build error: %v", err)
	}
	if !handled {
		t.Fatal("expected missing Slack profile to be handled as permanent")
	}

	var stored ragmodel.RAGSource
	if err := db.First(&stored, "id = ?", source.ID).Error; err != nil {
		t.Fatalf("load source: %v", err)
	}
	if stored.Enabled {
		t.Fatal("source should be disabled")
	}
	if stored.Status != ragmodel.RAGSourceStatusError {
		t.Fatalf("source status = %q, want ERROR", stored.Status)
	}
	if !stored.InRepeatedErrorState {
		t.Fatal("source should be marked in repeated error state")
	}

	var attempts []ragmodel.RAGIndexAttempt
	if err := db.Where("rag_source_id = ?", source.ID).Find(&attempts).Error; err != nil {
		t.Fatalf("load attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
	if attempts[0].Status != ragmodel.IndexingStatusFailed {
		t.Fatalf("attempt status = %q, want failed", attempts[0].Status)
	}
	if attempts[0].ErrorMsg == nil || *attempts[0].ErrorMsg == "" {
		t.Fatal("attempt should record connector build error")
	}
}

func TestHandleConnectorBuildError_DoesNotDisableTransientErrors(t *testing.T) {
	db := testhelpers.ConnectTestDB(t)
	source := ragmodel.RAGSource{
		ID:         uuid.New(),
		OrgIDValue: uuid.New(),
		KindValue:  ragmodel.RAGSourceKindSlackBotProfile,
		Name:       "Slack bot profile",
		Status:     ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:    true,
		ConfigValue: map[string]any{
			"agent_profile_id": uuid.NewString(),
		},
		AccessType: ragmodel.AccessTypePublic,
	}
	if err := db.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	handled, err := handleConnectorBuildError(context.Background(), db, &source, fmt.Errorf("temporary slack api issue"))
	if err != nil {
		t.Fatalf("handle connector build error: %v", err)
	}
	if handled {
		t.Fatal("transient connector build error should not be handled as permanent")
	}
}

func intPtr(v int) *int { return &v }
