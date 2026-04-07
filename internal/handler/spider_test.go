package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/handler"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/spider"
)

const spiderTestDBURL = "postgres://ziraloop:localdev@localhost:5433/ziraloop_test?sslmode=disable"

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

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = spiderTestDBURL
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to test database: %v", err)
	}
	if err := model.AutoMigrate(database); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	mockServer := httptest.NewServer(spiderHandler)
	t.Cleanup(mockServer.Close)

	spiderClient := spider.NewClient(mockServer.URL, "test-spider-key")
	usageWriter := middleware.NewToolUsageWriter(database, 1000)
	t.Cleanup(func() {
		usageWriter.Shutdown(t.Context())
	})

	spiderH := handler.NewSpiderHandler(spiderClient, usageWriter, database)

	// Create test org
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

	// Create test credential (needed for token foreign key)
	credID := uuid.New()
	cred := model.Credential{
		ID:           credID,
		OrgID:        orgID,
		ProviderID:   "openai",
		Label:        "test-cred",
		EncryptedKey: []byte("test-encrypted-key"),
		WrappedDEK:   []byte("test-wrapped-dek"),
	}
	if err := database.Create(&cred).Error; err != nil {
		t.Fatalf("create test credential: %v", err)
	}

	// Create test token with agent_id in meta
	agentID := uuid.New()
	tokenJTI := uuid.New().String()
	token := model.Token{
		ID:           uuid.New(),
		OrgID:        orgID,
		CredentialID: credID,
		JTI:          tokenJTI,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Meta:         model.JSON{"agent_id": agentID.String(), "type": "agent_proxy"},
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

	// Set up router with claims injection (simulating TokenAuth middleware)
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

// ---------- Crawl ----------

func TestSpiderCrawl_Success(t *testing.T) {
	var captured struct {
		Path string
		Body string
		Auth string
	}
	var mu sync.Mutex

	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured.Path = r.URL.Path
		captured.Body = string(body)
		captured.Auth = r.Header.Get("Authorization")
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "# Example", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL:          "https://example.com",
		ReturnFormat: "markdown",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var results []spider.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "# Example" {
		t.Fatalf("expected '# Example', got %q", results[0].Content)
	}

	mu.Lock()
	defer mu.Unlock()

	if captured.Path != "/v1/crawl" {
		t.Fatalf("expected spider path /v1/crawl, got %s", captured.Path)
	}
	if captured.Auth != "Bearer test-spider-key" {
		t.Fatalf("expected Bearer auth to spider, got %q", captured.Auth)
	}
}

func TestSpiderCrawl_MissingURL(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when URL is missing")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderCrawl_SpiderError(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ---------- Search ----------

func TestSpiderSearch_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spider.SearchResponse{
			Content: []spider.SearchResult{
				{Title: "Result 1", Description: "First result", URL: "https://example.com/1"},
				{Title: "Result 2", Description: "Second result", URL: "https://example.com/2"},
			},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/search", spider.SearchParams{
		Search: "test query",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	var results spider.SearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(results.Content) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results.Content))
	}
}

func TestSpiderSearch_MissingQuery(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when search is missing")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/search", spider.SearchParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ---------- Links ----------

func TestSpiderLinks_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "/about\n/contact", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/links", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ---------- Screenshot ----------

func TestSpiderScreenshot_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "base64-screenshot-data", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/screenshot", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ---------- Transform ----------

func TestSpiderTransform_Success(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "# Transformed", URL: "https://example.com", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/transform", spider.TransformParams{
		Data: []spider.TransformInput{
			{HTML: "<h1>Hello</h1>", URL: "https://example.com"},
		},
		ReturnFormat: "markdown",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSpiderTransform_EmptyData(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("spider API should not be called when data is empty")
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/transform", spider.TransformParams{})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", recorder.Code, recorder.Body.String())
	}
}

// ---------- Usage Tracking ----------

func TestSpiderCrawl_RecordsUsage(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{
			{Content: "Page 1", URL: "https://example.com/1", StatusCode: 200},
			{Content: "Page 2", URL: "https://example.com/2", StatusCode: 200},
		})
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", recorder.Code, recorder.Body.String())
	}

	// Flush the usage writer to ensure records are written
	harness.usageWriter.Shutdown(t.Context())

	// Wait briefly for the batch to flush
	time.Sleep(100 * time.Millisecond)

	// Query the database for the usage record
	var usages []model.ToolUsage
	if err := harness.db.Where("org_id = ? AND tool_name = ?", harness.orgID, "crawl").Find(&usages).Error; err != nil {
		t.Fatalf("query tool_usages: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usages))
	}

	usage := usages[0]
	if usage.OrgID != harness.orgID {
		t.Fatalf("expected org_id %s, got %s", harness.orgID, usage.OrgID)
	}
	if usage.ToolName != "crawl" {
		t.Fatalf("expected tool_name 'crawl', got %q", usage.ToolName)
	}
	if usage.Input != "https://example.com" {
		t.Fatalf("expected input 'https://example.com', got %q", usage.Input)
	}
	if usage.PagesReturned != 2 {
		t.Fatalf("expected pages_returned 2, got %d", usage.PagesReturned)
	}
	if usage.Status != "success" {
		t.Fatalf("expected status 'success', got %q", usage.Status)
	}
	if usage.TokenJTI != harness.tokenJTI {
		t.Fatalf("expected token_jti %q, got %q", harness.tokenJTI, usage.TokenJTI)
	}
	if usage.AgentID == "" {
		t.Fatal("expected agent_id to be set from token meta")
	}
}

func TestSpiderCrawl_RecordsErrorUsage(t *testing.T) {
	spiderAPI := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	})

	harness := newSpiderHarness(t, spiderAPI)

	recorder := harness.doRequest(t, "/v1/spider/crawl", spider.SpiderParams{
		URL: "https://example.com",
	})

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", recorder.Code)
	}

	// Flush the usage writer
	harness.usageWriter.Shutdown(t.Context())
	time.Sleep(100 * time.Millisecond)

	var usages []model.ToolUsage
	if err := harness.db.Where("org_id = ? AND tool_name = ?", harness.orgID, "crawl").Find(&usages).Error; err != nil {
		t.Fatalf("query tool_usages: %v", err)
	}

	if len(usages) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(usages))
	}

	if usages[0].Status != "error" {
		t.Fatalf("expected status 'error', got %q", usages[0].Status)
	}
	if usages[0].ErrorMessage == "" {
		t.Fatal("expected error_message to be set")
	}
}
