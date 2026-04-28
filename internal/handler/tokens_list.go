package handler

import (
	"net/http"


	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary List tokens
// @Description Returns tokens for the organization with cursor pagination. Supports filtering by credential_id.
// @Tags tokens
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Param credential_id query string false "Filter by credential ID"
// @Success 200 {object} paginatedResponse[tokenListItem]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens [get]
func (h *TokenHandler) List(w http.ResponseWriter, r *http.Request) {
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

	q := h.db.Where("org_id = ?", org.ID)

	if credID := r.URL.Query().Get("credential_id"); credID != "" {
		q = q.Where("credential_id = ?", credID)
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

	resp := make([]tokenListItem, len(tokens))
	for i, t := range tokens {
		resp[i] = toTokenListItem(t)
	}

	result := paginatedResponse[tokenListItem]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := tokens[len(tokens)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

