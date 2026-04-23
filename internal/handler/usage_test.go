package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type usageHarness struct {
	db      *gorm.DB
	handler *handler.UsageHandler
	router  *chi.Mux
}

func newUsageHarness(t *testing.T) *usageHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewUsageHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/usage", func(w http.ResponseWriter, req *http.Request) {
		h.Get(w, req)
	})
	return &usageHarness{db: db, handler: h, router: r}
}

func (h *usageHarness) doRequest(t *testing.T, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		if org != nil {
			r = middleware.WithOrg(r, org)
		}
		h.handler.Get(w, r)
	})
	r.ServeHTTP(rr, req)
	return rr
}

func TestUsageHandler_EmptyOrg(t *testing.T) {
	h := newUsageHarness(t)
	org := createTestOrg(t, h.db)
	t.Cleanup(func() { cleanupOrg(t, h.db, org.ID) })

	rr := h.doRequest(t, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Credentials struct {
			Total   int64 `json:"total"`
			Active  int64 `json:"active"`
			Revoked int64 `json:"revoked"`
		} `json:"credentials"`
		Tokens struct {
			Total int64 `json:"total"`
		} `json:"tokens"`
		APIKeys struct {
			Total int64 `json:"total"`
		} `json:"api_keys"`
		Requests struct {
			Total     int64 `json:"total"`
			Today     int64 `json:"today"`
			Yesterday int64 `json:"yesterday"`
			Last7d    int64 `json:"last_7d"`
			Last30d   int64 `json:"last_30d"`
		} `json:"requests"`
		DailyRequests  []any `json:"daily_requests"`
		TopCredentials []any `json:"top_credentials"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Credentials.Total != 0 {
		t.Errorf("credentials.total: got %d, want 0", resp.Credentials.Total)
	}
	if resp.Tokens.Total != 0 {
		t.Errorf("tokens.total: got %d, want 0", resp.Tokens.Total)
	}
	if resp.Requests.Total != 0 {
		t.Errorf("requests.total: got %d, want 0", resp.Requests.Total)
	}
	if resp.DailyRequests == nil {
		t.Error("daily_requests should be empty array, not null")
	}
	if resp.TopCredentials == nil {
		t.Error("top_credentials should be empty array, not null")
	}
}

func TestUsageHandler_NoOrgContext(t *testing.T) {
	h := newUsageHarness(t)
	rr := h.doRequest(t, nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}
