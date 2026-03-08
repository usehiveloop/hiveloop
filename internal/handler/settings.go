package handler

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/useportal/proxy-bridge/internal/middleware"
)

// SettingsHandler manages org-level settings.
type SettingsHandler struct {
	db *gorm.DB
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(db *gorm.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

type connectSettingsRequest struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

type connectSettingsResponse struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// GetConnectSettings handles GET /v1/settings/connect.
func (h *SettingsHandler) GetConnectSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	origins := []string{}
	if org.AllowedOrigins != nil {
		origins = org.AllowedOrigins
	}

	writeJSON(w, http.StatusOK, connectSettingsResponse{AllowedOrigins: origins})
}

// UpdateConnectSettings handles PUT /v1/settings/connect.
func (h *SettingsHandler) UpdateConnectSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req connectSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate each origin
	for _, origin := range req.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid origin: " + origin + " (must be http(s)://host)"})
			return
		}
	}

	if err := h.db.Model(org).Update("allowed_origins", pq.StringArray(req.AllowedOrigins)).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update settings"})
		return
	}

	writeJSON(w, http.StatusOK, connectSettingsResponse{AllowedOrigins: req.AllowedOrigins})
}
