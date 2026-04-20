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
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type generationTestHarness struct {
	db      *gorm.DB
	handler *handler.GenerationHandler
	router  *chi.Mux
}

func newGenerationHarness(t *testing.T) *generationTestHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewGenerationHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/generations", h.List)
	r.Get("/v1/generations/{id}", h.Get)
	return &generationTestHarness{db: db, handler: h, router: r}
}

func (h *generationTestHarness) doRequest(t *testing.T, path string, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func seedGenerations(t *testing.T, db *gorm.DB, orgID uuid.UUID, credID uuid.UUID, count int) []model.Generation {
	t.Helper()
	gens := make([]model.Generation, count)
	for i := range gens {
		ttfb := 50 + i*10
		gens[i] = model.Generation{
			ID:           fmt.Sprintf("gen_test_%s_%d", orgID.String()[:8], i),
			OrgID:        orgID,
			CredentialID: credID,
			TokenJTI:     fmt.Sprintf("jti_%s_%d", orgID.String()[:8], i),
			ProviderID:   "openai",
			Model:        "gpt-4o",
			RequestPath:  "/v1/chat/completions",
			IsStreaming:  i%2 == 0,
			InputTokens:  100 + i*50,
			OutputTokens: 50 + i*25,
			CachedTokens: 10 * i,
			Cost:         0.005 * float64(i+1),
			TTFBMs:       &ttfb,
			TotalMs:      200 + i*50,
			UpstreamStatus: 200,
			UserID:       fmt.Sprintf("user_%d", i%3),
			Tags:         pq.StringArray{"chat", fmt.Sprintf("tier_%d", i%2)},
			CreatedAt:    time.Now().Add(-time.Duration(count-i) * time.Minute),
		}
	}
	if err := db.Create(&gens).Error; err != nil {
		t.Fatalf("seed generations: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.Generation{})
	})
	return gens
}

// --------------------------------------------------------------------------
// GET /v1/generations/{id} — Get
// --------------------------------------------------------------------------

func TestGenerationHandler_Get_ReturnsGeneration(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)
	credID := uuid.New()
	gens := seedGenerations(t, h.db, org.ID, credID, 1)

	rr := h.doRequest(t, "/v1/generations/"+gens[0].ID, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != gens[0].ID {
		t.Errorf("id = %v, want %v", resp["id"], gens[0].ID)
	}
	if resp["provider_id"] != "openai" {
		t.Errorf("provider_id = %v, want openai", resp["provider_id"])
	}
	if resp["model"] != "gpt-4o" {
		t.Errorf("model = %v, want gpt-4o", resp["model"])
	}
}

func TestGenerationHandler_Get_WrongOrg(t *testing.T) {
	h := newGenerationHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	credID := uuid.New()
	gens := seedGenerations(t, h.db, org1.ID, credID, 1)

	rr := h.doRequest(t, "/v1/generations/"+gens[0].ID, &org2)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGenerationHandler_Get_NotFound(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/generations/gen_nonexistent", &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// GET /v1/generations — List
// --------------------------------------------------------------------------

func TestGenerationHandler_List_ReturnsGenerations(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)
	credID := uuid.New()
	seedGenerations(t, h.db, org.ID, credID, 5)

	rr := h.doRequest(t, "/v1/generations", &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 5 {
		t.Fatalf("expected 5 generations, got %d", len(page.Data))
	}
}

func TestGenerationHandler_List_FilterByModel(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)
	credID := uuid.New()
	seedGenerations(t, h.db, org.ID, credID, 3)

	// Add a generation with different model
	h.db.Create(&model.Generation{
		ID:           fmt.Sprintf("gen_diff_%s", org.ID.String()[:8]),
		OrgID:        org.ID,
		CredentialID: credID,
		TokenJTI:     "jti_diff",
		ProviderID:   "anthropic",
		Model:        "claude-sonnet-4-20250514",
		RequestPath:  "/v1/messages",
		UpstreamStatus: 200,
		CreatedAt:    time.Now(),
	})
	t.Cleanup(func() {
		h.db.Where("id = ?", fmt.Sprintf("gen_diff_%s", org.ID.String()[:8])).Delete(&model.Generation{})
	})

	rr := h.doRequest(t, "/v1/generations?model=claude-sonnet-4-20250514", &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 1 {
		t.Fatalf("expected 1 claude generation, got %d", len(page.Data))
	}
}

func TestGenerationHandler_List_FilterByUserID(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)
	credID := uuid.New()
	seedGenerations(t, h.db, org.ID, credID, 6)

	rr := h.doRequest(t, "/v1/generations?user_id=user_0", &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 2 {
		t.Fatalf("expected 2 user_0 generations, got %d", len(page.Data))
	}
}

func TestGenerationHandler_List_OrgIsolation(t *testing.T) {
	h := newGenerationHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	credID := uuid.New()
	seedGenerations(t, h.db, org1.ID, credID, 3)

	rr := h.doRequest(t, "/v1/generations", &org2)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 0 {
		t.Fatalf("org2 should see 0 generations, got %d", len(page.Data))
	}
}

func TestGenerationHandler_List_Empty(t *testing.T) {
	h := newGenerationHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, "/v1/generations", &org)
	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 0 {
		t.Fatalf("expected 0 generations, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Fatal("expected has_more=false")
	}
}

func TestGenerationHandler_List_MissingOrg(t *testing.T) {
	h := newGenerationHarness(t)
	rr := h.doRequest(t, "/v1/generations", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
