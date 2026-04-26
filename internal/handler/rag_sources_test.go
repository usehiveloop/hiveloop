package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

// ---------- List + detail ----------

func TestListSources_OrgScoped(t *testing.T) {
	h := newRAGHarness(t)
	for i := 0; i < 2; i++ {
		h.createSource(t)
	}
	otherConn := testhelpers.NewTestInConnection(t, h.DB, h.OtherOrg.ID, h.OtherUser.ID, h.Integ.ID)
	for i := 0; i < 2; i++ {
		c := testhelpers.NewTestInConnection(t, h.DB, h.OtherOrg.ID, h.OtherUser.ID, h.Integ.ID)
		_ = c
		// reuse otherConn id pattern via raw create
		src := &ragmodel.RAGSource{
			OrgIDValue:     h.OtherOrg.ID,
			KindValue:      ragmodel.RAGSourceKindIntegration,
			Name:           "other-" + uuid.New().String()[:8],
			Status:         ragmodel.RAGSourceStatusActive,
			Enabled:        true,
			AccessType:     ragmodel.AccessTypePrivate,
			InConnectionID: &c.ID,
		}
		if err := h.DB.Create(src).Error; err != nil {
			t.Fatalf("create other-org src: %v", err)
		}
		t.Cleanup(func() { h.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })
	}
	_ = otherConn

	rr := get(t, h, "/v1/rag/sources")
	mustStatus(t, rr, http.StatusOK)

	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Total != 2 {
		t.Fatalf("expected exactly 2 sources for caller org, got total=%d", resp.Total)
	}
	for _, row := range resp.Data {
		if row["org_id"] != h.Org.ID.String() {
			t.Fatalf("leaked cross-org row: %v", row["org_id"])
		}
	}
}

func TestListSources_Paginated(t *testing.T) {
	h := newRAGHarness(t)
	for i := 0; i < 25; i++ {
		h.createSource(t)
	}
	rr := get(t, h, "/v1/rag/sources?page=0&page_size=10")
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Total != 25 {
		t.Fatalf("expected total=25, got %d", resp.Total)
	}
	if len(resp.Data) != 10 {
		t.Fatalf("expected page size 10, got %d", len(resp.Data))
	}
}

func TestListSources_FilterByStatus(t *testing.T) {
	h := newRAGHarness(t)
	for i := 0; i < 2; i++ {
		h.createSource(t)
	}
	paused := h.createSource(t, func(s *ragmodel.RAGSource) { s.Status = ragmodel.RAGSourceStatusPaused })

	rr := get(t, h, "/v1/rag/sources?status=PAUSED")
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Total != 1 || resp.Data[0]["id"] != paused.ID.String() {
		t.Fatalf("expected only paused source; got total=%d", resp.Total)
	}
}

func TestGetSourceDetail_IncludesLast5Attempts(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	base := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 7; i++ {
		a := &ragmodel.RAGIndexAttempt{
			OrgID:       h.Org.ID,
			RAGSourceID: src.ID,
			Status:      ragmodel.IndexingStatusSuccess,
			TimeCreated: base.Add(time.Duration(i) * time.Minute),
		}
		if err := h.DB.Create(a).Error; err != nil {
			t.Fatalf("create attempt: %v", err)
		}
	}
	t.Cleanup(func() { h.DB.Where("rag_source_id = ?", src.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	rr := get(t, h, "/v1/rag/sources/"+src.ID.String())
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		RecentAttempts []map[string]any `json:"recent_attempts"`
	}
	decodeJSON(t, rr, &resp)
	if len(resp.RecentAttempts) != 5 {
		t.Fatalf("expected 5 recent attempts, got %d", len(resp.RecentAttempts))
	}
}

func TestGetSourceDetail_CrossOrg_Returns404(t *testing.T) {
	h := newRAGHarness(t)
	otherConn := testhelpers.NewTestInConnection(t, h.DB, h.OtherOrg.ID, h.OtherUser.ID, h.Integ.ID)
	src := &ragmodel.RAGSource{
		OrgIDValue:     h.OtherOrg.ID,
		KindValue:      ragmodel.RAGSourceKindIntegration,
		Name:           "cross",
		Status:         ragmodel.RAGSourceStatusActive,
		Enabled:        true,
		AccessType:     ragmodel.AccessTypePrivate,
		InConnectionID: &otherConn.ID,
	}
	if err := h.DB.Create(src).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { h.DB.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{}) })

	rr := get(t, h, "/v1/rag/sources/"+src.ID.String())
	mustStatus(t, rr, http.StatusNotFound)
}

// ---------- Update ----------

func TestUpdateSource_PauseAndResume(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr := patch(t, h, "/v1/rag/sources/"+src.ID.String(), map[string]any{"status": "PAUSED"})
	mustStatus(t, rr, http.StatusOK)
	var dbRow ragmodel.RAGSource
	h.DB.Where("id = ?", src.ID).First(&dbRow)
	if dbRow.Status != ragmodel.RAGSourceStatusPaused {
		t.Fatalf("expected PAUSED, got %s", dbRow.Status)
	}

	rr = patch(t, h, "/v1/rag/sources/"+src.ID.String(), map[string]any{"status": "ACTIVE"})
	mustStatus(t, rr, http.StatusOK)
	h.DB.Where("id = ?", src.ID).First(&dbRow)
	if dbRow.Status != ragmodel.RAGSourceStatusActive {
		t.Fatalf("expected ACTIVE, got %s", dbRow.Status)
	}
}

func TestUpdateSource_RejectsClientSetDeletingStatus(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	rr := patch(t, h, "/v1/rag/sources/"+src.ID.String(), map[string]any{"status": "DELETING"})
	mustStatus(t, rr, http.StatusUnprocessableEntity)
}

func TestUpdateSource_FreqValidation_PropagatesModelErrors(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	rr := patch(t, h, "/v1/rag/sources/"+src.ID.String(), map[string]any{"refresh_freq_seconds": 10})
	mustStatus(t, rr, http.StatusUnprocessableEntity)
	if !bodyContains(rr, "60") {
		t.Fatalf("expected 60s minimum mentioned; got %s", rr.Body.String())
	}
}

func TestUpdateSource_IndexingStartFloor(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	floor := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rr := patch(t, h, "/v1/rag/sources/"+src.ID.String(), map[string]any{
		"indexing_start": floor.Format(time.RFC3339),
	})
	mustStatus(t, rr, http.StatusOK)

	var dbRow ragmodel.RAGSource
	h.DB.Where("id = ?", src.ID).First(&dbRow)
	if dbRow.IndexingStart == nil || !dbRow.IndexingStart.Equal(floor) {
		t.Fatalf("expected indexing_start=%s, got %v", floor, dbRow.IndexingStart)
	}
}

// ---------- Delete ----------

func TestDeleteSource_TombstonesAndStopsScheduler(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)

	rr := del(t, h, "/v1/rag/sources/"+src.ID.String())
	mustStatus(t, rr, http.StatusAccepted)

	var dbRow ragmodel.RAGSource
	h.DB.Where("id = ?", src.ID).First(&dbRow)
	if dbRow.Status != ragmodel.RAGSourceStatusDeleting {
		t.Fatalf("expected DELETING, got %s", dbRow.Status)
	}

	var resp map[string]any
	decodeJSON(t, rr, &resp)
	if resp["status"] != "deleting" || resp["note"] == "" {
		t.Fatalf("expected deletion note, got %v", resp)
	}

	// scheduler eligibility query mirrors idx_rag_sources_needs_ingest:
	// status IN ('ACTIVE','INITIAL_INDEXING'). DELETING must be excluded.
	var n int64
	h.DB.Model(&ragmodel.RAGSource{}).
		Where("id = ? AND enabled = true AND status IN ('ACTIVE','INITIAL_INDEXING')", src.ID).
		Count(&n)
	if n != 0 {
		t.Fatalf("DELETING source should not be scheduler-eligible; matched %d", n)
	}
}
