package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

// SettingsHandler manages org-level settings.
type SettingsHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
}

// NewSettingsHandler creates a new settings handler.
func NewSettingsHandler(db *gorm.DB, encKey *crypto.SymmetricKey) *SettingsHandler {
	return &SettingsHandler{db: db, encKey: encKey}
}

type connectSettingsRequest struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

type connectSettingsResponse struct {
	AllowedOrigins []string `json:"allowed_origins"`
}

// GetConnectSettings handles GET /v1/settings/connect.
// @Summary Get Connect settings
// @Description Returns the Connect widget settings for the current organization.
// @Tags settings
// @Produce json
// @Success 200 {object} connectSettingsResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/connect [get]
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
// @Summary Update Connect settings
// @Description Updates the Connect widget settings (e.g. allowed origins) for the current organization.
// @Tags settings
// @Accept json
// @Produce json
// @Param body body connectSettingsRequest true "Settings to update"
// @Success 200 {object} connectSettingsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/connect [put]
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

// --- Webhook Settings ---

type webhookSettingsResponse struct {
	URL          string `json:"url"`
	SecretPrefix string `json:"secret_prefix"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type webhookSettingsCreateResponse struct {
	URL          string `json:"url"`
	Secret       string `json:"secret"` // plaintext, shown once
	SecretPrefix string `json:"secret_prefix"`
	CreatedAt    string `json:"created_at"`
}

type updateWebhookSettingsRequest struct {
	URL string `json:"url"`
}

// GetWebhookSettings handles GET /v1/settings/webhooks.
// @Summary Get webhook settings
// @Description Returns the webhook configuration for the current organization.
// @Tags settings
// @Produce json
// @Success 200 {object} webhookSettingsResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/webhooks [get]
func (h *SettingsHandler) GetWebhookSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var config model.OrgWebhookConfig
	if err := h.db.Where("org_id = ?", org.ID).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no webhook configuration found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load webhook settings"})
		return
	}

	writeJSON(w, http.StatusOK, webhookSettingsResponse{
		URL:          config.URL,
		SecretPrefix: config.SecretPrefix,
		CreatedAt:    config.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    config.UpdatedAt.Format(time.RFC3339),
	})
}

// UpdateWebhookSettings handles PUT /v1/settings/webhooks.
// Creates a new config (with generated secret) if none exists, or updates the URL.
// @Summary Update webhook settings
// @Description Creates a new webhook configuration (with generated secret) or updates the URL of an existing configuration.
// @Tags settings
// @Accept json
// @Produce json
// @Param body body updateWebhookSettingsRequest true "URL for webhook delivery"
// @Success 201 {object} webhookSettingsCreateResponse
// @Success 200 {object} webhookSettingsResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/webhooks [put]
func (h *SettingsHandler) UpdateWebhookSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req updateWebhookSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	u, err := url.Parse(req.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid url (must be http(s)://host)"})
		return
	}

	var existing model.OrgWebhookConfig
	if err := h.db.Where("org_id = ?", org.ID).First(&existing).Error; err == nil {
		// Update URL only
		if err := h.db.Model(&existing).Update("url", req.URL).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update webhook settings"})
			return
		}
		writeJSON(w, http.StatusOK, webhookSettingsResponse{
			URL:          req.URL,
			SecretPrefix: existing.SecretPrefix,
			CreatedAt:    existing.CreatedAt.Format(time.RFC3339),
			UpdatedAt:    existing.UpdatedAt.Format(time.RFC3339),
		})
		return
	}

	// Create new config with generated secret
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "encryption not configured"})
		return
	}

	plaintext, prefix, err := model.GenerateWebhookSecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate webhook secret"})
		return
	}

	encSecret, err := h.encKey.EncryptString(plaintext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt webhook secret"})
		return
	}

	config := model.OrgWebhookConfig{
		OrgID:           org.ID,
		URL:             req.URL,
		EncryptedSecret: encSecret,
		SecretPrefix:    prefix,
	}
	if err := h.db.Create(&config).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create webhook configuration"})
		return
	}

	writeJSON(w, http.StatusCreated, webhookSettingsCreateResponse{
		URL:          config.URL,
		Secret:       plaintext,
		SecretPrefix: config.SecretPrefix,
		CreatedAt:    config.CreatedAt.Format(time.RFC3339),
	})
}

// RotateWebhookSecret handles POST /v1/settings/webhooks/rotate-secret.
// @Summary Rotate webhook secret
// @Description Generates a new webhook secret for the existing configuration.
// @Tags settings
// @Produce json
// @Success 200 {object} webhookSettingsCreateResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/webhooks/rotate-secret [post]
func (h *SettingsHandler) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var config model.OrgWebhookConfig
	if err := h.db.Where("org_id = ?", org.ID).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no webhook configuration found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load webhook settings"})
		return
	}

	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "encryption not configured"})
		return
	}

	plaintext, prefix, err := model.GenerateWebhookSecret()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate webhook secret"})
		return
	}

	encSecret, err := h.encKey.EncryptString(plaintext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt webhook secret"})
		return
	}

	if err := h.db.Model(&config).Updates(map[string]any{
		"encrypted_secret": encSecret,
		"secret_prefix":    prefix,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to rotate webhook secret"})
		return
	}

	writeJSON(w, http.StatusOK, webhookSettingsCreateResponse{
		URL:          config.URL,
		Secret:       plaintext,
		SecretPrefix: prefix,
		CreatedAt:    config.CreatedAt.Format(time.RFC3339),
	})
}

// DeleteWebhookSettings handles DELETE /v1/settings/webhooks.
// @Summary Delete webhook settings
// @Description Deletes the webhook configuration for the current organization.
// @Tags settings
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/settings/webhooks [delete]
func (h *SettingsHandler) DeleteWebhookSettings(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	result := h.db.Where("org_id = ?", org.ID).Delete(&model.OrgWebhookConfig{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete webhook settings"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no webhook configuration found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
