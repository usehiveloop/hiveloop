package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// GenerationHandler serves generation records.
type GenerationHandler struct {
	db *gorm.DB
}

// NewGenerationHandler creates a new generation handler.
func NewGenerationHandler(db *gorm.DB) *GenerationHandler {
	return &GenerationHandler{db: db}
}

type generationResponse struct {
	ID              string   `json:"id"`
	OrgID           string   `json:"org_id"`
	CredentialID    string   `json:"credential_id"`
	IdentityID      *string  `json:"identity_id,omitempty"`
	TokenJTI        string   `json:"token_jti"`
	ProviderID      string   `json:"provider_id"`
	Model           string   `json:"model"`
	RequestPath     string   `json:"request_path"`
	IsStreaming     bool     `json:"is_streaming"`
	InputTokens     int      `json:"input_tokens"`
	OutputTokens    int      `json:"output_tokens"`
	CachedTokens    int      `json:"cached_tokens"`
	ReasoningTokens int      `json:"reasoning_tokens"`
	Cost            float64  `json:"cost"`
	TTFBMs          *int     `json:"ttfb_ms,omitempty"`
	TotalMs         int      `json:"total_ms"`
	UpstreamStatus  int      `json:"upstream_status"`
	UserID          string   `json:"user_id,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	ErrorType       string   `json:"error_type,omitempty"`
	ErrorMessage    string   `json:"error_message,omitempty"`
	IPAddress       *string  `json:"ip_address,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

func toGenerationResponse(g model.Generation) generationResponse {
	resp := generationResponse{
		ID:              g.ID,
		OrgID:           g.OrgID.String(),
		CredentialID:    g.CredentialID.String(),
		TokenJTI:        g.TokenJTI,
		ProviderID:      g.ProviderID,
		Model:           g.Model,
		RequestPath:     g.RequestPath,
		IsStreaming:     g.IsStreaming,
		InputTokens:     g.InputTokens,
		OutputTokens:    g.OutputTokens,
		CachedTokens:    g.CachedTokens,
		ReasoningTokens: g.ReasoningTokens,
		Cost:            g.Cost,
		TTFBMs:          g.TTFBMs,
		TotalMs:         g.TotalMs,
		UpstreamStatus:  g.UpstreamStatus,
		UserID:          g.UserID,
		Tags:            g.Tags,
		ErrorType:       g.ErrorType,
		ErrorMessage:    g.ErrorMessage,
		IPAddress:       g.IPAddress,
		CreatedAt:       g.CreatedAt.Format(time.RFC3339),
	}
	if g.IdentityID != nil {
		s := g.IdentityID.String()
		resp.IdentityID = &s
	}
	if resp.Tags == nil {
		resp.Tags = []string{}
	}
	return resp
}

// Get handles GET /v1/generations/{id}.
// @Summary Get a generation
// @Description Returns a single generation record by ID.
// @Tags generations
// @Produce json
// @Param id path string true "Generation ID"
// @Success 200 {object} generationResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/generations/{id} [get]
func (h *GenerationHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing generation id"})
		return
	}

	var gen model.Generation
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&gen).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "generation not found"})
		return
	}

	writeJSON(w, http.StatusOK, toGenerationResponse(gen))
}

// List handles GET /v1/generations.
// @Summary List generations
// @Description Returns generation records for the current organization with cursor pagination and filtering.
// @Tags generations
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Param model query string false "Filter by model name"
// @Param provider_id query string false "Filter by provider ID"
// @Param credential_id query string false "Filter by credential ID"
// @Param user_id query string false "Filter by user ID"
// @Param tags query string false "Filter by tag"
// @Param error_type query string false "Filter by error type"
// @Success 200 {object} paginatedResponse[generationResponse]
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/generations [get]
func (h *GenerationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ?", org.ID)

	// Filters
	if m := r.URL.Query().Get("model"); m != "" {
		q = q.Where("model = ?", m)
	}
	if p := r.URL.Query().Get("provider_id"); p != "" {
		q = q.Where("provider_id = ?", p)
	}
	if c := r.URL.Query().Get("credential_id"); c != "" {
		q = q.Where("credential_id = ?", c)
	}
	if u := r.URL.Query().Get("user_id"); u != "" {
		q = q.Where("user_id = ?", u)
	}
	if t := r.URL.Query().Get("tags"); t != "" {
		q = q.Where("tags @> ARRAY[?]::text[]", t)
	}
	if e := r.URL.Query().Get("error_type"); e != "" {
		q = q.Where("error_type = ?", e)
	}

	if cursor != nil {
		q = q.Where("(created_at, id) < (?, ?)", cursor.CreatedAt, cursor.ID.String())
	}
	q = q.Order("created_at DESC, id DESC").Limit(limit + 1)

	var gens []model.Generation
	if err := q.Find(&gens).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list generations"})
		return
	}

	hasMore := len(gens) > limit
	if hasMore {
		gens = gens[:limit]
	}

	items := make([]generationResponse, len(gens))
	for i, g := range gens {
		items[i] = toGenerationResponse(g)
	}

	resp := paginatedResponse[generationResponse]{
		Data:    items,
		HasMore: hasMore,
	}
	if hasMore && len(gens) > 0 {
		last := gens[len(gens)-1]
		// Use string cursor for generation IDs (not UUID)
		c := encodeCursor(last.CreatedAt, last.OrgID) // reuse org_id as placeholder for cursor encoding
		resp.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, resp)
}
