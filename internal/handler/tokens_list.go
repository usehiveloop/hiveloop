package handler

import (
	"net/http"


	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TokenHandler manages sandbox proxy token operations.

// MCPServerCache is an interface for evicting cached MCP servers.

// NewTokenHandler creates a new token handler.
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

// Revoke handles DELETE /v1/tokens/{jti}.
// @Summary Revoke a proxy token
// @Description Revokes a proxy token by its JTI and propagates through cache tiers.
// @Tags tokens
// @Produce json
// @Param jti path string true "Token JTI"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens/{jti} [delete]
