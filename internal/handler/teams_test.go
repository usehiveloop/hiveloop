package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type teamsHarness struct {
	db     *gorm.DB
	router *chi.Mux
}

func newTeamsHarness(t *testing.T) *teamsHarness {
	t.Helper()
	db := connectTestDB(t)
	teamHandler := handler.NewTeamHandler(db)

	r := chi.NewRouter()
	r.Route("/v1/teams", func(r chi.Router) {
		r.Get("/", teamHandler.List)
		r.Get("/{id}", teamHandler.Get)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/", teamHandler.Create)
			r.Patch("/{id}", teamHandler.Update)
			r.Delete("/{id}", teamHandler.Delete)
		})
	})

	return &teamsHarness{db: db, router: r}
}

func (h *teamsHarness) createOrg(t *testing.T, role string) (model.Org, model.User) {
	t.Helper()
	suffix := uuid.New().String()[:8]
	user := model.User{Email: "team-" + suffix + "@test.com", Name: "Team Test"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Team Org " + suffix, Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: role}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *teamsHarness) addMember(t *testing.T, orgID uuid.UUID, role string) model.User {
	t.Helper()
	suffix := uuid.New().String()[:8]
	user := model.User{Email: "team-extra-" + suffix + "@test.com", Name: "Extra"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create extra user: %v", err)
	}
	membership := model.OrgMembership{UserID: user.ID, OrgID: orgID, Role: role}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create extra membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user
}

func (h *teamsHarness) request(t *testing.T, method, path string, user model.User, org model.Org, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		reqBody = new(bytes.Buffer)
		if err := json.NewEncoder(reqBody).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	} else {
		reqBody = new(bytes.Buffer)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	claims := &auth.AuthClaims{UserID: user.ID.String(), OrgID: org.ID.String(), Role: "member"}
	req = middleware.WithAuthClaims(req, claims)
	orgCopy := org
	req = middleware.WithOrg(req, &orgCopy)

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func decodeTeam(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func TestTeamsCreate_AdminSucceeds(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	rr := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{
		"name":        "Engineering",
		"description": "Builds the product.",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	body := decodeTeam(t, rr)
	if body["name"] != "Engineering" {
		t.Errorf("name = %v, want Engineering", body["name"])
	}
	if body["description"] != "Builds the product." {
		t.Errorf("description = %v, want 'Builds the product.'", body["description"])
	}
	if _, ok := body["id"].(string); !ok {
		t.Errorf("missing id in response: %v", body)
	}

	var stored model.Team
	if err := h.db.Where("org_id = ? AND name = ?", org.ID, "Engineering").First(&stored).Error; err != nil {
		t.Fatalf("team not in db: %v", err)
	}
	if stored.OrgID != org.ID {
		t.Errorf("stored.OrgID = %v, want %v", stored.OrgID, org.ID)
	}
}

func TestTeamsCreate_OwnerSucceeds(t *testing.T) {
	h := newTeamsHarness(t)
	org, owner := h.createOrg(t, "owner")

	rr := h.request(t, "POST", "/v1/teams", owner, org, map[string]string{"name": "Owners can create"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsCreate_MemberForbidden(t *testing.T) {
	h := newTeamsHarness(t)
	org, _ := h.createOrg(t, "admin")
	member := h.addMember(t, org.ID, "member")

	rr := h.request(t, "POST", "/v1/teams", member, org, map[string]string{"name": "Nope"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for member, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsCreate_ViewerForbidden(t *testing.T) {
	h := newTeamsHarness(t)
	org, _ := h.createOrg(t, "admin")
	viewer := h.addMember(t, org.ID, "viewer")

	rr := h.request(t, "POST", "/v1/teams", viewer, org, map[string]string{"name": "Nope"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsCreate_RejectsEmptyName(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	rr := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{"name": "   "})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace name, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsCreate_RejectsDuplicateNameSameOrg(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	first := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{"name": "Engineering"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create failed: %d", first.Code)
	}

	second := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{"name": "Engineering"})
	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate, got %d: %s", second.Code, second.Body.String())
	}
}

func TestTeamsCreate_AllowsSameNameAcrossDifferentOrgs(t *testing.T) {
	h := newTeamsHarness(t)
	orgA, adminA := h.createOrg(t, "admin")
	orgB, adminB := h.createOrg(t, "admin")

	rr := h.request(t, "POST", "/v1/teams", adminA, orgA, map[string]string{"name": "Engineering"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("orgA create failed: %d", rr.Code)
	}
	rr = h.request(t, "POST", "/v1/teams", adminB, orgB, map[string]string{"name": "Engineering"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("orgB create failed (should allow same name in different org): %d", rr.Code)
	}
}

func TestTeamsCreate_TrimsWhitespace(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	rr := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{
		"name":        "  Engineering  ",
		"description": "   builds things   ",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	body := decodeTeam(t, rr)
	if body["name"] != "Engineering" {
		t.Errorf("name not trimmed: %v", body["name"])
	}
	if body["description"] != "builds things" {
		t.Errorf("description not trimmed: %v", body["description"])
	}
}

func TestTeamsList_ReturnsOnlyOwnOrg(t *testing.T) {
	h := newTeamsHarness(t)
	orgA, adminA := h.createOrg(t, "admin")
	orgB, adminB := h.createOrg(t, "admin")

	mustCreateTeam(t, h, adminA, orgA, "Platform")
	mustCreateTeam(t, h, adminA, orgA, "Growth")
	mustCreateTeam(t, h, adminB, orgB, "Sales")

	rr := h.request(t, "GET", "/v1/teams", adminA, orgA, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 teams, got %d: %v", len(resp.Data), resp.Data)
	}
	for _, row := range resp.Data {
		name := row["name"].(string)
		if name != "Platform" && name != "Growth" {
			t.Errorf("unexpected team in orgA list: %s", name)
		}
	}
}

func TestTeamsList_MemberCanRead(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	member := h.addMember(t, org.ID, "member")

	mustCreateTeam(t, h, admin, org, "Platform")

	rr := h.request(t, "GET", "/v1/teams", member, org, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for member read, got %d", rr.Code)
	}
}

func TestTeamsList_ExcludesSoftDeleted(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Doomed")

	now := time.Now()
	if err := h.db.Model(&model.Team{}).Where("id = ?", team.ID).Update("deleted_at", &now).Error; err != nil {
		t.Fatalf("soft-delete team: %v", err)
	}

	rr := h.request(t, "GET", "/v1/teams", admin, org, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Fatalf("soft-deleted team leaked into list: %v", resp.Data)
	}
}

func TestTeamsGet_OwnOrgReturnsTeam(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Platform")

	rr := h.request(t, "GET", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := decodeTeam(t, rr)
	if body["name"] != "Platform" {
		t.Errorf("got name = %v, want Platform", body["name"])
	}
}

func TestTeamsGet_OtherOrgReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	orgA, adminA := h.createOrg(t, "admin")
	orgB, adminB := h.createOrg(t, "admin")

	team := mustCreateTeam(t, h, adminA, orgA, "Secret")

	rr := h.request(t, "GET", "/v1/teams/"+team.ID.String(), adminB, orgB, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-org, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsGet_SoftDeletedReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Gone")

	now := time.Now()
	h.db.Model(&model.Team{}).Where("id = ?", team.ID).Update("deleted_at", &now)

	rr := h.request(t, "GET", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for soft-deleted, got %d", rr.Code)
	}
}

func TestTeamsGet_InvalidUUIDReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	rr := h.request(t, "GET", "/v1/teams/not-a-uuid", admin, org, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed id, got %d", rr.Code)
	}
}

func TestTeamsGet_MemberCanRead(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	member := h.addMember(t, org.ID, "member")
	team := mustCreateTeam(t, h, admin, org, "Open")

	rr := h.request(t, "GET", "/v1/teams/"+team.ID.String(), member, org, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for member, got %d", rr.Code)
	}
}

func TestTeamsUpdate_AdminCanRename(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Old name")

	newName := "New name"
	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), admin, org, map[string]any{
		"name": newName,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.Name != newName {
		t.Errorf("stored name = %q, want %q", stored.Name, newName)
	}
}

func TestTeamsUpdate_AdminCanUpdateDescription(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Platform")

	desc := "Builds and operates the platform."
	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), admin, org, map[string]any{
		"description": desc,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.Description != desc {
		t.Errorf("stored description = %q, want %q", stored.Description, desc)
	}
	if stored.Name != "Platform" {
		t.Errorf("name should not have changed: got %q", stored.Name)
	}
}

func TestTeamsUpdate_MemberForbidden(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	member := h.addMember(t, org.ID, "member")
	team := mustCreateTeam(t, h, admin, org, "Off-limits")

	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), member, org, map[string]any{
		"name": "Hijacked",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.Name == "Hijacked" {
		t.Fatal("team was renamed despite 403")
	}
}

func TestTeamsUpdate_RejectsEmptyName(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Keep me")

	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), admin, org, map[string]any{
		"name": "   ",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace name, got %d", rr.Code)
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.Name != "Keep me" {
		t.Errorf("name should not have changed: got %q", stored.Name)
	}
}

func TestTeamsUpdate_RejectsDuplicateNameSameOrg(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	mustCreateTeam(t, h, admin, org, "Engineering")
	growth := mustCreateTeam(t, h, admin, org, "Growth")

	rr := h.request(t, "PATCH", "/v1/teams/"+growth.ID.String(), admin, org, map[string]any{
		"name": "Engineering",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 renaming to existing name, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamsUpdate_OtherOrgReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	orgA, adminA := h.createOrg(t, "admin")
	orgB, adminB := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, adminA, orgA, "OrgA team")

	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), adminB, orgB, map[string]any{
		"name": "Hijacked",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-org, got %d", rr.Code)
	}
}

func TestTeamsUpdate_NoOpReturnsCurrentTeam(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Stable")

	rr := h.request(t, "PATCH", "/v1/teams/"+team.ID.String(), admin, org, map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := decodeTeam(t, rr)
	if body["name"] != "Stable" {
		t.Errorf("name = %v, want Stable", body["name"])
	}
}

func TestTeamsDelete_AdminSoftDeletes(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Doomed")

	rr := h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var stored model.Team
	if err := h.db.Where("id = ?", team.ID).First(&stored).Error; err != nil {
		t.Fatalf("team should still exist after soft-delete: %v", err)
	}
	if stored.DeletedAt == nil {
		t.Error("deleted_at not set")
	}
}

func TestTeamsDelete_HiddenAfterFromListAndGet(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Vanishing")

	h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), admin, org, nil)

	rr := h.request(t, "GET", "/v1/teams", admin, org, nil)
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	for _, row := range resp.Data {
		if row["id"] == team.ID.String() {
			t.Fatal("deleted team still in list")
		}
	}

	rr = h.request(t, "GET", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rr.Code)
	}
}

func TestTeamsDelete_DoubleDeleteReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, admin, org, "Once")

	first := h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first delete failed: %d", first.Code)
	}
	second := h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), admin, org, nil)
	if second.Code != http.StatusNotFound {
		t.Fatalf("second delete should be 404, got %d", second.Code)
	}
}

func TestTeamsDelete_MemberForbidden(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")
	member := h.addMember(t, org.ID, "member")
	team := mustCreateTeam(t, h, admin, org, "Stays")

	rr := h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), member, org, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.DeletedAt != nil {
		t.Fatal("team was deleted despite 403")
	}
}

func TestTeamsDelete_OtherOrgReturns404(t *testing.T) {
	h := newTeamsHarness(t)
	orgA, adminA := h.createOrg(t, "admin")
	orgB, adminB := h.createOrg(t, "admin")
	team := mustCreateTeam(t, h, adminA, orgA, "OrgA")

	rr := h.request(t, "DELETE", "/v1/teams/"+team.ID.String(), adminB, orgB, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 cross-org, got %d", rr.Code)
	}

	var stored model.Team
	h.db.Where("id = ?", team.ID).First(&stored)
	if stored.DeletedAt != nil {
		t.Fatal("team was deleted by foreign org admin")
	}
}

// Guards the partial unique index — without WHERE deleted_at IS NULL this
// would 409.
func TestTeamsDelete_AllowsReuseOfNameAfterDelete(t *testing.T) {
	h := newTeamsHarness(t)
	org, admin := h.createOrg(t, "admin")

	first := mustCreateTeam(t, h, admin, org, "Recyclable")
	h.request(t, "DELETE", "/v1/teams/"+first.ID.String(), admin, org, nil)

	rr := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{"name": "Recyclable"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("could not reuse name after delete: %d %s", rr.Code, rr.Body.String())
	}
}

func mustCreateTeam(t *testing.T, h *teamsHarness, admin model.User, org model.Org, name string) model.Team {
	t.Helper()
	rr := h.request(t, "POST", "/v1/teams", admin, org, map[string]string{"name": name})
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed team %q failed: %d %s", name, rr.Code, rr.Body.String())
	}
	body := decodeTeam(t, rr)
	id, err := uuid.Parse(body["id"].(string))
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	var team model.Team
	if err := h.db.Where("id = ?", id).First(&team).Error; err != nil {
		t.Fatalf("load team: %v", err)
	}
	return team
}
