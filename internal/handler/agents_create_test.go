package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

type agentCreateHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newAgentCreateHarness(t *testing.T) *agentCreateHarness {
	t.Helper()
	db := connectTestDB(t)
	agentHandler := handler.NewAgentHandler(db, registry.Global(), nil)

	r := chi.NewRouter()
	r.Route("/v1/agents", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/", agentHandler.Create)
	})

	return &agentCreateHarness{db: db, router: r}
}

func (h *agentCreateHarness) createOrgWithBYOK(t *testing.T, byok bool) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "agent-create-" + uuid.New().String()[:8] + "@test.com", Name: "Test"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Create Org " + uuid.New().String()[:8], Active: true, BYOK: byok}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "admin"}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *agentCreateHarness) post(t *testing.T, userID, orgID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/agents", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())

	claims := &auth.AuthClaims{UserID: userID.String(), OrgID: orgID.String(), Role: "admin"}
	req = middleware.WithAuthClaims(req, claims)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func decodeError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, rr.Body.String())
	}
	return resp["error"]
}

func TestAgentCreate_NameOnly_NonBYOK_Succeeds(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name": "name-only-" + uuid.New().String()[:8],
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentCreate_NameOnly_BYOK_Succeeds(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, true)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name": "byok-name-only-" + uuid.New().String()[:8],
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentCreate_MissingName_Rejected(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, true)

	rr := h.post(t, user.ID, org.ID, map[string]any{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "name is required") {
		t.Fatalf("unexpected error: %q", msg)
	}
}

func TestAgentCreate_NonBYOK_RejectsCredentialID(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":          "no-byok-cred-" + uuid.New().String()[:8],
		"credential_id": uuid.New().String(),
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "BYOK") {
		t.Fatalf("expected BYOK rejection, got: %q", msg)
	}
}

func TestAgentCreate_NonBYOK_AcceptsValidModel(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":  "no-byok-model-" + uuid.New().String()[:8],
		"model": "google/gemini-3-flash-preview",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentCreate_RejectsUnknownModel(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, false)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":  "unknown-model-" + uuid.New().String()[:8],
		"model": "totally-made-up-model-name",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "catalog") {
		t.Fatalf("expected catalog rejection, got: %q", msg)
	}
}

func TestAgentCreate_BYOK_UnknownCredential_Rejected(t *testing.T) {
	h := newAgentCreateHarness(t)
	org, user := h.createOrgWithBYOK(t, true)

	rr := h.post(t, user.ID, org.ID, map[string]any{
		"name":          "byok-bad-cred-" + uuid.New().String()[:8],
		"credential_id": uuid.New().String(),
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if msg := decodeError(t, rr); !strings.Contains(msg, "credential not found") {
		t.Fatalf("expected credential-not-found, got: %q", msg)
	}
}
