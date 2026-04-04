package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

// InIntegrationHandler manages CRUD for app-owned (built-in) integrations.
type InIntegrationHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

// NewInIntegrationHandler creates a new in-integration handler.
func NewInIntegrationHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *InIntegrationHandler {
	return &InIntegrationHandler{db: db, nango: nangoClient, catalog: cat}
}

type createInIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type updateInIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type inIntegrationResponse struct {
	ID          string     `json:"id"`
	UniqueKey   string     `json:"unique_key"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	NangoConfig model.JSON `json:"nango_config,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type inIntegrationAvailableResponse struct {
	ID          string     `json:"id"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	CreatedAt   string     `json:"created_at"`
}

func toInIntegrationResponse(integ model.InIntegration) inIntegrationResponse {
	return inIntegrationResponse{
		ID:          integ.ID.String(),
		UniqueKey:   integ.UniqueKey,
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		NangoConfig: integ.NangoConfig,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   integ.UpdatedAt.Format(time.RFC3339),
	}
}

func toInIntegrationAvailableResponse(integ model.InIntegration) inIntegrationAvailableResponse {
	return inIntegrationAvailableResponse{
		ID:          integ.ID.String(),
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
	}
}

// inNangoKey returns the Nango provider config key for an in-integration.
func inNangoKey(uniqueKey string) string {
	return "in_" + uniqueKey
}

// Create handles POST /v1/in/integrations.
func (h *InIntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	var req createInIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if req.DisplayName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name is required"})
		return
	}

	// Validate provider exists in Nango catalog
	provider, found := h.nango.GetProvider(req.Provider)
	if !found {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", req.Provider)})
		return
	}

	// Validate provider has action definitions in the catalog
	if _, ok := h.catalog.GetProvider(req.Provider); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not supported — no action definitions available", req.Provider)})
		return
	}

	// Validate credentials against provider's auth_mode
	if err := validateCredentials(provider, req.Credentials); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s", req.Provider, integID.String()[:8])

	// Push to Nango first
	nk := inNangoKey(uniqueKey)
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nk,
		Provider:    req.Provider,
		Credentials: req.Credentials,
	}
	slog.Info("creating in-integration in nango", "provider", req.Provider, "nango_key", nk)
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		slog.Error("nango in-integration creation failed", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create integration in Nango: " + err.Error()})
		return
	}

	// Fetch Nango config (best-effort)
	var nangoConfig model.JSON
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango in-integration details for config", "error", err, "nango_key", nk)
	} else {
		template, _ := h.nango.GetProviderTemplate(req.Provider)
		nangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	integ := model.InIntegration{
		ID:          integID,
		UniqueKey:   uniqueKey,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Meta:        req.Meta,
		NangoConfig: nangoConfig,
	}

	if err := h.db.Create(&integ).Error; err != nil {
		slog.Error("failed to store in-integration", "error", err, "provider", req.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration"})
		return
	}

	slog.Info("in-integration created", "integration_id", integ.ID, "provider", req.Provider)
	writeJSON(w, http.StatusCreated, toInIntegrationResponse(integ))
}

// List handles GET /v1/in/integrations.
func (h *InIntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("deleted_at IS NULL")

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}

	q = applyPagination(q, cursor, limit)

	var integrations []model.InIntegration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]inIntegrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toInIntegrationResponse(integ)
	}

	result := paginatedResponse[inIntegrationResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := integrations[len(integrations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/in/integrations/{id}.
func (h *InIntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango in-integration", "error", err, "integration_id", integ.ID)
	} else {
		template, _ := h.nango.GetProviderTemplate(integ.Provider)
		integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	writeJSON(w, http.StatusOK, toInIntegrationResponse(integ))
}

// Update handles PUT /v1/in/integrations/{id}.
func (h *InIntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req updateInIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Credentials != nil {
		provider, found := h.nango.GetProvider(integ.Provider)
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", integ.Provider)})
			return
		}
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		nk := inNangoKey(integ.UniqueKey)
		nangoReq := nango.UpdateIntegrationRequest{
			Credentials: req.Credentials,
		}
		slog.Info("updating nango in-integration credentials", "integration_id", integ.ID, "nango_key", nk)
		if err := h.nango.UpdateIntegration(r.Context(), nk, nangoReq); err != nil {
			slog.Error("nango in-integration update failed", "error", err, "integration_id", integ.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to update integration in Nango: " + err.Error()})
			return
		}

		integResp, fetchErr := h.nango.GetIntegration(r.Context(), nk)
		if fetchErr != nil {
			slog.Warn("failed to fetch nango in-integration details for config rebuild", "error", fetchErr, "nango_key", nk)
		} else {
			template, _ := h.nango.GetProviderTemplate(integ.Provider)
			integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
		}
	}

	updates := map[string]any{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if integ.NangoConfig != nil {
		updates["nango_config"] = integ.NangoConfig
	}
	if len(updates) > 0 {
		if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
			slog.Error("failed to update in-integration", "error", err, "integration_id", integ.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update integration"})
			return
		}
	}

	h.db.Where("id = ?", integ.ID).First(&integ)

	slog.Info("in-integration updated", "integration_id", integ.ID, "credentials_updated", req.Credentials != nil)
	writeJSON(w, http.StatusOK, toInIntegrationResponse(integ))
}

// Delete handles DELETE /v1/in/integrations/{id}.
func (h *InIntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	slog.Info("deleting nango in-integration", "integration_id", integ.ID, "nango_key", nk)
	if err := h.nango.DeleteIntegration(r.Context(), nk); err != nil {
		slog.Error("nango in-integration deletion failed", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete integration from Nango: " + err.Error()})
		return
	}

	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		slog.Error("failed to soft-delete in-integration", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	slog.Info("in-integration deleted", "integration_id", integ.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListAvailable handles GET /v1/in/integrations/available.
// Returns non-deleted in-integrations with safe fields (no credentials/nango_config).
func (h *InIntegrationHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	var integrations []model.InIntegration
	if err := h.db.Where("deleted_at IS NULL").Order("created_at ASC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	resp := make([]inIntegrationAvailableResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toInIntegrationAvailableResponse(integ)
	}

	writeJSON(w, http.StatusOK, resp)
}
