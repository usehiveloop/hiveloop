package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *employeeHarness) patchEmployeeSpecialist(t *testing.T, m orgWithMember, employeeID uuid.UUID, slug string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("PATCH", "/v1/employees/"+employeeID.String()+"/specialists/"+slug, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestIntegration_EmployeeSpecialists_ListIncludesModelConfig(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)

	req := httptest.NewRequest("GET", "/v1/employees/"+emp.ID.String()+"/specialists", nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var specialists []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &specialists); err != nil {
		t.Fatalf("decode specialists: %v", err)
	}
	if len(specialists) == 0 {
		t.Fatal("expected specialists")
	}
	for _, item := range specialists {
		if item["default_model"] != "qwen3.7-max" {
			t.Fatalf("default_model = %v, want qwen3.7-max in %#v", item["default_model"], item)
		}
		if item["effective_model"] != "qwen3.7-max" {
			t.Fatalf("effective_model = %v, want qwen3.7-max in %#v", item["effective_model"], item)
		}
		if _, ok := item["configured_model"]; ok {
			t.Fatalf("configured_model should be omitted without override: %#v", item)
		}
	}
}

func TestIntegration_EmployeeSpecialists_UpdateModelOverride(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)

	rr := h.patchEmployeeSpecialist(t, m, emp.ID, "software-engineering-specialist", map[string]any{"model": "gpt-5.4-mini"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if resp["configured_model"] != "gpt-5.4-mini" || resp["effective_model"] != "gpt-5.4-mini" {
		t.Fatalf("model response mismatch: %#v", resp)
	}

	var stored model.Employee
	if err := h.db.First(&stored, "id = ?", emp.ID).Error; err != nil {
		t.Fatalf("reload employee: %v", err)
	}
	if got := employeeruntime.SpecialistModelOverride(stored.RuntimeConfig, "software-engineering-specialist"); got != "gpt-5.4-mini" {
		t.Fatalf("stored override = %q, want gpt-5.4-mini", got)
	}

	rr = h.patchEmployeeSpecialist(t, m, emp.ID, "software-engineering-specialist", map[string]any{"model": nil})
	if rr.Code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	resp = map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode clear response: %v", err)
	}
	if _, ok := resp["configured_model"]; ok {
		t.Fatalf("configured_model should be omitted after clear: %#v", resp)
	}
	if resp["effective_model"] != "qwen3.7-max" {
		t.Fatalf("effective_model = %v, want qwen3.7-max", resp["effective_model"])
	}
}

func TestIntegration_EmployeeSpecialists_UpdateRejectsInvalidModel(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)

	rr := h.patchEmployeeSpecialist(t, m, emp.ID, "software-engineering-specialist", map[string]any{"model": "not-a-model"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}
