package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type auditTestHarness struct {
	db      *gorm.DB
	handler *handler.AuditHandler
	router  *chi.Mux
}

func newAuditHarness(t *testing.T) *auditTestHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewAuditHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/audit", h.List)
	return &auditTestHarness{db: db, handler: h, router: r}
}

func (h *auditTestHarness) doRequest(t *testing.T, path string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func seedAuditEntries(t *testing.T, db *gorm.DB, orgID uuid.UUID, count int, action string) []model.AuditEntry {
	t.Helper()
	entries := make([]model.AuditEntry, count)
	for i := range entries {
		ip := fmt.Sprintf("10.0.0.%d", i+1)
		entries[i] = model.AuditEntry{
			OrgID:  orgID,
			Action: action,
			Metadata: model.JSON{
				"method":     "POST",
				"path":       fmt.Sprintf("/v1/test/%d", i),
				"status":     float64(200),
				"latency_ms": float64(50 + i),
			},
			IPAddress: &ip,
			CreatedAt: time.Now().Add(-time.Duration(count-i) * time.Second),
		}
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatalf("seed audit entries: %v", err)
	}
	return entries
}

func TestAuditHandler_List_ReturnsEntries(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)
	seedAuditEntries(t, h.db, org.ID, 3, "api.request")

	rr := h.doRequest(t, "/v1/audit", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(page.Data))
	}

	first := page.Data[0]
	if first["method"] == nil {
		t.Fatal("expected method in response")
	}
	if first["path"] == nil {
		t.Fatal("expected path in response")
	}
	if first["status"] == nil {
		t.Fatal("expected status in response")
	}
	if first["latency_ms"] == nil {
		t.Fatal("expected latency_ms in response")
	}
	if first["ip_address"] == nil {
		t.Fatal("expected ip_address in response")
	}
	if first["created_at"] == nil {
		t.Fatal("expected created_at in response")
	}
}

func TestAuditHandler_List_OrderedByIDDesc(t *testing.T) {
	h := newAuditHarness(t)
	org := createTestOrg(t, h.db)
	seedAuditEntries(t, h.db, org.ID, 3, "api.request")

	rr := h.doRequest(t, "/v1/audit", &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)

	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(page.Data))
	}

	for i := 1; i < len(page.Data); i++ {
		prev := page.Data[i-1]["id"].(float64)
		curr := page.Data[i]["id"].(float64)
		if prev <= curr {
			t.Fatalf("expected descending IDs, got %v then %v at index %d", prev, curr, i)
		}
	}
}
