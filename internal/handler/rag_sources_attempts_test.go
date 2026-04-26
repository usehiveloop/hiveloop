package handler_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	"github.com/usehiveloop/hiveloop/internal/rag/testhelpers"
)

func TestListAttempts_Paginated(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	base := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 12; i++ {
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

	rr := get(t, h, "/v1/rag/sources/"+src.ID.String()+"/attempts?page=0&page_size=5")
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	decodeJSON(t, rr, &resp)
	if resp.Total != 12 || len(resp.Data) != 5 {
		t.Fatalf("expected 12 total / 5 returned; got %d / %d", resp.Total, len(resp.Data))
	}
}

func TestGetAttemptDetail_IncludesPerDocErrors(t *testing.T) {
	h := newRAGHarness(t)
	src := h.createSource(t)
	attempt := &ragmodel.RAGIndexAttempt{
		OrgID:       h.Org.ID,
		RAGSourceID: src.ID,
		Status:      ragmodel.IndexingStatusCompletedWithErrors,
	}
	if err := h.DB.Create(attempt).Error; err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	t.Cleanup(func() { h.DB.Where("id = ?", attempt.ID).Delete(&ragmodel.RAGIndexAttempt{}) })

	for i := 0; i < 3; i++ {
		e := &ragmodel.RAGIndexAttemptError{
			OrgID:          h.Org.ID,
			IndexAttemptID: attempt.ID,
			RAGSourceID:    src.ID,
			FailureMessage: "doc failed",
		}
		if err := h.DB.Create(e).Error; err != nil {
			t.Fatalf("create error: %v", err)
		}
	}

	rr := get(t, h, "/v1/rag/sources/"+src.ID.String()+"/attempts/"+attempt.ID.String())
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		Errors     []map[string]any `json:"errors"`
		ErrorCount int              `json:"error_count"`
	}
	decodeJSON(t, rr, &resp)
	if resp.ErrorCount != 3 || len(resp.Errors) != 3 {
		t.Fatalf("expected 3 errors, got count=%d len=%d", resp.ErrorCount, len(resp.Errors))
	}
}

func TestListIntegrations_OnlySupportedReturned(t *testing.T) {
	h := newRAGHarness(t)
	supportedA := testhelpers.NewTestInIntegration(t, h.DB, "picker-a")
	supportedB := testhelpers.NewTestInIntegration(t, h.DB, "picker-b")
	unsupported := testhelpers.NewTestInIntegration(t, h.DB, "picker-c")

	if err := h.DB.Model(&model.InIntegration{}).
		Where("id IN ?", []uuid.UUID{supportedA.ID, supportedB.ID}).
		Update("supports_rag_source", true).Error; err != nil {
		t.Fatalf("flag supported: %v", err)
	}

	rr := get(t, h, "/v1/rag/integrations")
	mustStatus(t, rr, http.StatusOK)
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	decodeJSON(t, rr, &resp)

	seen := map[string]bool{}
	for _, r := range resp.Data {
		seen[r["id"].(string)] = true
	}
	if !seen[supportedA.ID.String()] || !seen[supportedB.ID.String()] {
		t.Fatalf("expected both supported integrations to appear")
	}
	if seen[unsupported.ID.String()] {
		t.Fatalf("unsupported integration leaked into picker")
	}
}
