package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestInConnectionHandler_Create_Success(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp["in_integration_id"] != integ.ID.String() {
		t.Fatalf("expected in_integration_id=%s, got %v", integ.ID.String(), resp["in_integration_id"])
	}
	if resp["provider"] != "notion" {
		t.Fatalf("expected provider=notion, got %v", resp["provider"])
	}
	if resp["nango_connection_id"] != "nango-conn-123" {
		t.Fatalf("expected nango_connection_id=nango-conn-123, got %v", resp["nango_connection_id"])
	}

	var conn model.InConnection
	if err := db.Where("id = ?", resp["id"]).First(&conn).Error; err != nil {
		t.Fatalf("connection not found in DB: %v", err)
	}
	if conn.UserID != user.ID {
		t.Fatalf("expected user_id=%s, got %s", user.ID, conn.UserID)
	}
}

func TestInConnectionHandler_CreateSlackKeepsOnboardingOpenAndEnsuresHivy(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("slack-conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "slack")

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "slack-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	if err := db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload org: %v", err)
	}
	if reloaded.Onboarded {
		t.Fatal("org onboarded = true, want false until org profile update")
	}

	var employee model.Agent
	if err := db.Where("org_id = ? AND status <> ?", org.ID, "archived").First(&employee).Error; err != nil {
		t.Fatalf("load Hivy employee: %v", err)
	}
	if employee.ID == uuid.Nil {
		t.Fatal("Hivy employee was not created")
	}
}

func TestInConnectionHandler_CreateAttachesIntegrationManagedSkill(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("linear-conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "linear")
	skill := createTestIntegrationManagedSkill(t, db, "linear-managed-"+uuid.New().String()[:8], []string{"linear"})

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "linear-conn-123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var employee model.Agent
	if err := db.Where("org_id = ? AND status <> ?", org.ID, "archived").First(&employee).Error; err != nil {
		t.Fatalf("load Hivy employee: %v", err)
	}
	var count int64
	if err := db.Model(&model.AgentSkill{}).
		Where("agent_id = ? AND skill_id = ?", employee.ID, skill.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count attached skill: %v", err)
	}
	if count != 1 {
		t.Fatalf("integration-managed skill attachments = %d, want 1", count)
	}
}

func TestSkillHandler_DetachRejectsActiveIntegrationManagedSkill(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	user := createTestUser(t, db, fmt.Sprintf("detach-managed-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "linear")
	skill := createTestIntegrationManagedSkill(t, db, "locked-linear-"+uuid.New().String()[:8], []string{"linear"})
	employee := model.Agent{
		ID:          uuid.New(),
		OrgID:       &org.ID,
		Model:       "test-model",
		Status:      "active",
		Tools:       model.JSON{},
		McpServers:  model.JSON{},
		Skills:      model.JSON{},
		AgentConfig: model.JSON{},
		Permissions: model.JSON{},
		Resources:   model.JSON{},
	}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if err := db.Create(&model.AgentSkill{AgentID: employee.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	if err := db.Create(&model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "linear-active",
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	h := handler.NewSkillHandler(db, nil)
	r := chi.NewRouter()
	r.Delete("/v1/employees/{id}/skills/{skillID}", h.DetachFromEmployee)

	req := httptest.NewRequest(http.MethodDelete, "/v1/employees/"+employee.ID.String()+"/skills/"+skill.ID.String(), nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInConnectionHandler_Create_DuplicateUserIntegration(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")

	db.Create(&model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "first-conn",
	})

	body, _ := json.Marshal(map[string]any{"nango_connection_id": "second-conn"})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int64
	db.Model(&model.InConnection{}).Where("user_id = ? AND in_integration_id = ?", user.ID, integ.ID).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 connections, got %d", count)
	}
}

func TestInConnectionHandler_Create_WithMeta(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Post("/v1/in/integrations/{id}/connections", h.Create)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")

	body, _ := json.Marshal(map[string]any{
		"nango_connection_id": "nango-conn-meta",
		"meta":                map[string]any{"resources": map[string]any{"repos": []string{"hivy"}}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/in/integrations/"+integ.ID.String()+"/connections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	meta, ok := resp["meta"].(map[string]any)
	if !ok || meta["resources"] == nil {
		t.Fatalf("expected meta.resources to be set, got %v", resp["meta"])
	}
}
