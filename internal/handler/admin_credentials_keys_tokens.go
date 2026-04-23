package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListCredentials handles GET /admin/v1/credentials.
// @Summary List all credentials
// @Description Returns credentials across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param provider_id query string false "Filter by provider ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminCredentialResponse]
// @Security BearerAuth
// @Router /admin/v1/credentials [get]
func (h *AdminHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Credential{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if provider := r.URL.Query().Get("provider_id"); provider != "" {
		q = q.Where("provider_id = ?", provider)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var creds []model.Credential
	if err := q.Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list credentials"})
		return
	}

	hasMore := len(creds) > limit
	if hasMore {
		creds = creds[:limit]
	}

	resp := make([]adminCredentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = toAdminCredentialResponse(c)
	}

	result := paginatedResponse[adminCredentialResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// GetCredential handles GET /admin/v1/credentials/{id}.
// @Summary Get credential details
// @Description Returns credential metadata (no decrypted key).
// @Tags admin
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} adminCredentialResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id} [get]
func (h *AdminHandler) GetCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cred model.Credential
	if err := h.db.Where("id = ?", id).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminCredentialResponse(cred))
}

// RevokeCredential handles POST /admin/v1/credentials/{id}/revoke.
// @Summary Revoke a credential
// @Description Force-revokes a credential.
// @Tags admin
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/credentials/{id}/revoke [post]
func (h *AdminHandler) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.Credential{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke credential"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found or already revoked"})
		return
	}

	slog.Info("admin: credential revoked", "credential_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}


// ListAPIKeys handles GET /admin/v1/api-keys.
// @Summary List all API keys
// @Description Returns API keys across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminAPIKeyResponse]
// @Security BearerAuth
// @Router /admin/v1/api-keys [get]
func (h *AdminHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.APIKey{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var keys []model.APIKey
	if err := q.Find(&keys).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list api keys"})
		return
	}

	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}

	resp := make([]adminAPIKeyResponse, len(keys))
	for i, k := range keys {
		resp[i] = toAdminAPIKeyResponse(k)
	}

	result := paginatedResponse[adminAPIKeyResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := keys[len(keys)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// RevokeAPIKey handles POST /admin/v1/api-keys/{id}/revoke.
// @Summary Revoke an API key
// @Description Force-revokes an API key.
// @Tags admin
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/api-keys/{id}/revoke [post]
func (h *AdminHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.APIKey{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke api key"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found or already revoked"})
		return
	}

	slog.Info("admin: api key revoked", "api_key_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}


// ListTokens handles GET /admin/v1/tokens.
// @Summary List all proxy tokens
// @Description Returns proxy tokens across all organizations.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminTokenResponse]
// @Security BearerAuth
// @Router /admin/v1/tokens [get]
func (h *AdminHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.Token{})
	if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var tokens []model.Token
	if err := q.Find(&tokens).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tokens"})
		return
	}

	hasMore := len(tokens) > limit
	if hasMore {
		tokens = tokens[:limit]
	}

	resp := make([]adminTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = toAdminTokenResponse(t)
	}

	result := paginatedResponse[adminTokenResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := tokens[len(tokens)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// RevokeToken handles POST /admin/v1/tokens/{id}/revoke.
// @Summary Revoke a proxy token
// @Description Force-revokes a proxy token.
// @Tags admin
// @Produce json
// @Param id path string true "Token ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/tokens/{id}/revoke [post]
func (h *AdminHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.Model(&model.Token{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	slog.Info("admin: token revoked", "token_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}