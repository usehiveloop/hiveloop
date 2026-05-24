package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestSkillHandler_ListHidesIntegrationManagedPublicSkills(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	visible := createPublicSkillForList(t, db, "visible-public-"+uuid.NewString()[:8], nil, false)
	integrationManaged := createPublicSkillForList(t, db, "hidden-linear-"+uuid.NewString()[:8], []string{"linear"}, false)
	hidden := createPublicSkillForList(t, db, "hidden-public-"+uuid.NewString()[:8], nil, true)
	own := createOrgSkillForList(t, db, org.ID, "own-linear-"+uuid.NewString()[:8], []string{"linear"}, false)
	ownHidden := createOrgSkillForList(t, db, org.ID, "own-hidden-"+uuid.NewString()[:8], nil, true)

	h := handler.NewSkillHandler(db, nil)
	r := chi.NewRouter()
	r.Get("/v1/skills", h.List)

	for _, scope := range []string{"public", "all"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/skills?scope="+scope+"&limit=100", nil)
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("scope %s: expected 200, got %d: %s", scope, rr.Code, rr.Body.String())
		}
		names := decodeSkillListNames(t, rr.Body.Bytes())
		if names[integrationManaged.Name] {
			t.Fatalf("scope %s exposed integration-managed public skill %q", scope, integrationManaged.Name)
		}
		if names[hidden.Name] {
			t.Fatalf("scope %s exposed hidden public skill %q", scope, hidden.Name)
		}
		if !names[visible.Name] {
			t.Fatalf("scope %s did not include normal public skill %q", scope, visible.Name)
		}
		if scope == "all" && !names[own.Name] {
			t.Fatalf("scope all did not include org-owned skill %q", own.Name)
		}
		if scope == "public" && names[own.Name] {
			t.Fatalf("scope public included org-owned skill %q", own.Name)
		}
		if names[ownHidden.Name] {
			t.Fatalf("scope %s exposed hidden org-owned skill %q", scope, ownHidden.Name)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/skills?scope=own&limit=100", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("scope own: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	names := decodeSkillListNames(t, rr.Body.Bytes())
	if !names[ownHidden.Name] {
		t.Fatalf("scope own did not include hidden org-owned skill %q", ownHidden.Name)
	}
}

func TestSkillHandler_GetHidesHiddenPublicSkills(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	hidden := createPublicSkillForList(t, db, "hidden-detail-"+uuid.NewString()[:8], nil, true)

	h := handler.NewSkillHandler(db, nil)
	r := chi.NewRouter()
	r.Get("/v1/skills/{id}", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/skills/"+hidden.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected hidden public skill detail to be 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func createPublicSkillForList(t *testing.T, db *gorm.DB, name string, integrationIDs []string, hidden bool) model.Skill {
	t.Helper()
	return createSkillForList(t, db, nil, name, integrationIDs, hidden)
}

func createOrgSkillForList(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string, integrationIDs []string, hidden bool) model.Skill {
	t.Helper()
	return createSkillForList(t, db, &orgID, name, integrationIDs, hidden)
}

func createSkillForList(t *testing.T, db *gorm.DB, orgID *uuid.UUID, name string, integrationIDs []string, hidden bool) model.Skill {
	t.Helper()
	description := fmt.Sprintf("%s description", name)
	skill := model.Skill{
		ID:             uuid.New(),
		OrgID:          orgID,
		Slug:           model.GenerateSlug(name),
		Name:           name,
		Description:    &description,
		SourceType:     model.SkillSourceInline,
		RepoRef:        "main",
		Bundle:         model.RawJSON(`{"id":"test","title":"test","description":"test","content":"# Test"}`),
		IntegrationIDs: integrationIDs,
		Hidden:         hidden,
		Status:         model.SkillStatusPublished,
	}
	if err := db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", skill.ID).Delete(&model.Skill{})
	})
	return skill
}

func decodeSkillListNames(t *testing.T, body []byte) map[string]bool {
	t.Helper()
	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	names := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		names[item.Name] = true
	}
	return names
}
