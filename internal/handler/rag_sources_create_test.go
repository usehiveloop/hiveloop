package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

func TestCreateSource_IntegrationKind_HappyPath(t *testing.T) {
	h := newRAGHarness(t)
	rr := post(t, h, "/v1/rag/sources", map[string]any{
		"kind":                 "INTEGRATION",
		"name":                 "my-source",
		"in_connection_id":     h.Conn.ID.String(),
		"access_type":          "private",
		"refresh_freq_seconds": 120,
	})
	mustStatus(t, rr, http.StatusCreated)

	var resp map[string]any
	decodeJSON(t, rr, &resp)
	if resp["status"] != "INITIAL_INDEXING" {
		t.Fatalf("expected status INITIAL_INDEXING, got %v", resp["status"])
	}
	if resp["name"] != "my-source" {
		t.Fatalf("expected name my-source, got %v", resp["name"])
	}

	var dbRow ragmodel.RAGSource
	if err := h.DB.Where("id = ?", resp["id"]).First(&dbRow).Error; err != nil {
		t.Fatalf("row not found: %v", err)
	}
	if dbRow.OrgIDValue != h.Org.ID {
		t.Fatalf("org mismatch")
	}
	t.Cleanup(func() { h.DB.Where("id = ?", dbRow.ID).Delete(&ragmodel.RAGSource{}) })
}

func TestCreateSource_IntegrationKind_RejectsCrossOrgConnection(t *testing.T) {
	h := newRAGHarness(t)
	rr := post(t, h, "/v1/rag/sources", map[string]any{
		"kind":             "INTEGRATION",
		"name":             "cross-org",
		"in_connection_id": h.OtherConn.ID.String(),
		"access_type":      "private",
	})
	mustStatus(t, rr, http.StatusNotFound)
}

func TestCreateSource_IntegrationKind_RejectsUnsupportedIntegration(t *testing.T) {
	h := newRAGHarness(t)
	if err := h.DB.Model(&model.InIntegration{}).
		Where("id = ?", h.Integ.ID).
		Update("supports_rag_source", false).Error; err != nil {
		t.Fatalf("toggle off support: %v", err)
	}
	rr := post(t, h, "/v1/rag/sources", map[string]any{
		"kind":             "INTEGRATION",
		"name":             "unsupported",
		"in_connection_id": h.Conn.ID.String(),
		"access_type":      "private",
	})
	mustStatus(t, rr, http.StatusUnprocessableEntity)
	if !bodyContains(rr, "does not support") {
		t.Fatalf("expected explicit unsupported message; got %s", rr.Body.String())
	}
}

func TestCreateSource_DuplicateInConnection_Returns409(t *testing.T) {
	h := newRAGHarness(t)
	body := map[string]any{
		"kind":             "INTEGRATION",
		"name":             "first",
		"in_connection_id": h.Conn.ID.String(),
		"access_type":      "private",
	}
	rr := post(t, h, "/v1/rag/sources", body)
	mustStatus(t, rr, http.StatusCreated)
	var first map[string]any
	decodeJSON(t, rr, &first)
	t.Cleanup(func() { h.DB.Where("id = ?", first["id"]).Delete(&ragmodel.RAGSource{}) })

	body["name"] = "second"
	rr = post(t, h, "/v1/rag/sources", body)
	mustStatus(t, rr, http.StatusConflict)
}

func TestCreateSource_RefreshFreqUnder60s_Rejects(t *testing.T) {
	h := newRAGHarness(t)
	rr := post(t, h, "/v1/rag/sources", map[string]any{
		"kind":                 "INTEGRATION",
		"name":                 "fast",
		"in_connection_id":     h.Conn.ID.String(),
		"access_type":          "private",
		"refresh_freq_seconds": 30,
	})
	mustStatus(t, rr, http.StatusUnprocessableEntity)
	if !bodyContains(rr, "60") {
		t.Fatalf("expected 60s minimum mentioned; got %s", rr.Body.String())
	}
}

func TestCreateSource_NonAdmin_Returns403(t *testing.T) {
	h := newRAGHarness(t)
	makeAdminUser(t, h, h.User, h.Org, "member")
	r := adminGuardedRouter(t, h)

	body, _ := json.Marshal(map[string]any{
		"kind":             "INTEGRATION",
		"name":             "x",
		"in_connection_id": h.Conn.ID.String(),
		"access_type":      "private",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/sources", bytesReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, h.Org)
	req = middleware.WithUser(req, h.User)
	req = withClaimsFor(req, h.User, h.Org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
