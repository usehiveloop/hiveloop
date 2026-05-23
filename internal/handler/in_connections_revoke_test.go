package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestInConnectionHandler_Revoke_Success(t *testing.T) {
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
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "revoke-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var conn model.InConnection
	db.Where("id = ?", connID).First(&conn)
	if conn.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}

	mockCfg.mu.Lock()
	foundDelete := false
	for _, m := range mockCfg.capturedMethods {
		if m == http.MethodDelete {
			foundDelete = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundDelete {
		t.Fatal("expected Nango to receive DELETE for connection")
	}
}

func TestInConnectionHandler_Revoke_NotFound(t *testing.T) {
	db := connectTestDB(t)
	nangoSrv := httptest.NewServer(newNangoConnMock(&nangoConnMockConfig{}))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+uuid.New().String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_WrongUser(t *testing.T) {
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
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user1 := createTestUser(t, db, fmt.Sprintf("user1-%s@test.com", uuid.New().String()[:8]))
	user2 := createTestUser(t, db, fmt.Sprintf("user2-%s@test.com", uuid.New().String()[:8]))
	org1 := createTestOrg(t, db)
	org2 := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org1.ID, UserID: user1.ID, InIntegrationID: integ.ID, NangoConnectionID: "user1-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user2)
	req = middleware.WithOrg(req, &org2)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_AlreadyRevoked(t *testing.T) {
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
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")
	now := time.Now()
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID,
		NangoConnectionID: "already-revoked", RevokedAt: &now,
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInConnectionHandler_Revoke_NangoFailure(t *testing.T) {
	db := connectTestDB(t)
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})

	mockCfg := &nangoConnMockConfig{deleteConnStatus: http.StatusInternalServerError}
	nangoSrv := httptest.NewServer(newNangoConnMock(mockCfg))
	t.Cleanup(nangoSrv.Close)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	_ = nangoClient.FetchProviders(context.Background())

	h := handler.NewInConnectionHandler(db, nangoClient, catalog.Global())
	r := chi.NewRouter()
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("conn-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "notion")
	connID := uuid.New()
	db.Create(&model.InConnection{
		ID: connID, OrgID: org.ID, UserID: user.ID, InIntegrationID: integ.ID, NangoConnectionID: "nango-fail-conn",
	})

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d: %s", rr.Code, rr.Body.String())
	}

	var conn model.InConnection
	db.Where("id = ?", connID).First(&conn)
	if conn.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set despite Nango failure")
	}
}

func TestInConnectionHandler_RevokeDetachesIntegrationManagedSkillWhenLastConnectionEnds(t *testing.T) {
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
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("revoke-managed-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	integ := createTestInIntegration(t, db, "linear")
	skill := createTestIntegrationManagedSkill(t, db, "revoke-linear-"+uuid.New().String()[:8], []string{"linear"})
	employee := createTestEmployee(t, db, org.ID)
	if err := db.Create(&model.AgentSkill{AgentID: employee.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	connID := uuid.New()
	if err := db.Create(&model.InConnection{
		ID:                connID,
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: "linear-revoke",
	}).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+connID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var count int64
	if err := db.Model(&model.AgentSkill{}).
		Where("agent_id = ? AND skill_id = ?", employee.ID, skill.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count attached skill: %v", err)
	}
	if count != 0 {
		t.Fatalf("integration-managed skill attachments = %d, want 0", count)
	}
}

func TestInConnectionHandler_RevokeKeepsSkillRequiredByAnotherActiveIntegration(t *testing.T) {
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
	r.Delete("/v1/in/connections/{id}", h.Revoke)

	user := createTestUser(t, db, fmt.Sprintf("revoke-shared-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, db)
	github := createTestInIntegration(t, db, "github-app")
	reviews := createTestInIntegration(t, db, "github-app-code-reviews")
	skill := createTestIntegrationManagedSkill(t, db, "shared-github-"+uuid.New().String()[:8], []string{"github-app", "github-app-code-reviews"})
	employee := createTestEmployee(t, db, org.ID)
	if err := db.Create(&model.AgentSkill{AgentID: employee.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	revokedConnID := uuid.New()
	for _, row := range []struct {
		id          uuid.UUID
		integration model.InIntegration
		nangoID     string
	}{
		{id: revokedConnID, integration: github, nangoID: "github-revoke"},
		{id: uuid.New(), integration: reviews, nangoID: "reviews-active"},
	} {
		if err := db.Create(&model.InConnection{
			ID:                row.id,
			OrgID:             org.ID,
			UserID:            user.ID,
			InIntegrationID:   row.integration.ID,
			NangoConnectionID: row.nangoID,
		}).Error; err != nil {
			t.Fatalf("create connection: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/in/connections/"+revokedConnID.String(), nil)
	req = middleware.WithUser(req, &user)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
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

func createTestEmployee(t *testing.T, db *gorm.DB, orgID uuid.UUID) model.Agent {
	t.Helper()
	employee := model.Agent{
		ID:          uuid.New(),
		OrgID:       &orgID,
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
	t.Cleanup(func() {
		db.Where("agent_id = ?", employee.ID).Delete(&model.AgentSkill{})
		db.Where("id = ?", employee.ID).Delete(&model.Agent{})
	})
	return employee
}
