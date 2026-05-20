package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_Audit_WritesToPostgres(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      "integration-audit-org",
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	aw := middleware.NewAuditWriter(t.Context(), db, 100, 10*time.Millisecond)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	handler := middleware.Audit(aw, "proxy.request")(inner)

	req := httptest.NewRequest(http.MethodPost, "/v1/proxy/v1/messages", nil)
	req = middleware.WithOrg(req, &org)
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aw.Shutdown(ctx)

	var entries []model.AuditEntry
	if err := db.Where("org_id = ?", orgID).Find(&entries).Error; err != nil {
		t.Fatalf("failed to query audit_log: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Action != "proxy.request" {
		t.Fatalf("expected action 'proxy.request', got %s", entry.Action)
	}
	if entry.OrgID != orgID {
		t.Fatalf("expected org_id %s, got %s", orgID, entry.OrgID)
	}
	if entry.IPAddress == nil || *entry.IPAddress != "192.168.1.100" {
		t.Fatalf("expected IP '192.168.1.100', got %v", entry.IPAddress)
	}
	if entry.Metadata == nil {
		t.Fatal("expected metadata, got nil")
	}
	if entry.Metadata["method"] != "POST" {
		t.Fatalf("expected method POST in metadata, got %v", entry.Metadata["method"])
	}
}

func TestIntegration_Audit_MultipleRequestsFlushed(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      "integration-audit-multi",
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	aw := middleware.NewAuditWriter(t.Context(), db, 100, 10*time.Millisecond)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Audit(aw)(inner)

	for range 10 {
		req := httptest.NewRequest(http.MethodPost, "/v1/proxy/chat", nil)
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aw.Shutdown(ctx)

	var count int64
	db.Model(&model.AuditEntry{}).Where("org_id = ?", orgID).Count(&count)
	if count != 10 {
		t.Fatalf("expected 10 audit entries in Postgres, got %d", count)
	}
}

func TestIntegration_RateLimit_EnforcesLimit(t *testing.T) {
	db := connectTestDB(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      "integration-ratelimit-org",
		RateLimit: 1,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2 = middleware.WithOrg(req2, &org)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rr2.Code)
	}

	if rr2.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429")
	}
}

func TestIntegration_RateLimit_IsolatedPerOrg(t *testing.T) {
	db := connectTestDB(t)

	org1 := model.Org{
		ID:        uuid.New(),
		Name:      "integration-rl-org1",
		RateLimit: 1,
		Active:    true,
	}
	if err := db.Create(&org1).Error; err != nil {
		t.Fatalf("failed to create org1: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org1.ID) })

	org2 := model.Org{
		ID:        uuid.New(),
		Name:      "integration-rl-org2",
		RateLimit: 6000,
		Active:    true,
	}
	if err := db.Create(&org2).Error; err != nil {
		t.Fatalf("failed to create org2: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org2.ID) })

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org1)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org1)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("org1 should be rate limited, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("org2 should not be rate limited, got %d", rr.Code)
	}
}
