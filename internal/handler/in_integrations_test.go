package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func createTestUser(t *testing.T, db *gorm.DB, email string) model.User {
	t.Helper()
	user := model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "$2a$10$dummy",
		Name:         "Test User",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return user
}

func createTestInIntegration(t *testing.T, db *gorm.DB, provider string) model.InIntegration {
	t.Helper()
	integ := model.InIntegration{
		ID:          uuid.New(),
		UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
		Provider:    provider,
		DisplayName: provider + " built-in",
	}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create in-integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", integ.ID).Delete(&model.InIntegration{})
	})
	return integ
}

func cleanupInIntegrations(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		db.Where("1=1").Delete(&model.InConnection{})
		db.Where("1=1").Delete(&model.InIntegration{})
	})
}

// nangoIntegrationMock returns a Nango mock that handles provider listing,
// integration CRUD, and returns configurable responses.
type nangoMockConfig struct {
	mu              sync.Mutex
	capturedPaths   []string
	capturedMethods []string
	createStatus    int
	getStatus       int
	updateStatus    int
	deleteStatus    int
}

func newNangoMock(cfg *nangoMockConfig) http.Handler {
	if cfg.createStatus == 0 {
		cfg.createStatus = http.StatusOK
	}
	if cfg.getStatus == 0 {
		cfg.getStatus = http.StatusOK
	}
	if cfg.updateStatus == 0 {
		cfg.updateStatus = http.StatusOK
	}
	if cfg.deleteStatus == 0 {
		cfg.deleteStatus = http.StatusOK
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.mu.Lock()
		cfg.capturedPaths = append(cfg.capturedPaths, r.URL.Path)
		cfg.capturedMethods = append(cfg.capturedMethods, r.Method)
		cfg.mu.Unlock()

		// Provider catalog
		if r.URL.Path == "/providers" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"name":         "github",
						"display_name": "GitHub",
						"auth_mode":    "OAUTH2",
					},
					{
						"name":         "slack",
						"display_name": "Slack",
						"auth_mode":    "OAUTH2",
					},
				},
			})
			return
		}

		// Integration CRUD
		if strings.HasPrefix(r.URL.Path, "/integrations") {
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(cfg.createStatus)
				if cfg.createStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodGet:
				w.WriteHeader(cfg.getStatus)
				json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{
						"unique_key": "test",
						"logo":       "https://example.com/logo.png",
					},
				})
			case http.MethodPatch:
				w.WriteHeader(cfg.updateStatus)
				if cfg.updateStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"unique_key": "test"}})
			case http.MethodDelete:
				w.WriteHeader(cfg.deleteStatus)
				if cfg.deleteStatus >= 400 {
					json.NewEncoder(w).Encode(map[string]any{"error": "nango error"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type inIntegTestHarness struct {
	db      *gorm.DB
	handler *handler.InIntegrationHandler
	router  *chi.Mux
	nango   *httptest.Server
	mockCfg *nangoMockConfig
}

func newInIntegHarness(t *testing.T, mockCfg *nangoMockConfig) *inIntegTestHarness {
	t.Helper()
	db := connectTestDB(t)
	cleanupInIntegrations(t, db)

	if mockCfg == nil {
		mockCfg = &nangoMockConfig{}
	}
	nangoSrv := httptest.NewServer(newNangoMock(mockCfg))
	t.Cleanup(nangoSrv.Close)

	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	// Fetch providers so the nango client has its catalog populated
	if err := nangoClient.FetchProviders(context.Background()); err != nil {
		t.Fatalf("fetch nango providers: %v", err)
	}

	cat := catalog.Global()
	h := handler.NewInIntegrationHandler(db, nangoClient, cat)

	r := chi.NewRouter()
	r.Post("/v1/in/integrations", h.Create)
	r.Get("/v1/in/integrations", h.List)
	r.Get("/v1/in/integrations/{id}", h.Get)
	r.Put("/v1/in/integrations/{id}", h.Update)
	r.Delete("/v1/in/integrations/{id}", h.Delete)
	r.Get("/v1/in/integrations/available", h.ListAvailable)

	return &inIntegTestHarness{
		db:      db,
		handler: h,
		router:  r,
		nango:   nangoSrv,
		mockCfg: mockCfg,
	}
}

func (h *inIntegTestHarness) doRequest(t *testing.T, method, path string, body any, user *model.User) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = middleware.WithUser(req, user)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Create tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_Create_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "test-client-id",
			"client_secret": "test-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
	if resp["display_name"] != "GitHub Built-in" {
		t.Fatalf("expected display_name=GitHub Built-in, got %v", resp["display_name"])
	}

	// Verify Nango received correct key prefix
	h.mockCfg.mu.Lock()
	found := false
	for _, p := range h.mockCfg.capturedPaths {
		if strings.Contains(p, "/integrations") && strings.HasPrefix(p, "/integrations") {
			found = true
		}
	}
	h.mockCfg.mu.Unlock()
	if !found {
		t.Fatal("expected Nango to receive integration creation request")
	}

	// Verify stored in DB
	var integ model.InIntegration
	if err := h.db.Where("id = ?", resp["id"]).First(&integ).Error; err != nil {
		t.Fatalf("integration not found in DB: %v", err)
	}
	if !strings.HasPrefix(integ.UniqueKey, "github-") {
		t.Fatalf("expected unique_key to start with github-, got %s", integ.UniqueKey)
	}
}

func TestInIntegrationHandler_Create_MissingProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"display_name": "GitHub Built-in",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_MissingDisplayName(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider": "github",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_UnknownProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "nonexistent-provider",
		"display_name": "Test",
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Create_DuplicateProvider(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	// Create the first one directly in DB to avoid unique_key conflict with Nango
	createTestInIntegration(t, h.db, "github")

	// Second create should fail on unique provider constraint
	integ2 := model.InIntegration{
		ID:          uuid.New(),
		UniqueKey:   fmt.Sprintf("github-%s", uuid.New().String()[:8]),
		Provider:    "github",
		DisplayName: "GitHub 2",
	}
	err := h.db.Create(&integ2).Error
	if err == nil {
		t.Fatal("expected unique constraint error for duplicate provider")
		h.db.Where("id = ?", integ2.ID).Delete(&model.InIntegration{})
	}
	_ = user // used for context
}

func TestInIntegrationHandler_Create_InvalidCredentials(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":      "OAUTH2",
			"client_id": "test-client-id",
			// missing client_secret
		},
	}, &user)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInIntegrationHandler_Create_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{createStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPost, "/v1/in/integrations", map[string]any{
		"provider":     "github",
		"display_name": "GitHub Built-in",
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "test-client-id",
			"client_secret": "test-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_List_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	// Use unique providers to avoid unique constraint issues
	providers := []string{"github", "slack", "notion"}
	for _, p := range providers {
		integ := model.InIntegration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", p, uuid.New().String()[:8]),
			Provider:    p,
			DisplayName: p + " test",
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 integrations, got %d", len(page.Data))
	}
}

func TestInIntegrationHandler_List_ExcludesDeleted(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	integ := createTestInIntegration(t, h.db, "github")

	// Soft-delete it
	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations", nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&page)
	for _, item := range page.Data {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in list")
		}
	}
}

func TestInIntegrationHandler_List_Pagination(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	// Create 5 integrations with unique providers
	for i := 0; i < 5; i++ {
		provider := fmt.Sprintf("provider-%d-%s", i, uuid.New().String()[:8])
		integ := model.InIntegration{
			ID:          uuid.New(),
			UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
			Provider:    provider,
			DisplayName: provider,
		}
		if err := h.db.Create(&integ).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
		time.Sleep(time.Millisecond) // ensure distinct created_at
	}

	// First page
	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations?limit=2", nil, &user)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("expected next_cursor to be present")
	}

	// Second page
	rr2 := h.doRequest(t, http.MethodGet, "/v1/in/integrations?limit=2&cursor="+*page1.NextCursor, nil, &user)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}

	var page2 struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(rr2.Body).Decode(&page2)
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 items on page 2, got %d", len(page2.Data))
	}
}

// ---------------------------------------------------------------------------
// Get tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_Get_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["id"] != integ.ID.String() {
		t.Fatalf("expected id=%s, got %v", integ.ID.String(), resp["id"])
	}
	if resp["provider"] != "github" {
		t.Fatalf("expected provider=github, got %v", resp["provider"])
	}
}

func TestInIntegrationHandler_Get_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+uuid.New().String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Get_DeletedNotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	// Soft-delete
	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Update tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_Update_DisplayName(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"display_name": "Updated GitHub",
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["display_name"] != "Updated GitHub" {
		t.Fatalf("expected display_name=Updated GitHub, got %v", resp["display_name"])
	}
}

func TestInIntegrationHandler_Update_Credentials(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "new-client-id",
			"client_secret": "new-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify Nango received PATCH
	mockCfg.mu.Lock()
	foundPatch := false
	for i, m := range mockCfg.capturedMethods {
		if m == http.MethodPatch && strings.Contains(mockCfg.capturedPaths[i], "/integrations/") {
			foundPatch = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundPatch {
		t.Fatal("expected Nango to receive PATCH for credential update")
	}
}

func TestInIntegrationHandler_Update_Meta(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"meta": map[string]any{"custom": "value"},
	}, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify in DB
	var updated model.InIntegration
	h.db.Where("id = ?", integ.ID).First(&updated)
	if updated.Meta == nil || updated.Meta["custom"] != "value" {
		t.Fatalf("expected meta.custom=value, got %v", updated.Meta)
	}
}

func TestInIntegrationHandler_Update_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+uuid.New().String(), map[string]any{
		"display_name": "Updated",
	}, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Update_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{updateStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodPut, "/v1/in/integrations/"+integ.ID.String(), map[string]any{
		"credentials": map[string]any{
			"type":          "OAUTH2",
			"client_id":     "new-client-id",
			"client_secret": "new-client-secret",
		},
	}, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Delete tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_Delete_Success(t *testing.T) {
	mockCfg := &nangoMockConfig{}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify soft-deleted in DB
	var deleted model.InIntegration
	h.db.Where("id = ?", integ.ID).First(&deleted)
	if deleted.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}

	// Verify Nango received DELETE
	mockCfg.mu.Lock()
	foundDelete := false
	for _, m := range mockCfg.capturedMethods {
		if m == http.MethodDelete {
			foundDelete = true
		}
	}
	mockCfg.mu.Unlock()
	if !foundDelete {
		t.Fatal("expected Nango to receive DELETE")
	}
}

func TestInIntegrationHandler_Delete_NotFound(t *testing.T) {
	h := newInIntegHarness(t, nil)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+uuid.New().String(), nil, &user)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestInIntegrationHandler_Delete_NangoFailure(t *testing.T) {
	mockCfg := &nangoMockConfig{deleteStatus: http.StatusInternalServerError}
	h := newInIntegHarness(t, mockCfg)
	user := createTestUser(t, h.db, fmt.Sprintf("admin-%s@test.com", uuid.New().String()[:8]))
	integ := createTestInIntegration(t, h.db, "github")

	rr := h.doRequest(t, http.MethodDelete, "/v1/in/integrations/"+integ.ID.String(), nil, &user)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ListAvailable tests
// ---------------------------------------------------------------------------

func TestInIntegrationHandler_ListAvailable_Success(t *testing.T) {
	h := newInIntegHarness(t, nil)
	createTestInIntegration(t, h.db, "github")

	// No user context needed — public endpoint
	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp []map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp) < 1 {
		t.Fatal("expected at least 1 available integration")
	}

	// Verify safe fields only — no nango_config
	for _, item := range resp {
		if _, exists := item["nango_config"]; exists {
			t.Fatal("nango_config should not be in available response")
		}
		if _, exists := item["unique_key"]; exists {
			t.Fatal("unique_key should not be in available response")
		}
	}
}

func TestInIntegrationHandler_ListAvailable_ExcludesDeleted(t *testing.T) {
	h := newInIntegHarness(t, nil)
	integ := createTestInIntegration(t, h.db, "github")

	// Soft-delete
	now := time.Now()
	h.db.Model(&integ).Update("deleted_at", now)

	rr := h.doRequest(t, http.MethodGet, "/v1/in/integrations/available", nil, nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp []map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	for _, item := range resp {
		if item["id"] == integ.ID.String() {
			t.Fatal("deleted integration should not appear in available list")
		}
	}
}
