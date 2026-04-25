package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// List handles GET /v1/credentials.
// @Summary List credentials
// @Description Returns credentials for the current organization with cursor-based pagination and usage stats.
// @Tags credentials
// @Produce json
// @Param meta query string false "Filter by JSONB metadata (e.g. {\"key\":\"value\"})"
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[credentialResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials [get]
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// is_system = false is defense in depth: org scoping already prevents a
	// customer from seeing the platform org's credentials, but this makes it
	// impossible for a misconfigured row (is_system = true attached to a
	// customer org) to leak either.
	q := h.db.Where("credentials.org_id = ? AND credentials.is_system = ?", org.ID, false)

	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("credentials.meta @> ?::jsonb", metaFilter)
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

	credIDs := make([]uuid.UUID, len(creds))
	for i, c := range creds {
		credIDs[i] = c.ID
	}

	type credStats struct {
		CredentialID uuid.UUID  `gorm:"column:credential_id"`
		RequestCount int64      `gorm:"column:request_count"`
		LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	}
	statsMap := make(map[string]credStats)
	if len(credIDs) > 0 {
		var stats []credStats
		h.db.Raw(`SELECT credential_id, COUNT(*) AS request_count, MAX(created_at) AS last_used_at
			FROM audit_log
			WHERE org_id = ? AND action = 'proxy.request' AND credential_id IN (?)
			GROUP BY credential_id`, org.ID, credIDs).Scan(&stats)
		for _, s := range stats {
			statsMap[s.CredentialID.String()] = s
		}
	}

	resp := make([]credentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = credentialResponse{
			ID:             c.ID.String(),
			Label:          c.Label,
			BaseURL:        c.BaseURL,
			AuthScheme:     c.AuthScheme,
			ProviderID:     c.ProviderID,
			Remaining:      c.Remaining,
			RefillAmount:   c.RefillAmount,
			RefillInterval: c.RefillInterval,
			Meta:           c.Meta,
			CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		}
		if c.RevokedAt != nil {
			s := c.RevokedAt.Format(time.RFC3339)
			resp[i].RevokedAt = &s
		}
		if st, ok := statsMap[c.ID.String()]; ok {
			resp[i].RequestCount = st.RequestCount
			if st.LastUsedAt != nil {
				s := st.LastUsedAt.Format(time.RFC3339)
				resp[i].LastUsedAt = &s
			}
		}
	}

	result := paginatedResponse[credentialResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/credentials/{id}.
// @Summary Get a credential
// @Description Returns a single credential by ID with usage stats.
// @Tags credentials
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} credentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials/{id} [get]
func (h *CredentialHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}

	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ? AND is_system = ?", credID, org.ID, false).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get credential"})
		return
	}

	resp := credentialResponse{
		ID:             cred.ID.String(),
		Label:          cred.Label,
		BaseURL:        cred.BaseURL,
		AuthScheme:     cred.AuthScheme,
		ProviderID:     cred.ProviderID,
		Remaining:      cred.Remaining,
		RefillAmount:   cred.RefillAmount,
		RefillInterval: cred.RefillInterval,
		Meta:           cred.Meta,
		CreatedAt:      cred.CreatedAt.Format(time.RFC3339),
	}
	if cred.RevokedAt != nil {
		s := cred.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}

	var stats struct {
		RequestCount int64      `gorm:"column:request_count"`
		LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	}
	h.db.Raw(`SELECT COUNT(*) AS request_count, MAX(created_at) AS last_used_at
		FROM audit_log
		WHERE org_id = ? AND action = 'proxy.request' AND credential_id = ?`, org.ID, cred.ID).Scan(&stats)
	resp.RequestCount = stats.RequestCount
	if stats.LastUsedAt != nil {
		s := stats.LastUsedAt.Format(time.RFC3339)
		resp.LastUsedAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}
