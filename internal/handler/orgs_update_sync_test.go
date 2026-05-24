package handler_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type stubOrgEmployeeSyncer struct {
	calls int
	orgID uuid.UUID
	err   error
}

func (s *stubOrgEmployeeSyncer) SyncOrgHivyEmployee(_ context.Context, orgID uuid.UUID) error {
	s.calls++
	s.orgID = orgID
	return s.err
}

func TestOrgUpdate_SyncTrueMarksOrgOnboardedAfterEmployeeSync(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")
	syncer := &stubOrgEmployeeSyncer{}
	h.orgHandler.SetEmployeeSyncer(syncer)

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"website": "https://acme.example",
		"sync":    true,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	if syncer.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncer.calls)
	}
	if syncer.orgID != org.ID {
		t.Fatalf("sync org id = %s, want %s", syncer.orgID, org.ID)
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Website != "https://acme.example" {
		t.Errorf("db website: got %q", reloaded.Website)
	}
	if !reloaded.Onboarded {
		t.Fatal("org onboarded = false, want true after employee sync")
	}
}

func TestOrgUpdate_SyncFailureSavesFieldsWithoutMarkingOnboarded(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")
	syncer := &stubOrgEmployeeSyncer{err: errors.New("runtime unavailable")}
	h.orgHandler.SetEmployeeSyncer(syncer)

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"prompt_company": " Runs field service operations. ",
		"sync":           true,
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to start Hivy employee sandbox") {
		t.Fatalf("body = %s, want sandbox retry error", rr.Body.String())
	}
	if syncer.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncer.calls)
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.PromptCompany != "Runs field service operations." {
		t.Errorf("db prompt_company: got %q", reloaded.PromptCompany)
	}
	if reloaded.Onboarded {
		t.Fatal("org onboarded = true, want false after failed employee sync")
	}
}

func TestOrgUpdate_SyncTrueWithoutConfiguredSyncerSavesFieldsWithoutMarkingOnboarded(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"website": "https://retry.example",
		"sync":    true,
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Website != "https://retry.example" {
		t.Errorf("db website: got %q", reloaded.Website)
	}
	if reloaded.Onboarded {
		t.Fatal("org onboarded = true, want false when employee sync is not configured")
	}
}
