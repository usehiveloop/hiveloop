package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/spider"
	"github.com/usehivy/hivy/internal/testdb"
)

const spiderTestDBURL = testdb.DefaultDatabaseURL

type spiderTestHarness struct {
	db          *gorm.DB
	router      *chi.Mux
	mockSpider  *httptest.Server
	usageWriter *middleware.ToolUsageWriter
	orgID       uuid.UUID
	tokenJTI    string
}

func newSpiderHarness(t *testing.T, spiderHandler http.Handler) *spiderTestHarness {
	t.Helper()

	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to test database: %v", err)
	}
	testdb.ApplyMigrations(t, database)

	mockServer := httptest.NewServer(spiderHandler)
	t.Cleanup(mockServer.Close)

	spiderClient := spider.NewClient(mockServer.URL, "test-spider-key")
	usageWriter := middleware.NewToolUsageWriter(t.Context(), database, 1000)
	t.Cleanup(func() {
		usageWriter.Shutdown(t.Context())
	})

	spiderH := handler.NewSpiderHandler(spiderClient, usageWriter, database)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("spider-test-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := database.Create(&org).Error; err != nil {
		t.Fatalf("create test org: %v", err)
	}

	credID := uuid.New()
	cred := model.Credential{
		ID:           credID,
		OrgID:        &orgID,
		ProviderID:   "openai",
		Label:        "test-cred",
		EncryptedKey: []byte("test-encrypted-key"),
		WrappedDEK:   []byte("test-wrapped-dek"),
	}
	if err := database.Create(&cred).Error; err != nil {
		t.Fatalf("create test credential: %v", err)
	}

	agentID := uuid.New()
	tokenJTI := uuid.New().String()
	token := model.Token{
		ID:           uuid.New(),
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          tokenJTI,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Meta:         model.JSON{"employee_id": agentID.String(), "type": "employee_proxy"},
	}
	if err := database.Create(&token).Error; err != nil {
		t.Fatalf("create test token: %v", err)
	}

	t.Cleanup(func() {
		database.Where("org_id = ?", orgID).Delete(&model.ToolUsage{})
		database.Where("org_id = ?", orgID).Delete(&model.Token{})
		database.Where("org_id = ?", orgID).Delete(&model.Credential{})
		database.Where("id = ?", orgID).Delete(&model.Org{})
	})

	router := chi.NewRouter()
	router.Route("/v1/spider", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims := &middleware.TokenClaims{
					OrgID:        orgID.String(),
					CredentialID: credID.String(),
					JTI:          tokenJTI,
				}
				next.ServeHTTP(w, middleware.WithClaims(r, claims))
			})
		})
		r.Post("/crawl", spiderH.Crawl)
		r.Post("/search", spiderH.Search)
		r.Post("/links", spiderH.Links)
		r.Post("/screenshot", spiderH.Screenshot)
		r.Post("/transform", spiderH.Transform)
	})

	return &spiderTestHarness{
		db:          database,
		router:      router,
		mockSpider:  mockServer,
		usageWriter: usageWriter,
		orgID:       orgID,
		tokenJTI:    tokenJTI,
	}
}

func (harness *spiderTestHarness) doRequest(t *testing.T, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	jsonBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)
	return recorder
}
