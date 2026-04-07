package handler

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/spider"
)

// SpiderHandler proxies web crawling/search/screenshot requests to Spider.cloud
// and records every call for per-org, per-agent billing.
type SpiderHandler struct {
	spider      *spider.Client
	usageWriter *middleware.ToolUsageWriter
	db          *gorm.DB
}

// NewSpiderHandler creates a spider handler.
func NewSpiderHandler(spiderClient *spider.Client, usageWriter *middleware.ToolUsageWriter, db *gorm.DB) *SpiderHandler {
	return &SpiderHandler{
		spider:      spiderClient,
		usageWriter: usageWriter,
		db:          db,
	}
}

// Crawl handles POST /v1/spider/crawl.
func (handler *SpiderHandler) Crawl(w http.ResponseWriter, r *http.Request) {
	var params spider.SpiderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if params.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	start := time.Now()
	results, err := handler.spider.Crawl(r.Context(), params)
	duration := time.Since(start)

	handler.recordUsage(r, "crawl", params.URL, results, err, duration)

	if err != nil {
		slog.Error("spider crawl failed", "error", err, "url", params.URL)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "spider crawl failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// Search handles POST /v1/spider/search.
func (handler *SpiderHandler) Search(w http.ResponseWriter, r *http.Request) {
	var params spider.SearchParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if params.Search == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "search is required"})
		return
	}

	start := time.Now()
	results, err := handler.spider.Search(r.Context(), params)
	duration := time.Since(start)

	handler.recordUsage(r, "search", params.Search, results, err, duration)

	if err != nil {
		slog.Error("spider search failed", "error", err, "query", params.Search)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "spider search failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// Links handles POST /v1/spider/links.
func (handler *SpiderHandler) Links(w http.ResponseWriter, r *http.Request) {
	var params spider.SpiderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if params.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	start := time.Now()
	results, err := handler.spider.Links(r.Context(), params)
	duration := time.Since(start)

	handler.recordUsage(r, "links", params.URL, results, err, duration)

	if err != nil {
		slog.Error("spider links failed", "error", err, "url", params.URL)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "spider links failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// Screenshot handles POST /v1/spider/screenshot.
func (handler *SpiderHandler) Screenshot(w http.ResponseWriter, r *http.Request) {
	var params spider.SpiderParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if params.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	start := time.Now()
	results, err := handler.spider.Screenshot(r.Context(), params)
	duration := time.Since(start)

	handler.recordUsage(r, "screenshot", params.URL, results, err, duration)

	if err != nil {
		slog.Error("spider screenshot failed", "error", err, "url", params.URL)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "spider screenshot failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// Transform handles POST /v1/spider/transform.
func (handler *SpiderHandler) Transform(w http.ResponseWriter, r *http.Request) {
	var params spider.TransformParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if len(params.Data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "data is required"})
		return
	}

	// Use the first item's URL as the input for tracking
	input := ""
	if len(params.Data) > 0 && params.Data[0].URL != "" {
		input = params.Data[0].URL
	}

	start := time.Now()
	results, err := handler.spider.Transform(r.Context(), params)
	duration := time.Since(start)

	handler.recordUsage(r, "transform", input, results, err, duration)

	if err != nil {
		slog.Error("spider transform failed", "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "spider transform failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// recordUsage builds a ToolUsage record and queues it for async writing.
func (handler *SpiderHandler) recordUsage(r *http.Request, toolName, input string, results []spider.Response, callErr error, duration time.Duration) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return
	}

	orgID, _ := uuid.Parse(claims.OrgID)
	agentID := handler.lookupAgentID(claims.JTI)

	status := "success"
	errorMessage := ""
	if callErr != nil {
		status = "error"
		errorMessage = callErr.Error()
		if len(errorMessage) > 1000 {
			errorMessage = errorMessage[:1000]
		}
	}

	usage := model.ToolUsage{
		ID:            "tu_" + ulid.Make().String(),
		OrgID:         orgID,
		AgentID:       agentID,
		TokenJTI:      claims.JTI,
		ToolName:      toolName,
		Input:         truncateInput(input, 2000),
		PagesReturned: len(results),
		Status:        status,
		ErrorMessage:  errorMessage,
		TotalMs:       int(duration.Milliseconds()),
		CreditsUsed:   len(results), // 1 credit per page returned
		CreatedAt:     time.Now().UTC(),
	}

	if ipAddr, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		usage.IPAddress = &ipAddr
	} else {
		addr := r.RemoteAddr
		usage.IPAddress = &addr
	}

	handler.usageWriter.Write(usage)
}

// lookupAgentID extracts the agent_id from the token's meta JSONB field.
func (handler *SpiderHandler) lookupAgentID(jti string) string {
	var token model.Token
	if err := handler.db.Select("meta").Where("jti = ?", jti).First(&token).Error; err != nil {
		return ""
	}
	if token.Meta == nil {
		return ""
	}
	if agentID, ok := token.Meta["agent_id"].(string); ok {
		return agentID
	}
	return ""
}

func truncateInput(input string, maxLen int) string {
	if len(input) <= maxLen {
		return input
	}
	return input[:maxLen]
}
