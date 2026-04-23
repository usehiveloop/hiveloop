package handler

import (
	"net/http"


	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListUsers handles GET /admin/v1/users.
// @Summary List all users
// @Description Returns all users across the platform with optional filters.
// @Tags admin
// @Produce json
// @Param search query string false "Search by email or name"
// @Param banned query string false "Filter by banned status (true/false)"
// @Param confirmed query string false "Filter by email confirmed status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminUserResponse]
// @Security BearerAuth
// @Router /admin/v1/users [get]
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.User{})

	if search := r.URL.Query().Get("search"); search != "" {
		q = q.Where("email ILIKE ? OR name ILIKE ?", "%"+search+"%", "%"+search+"%")
	}
	if r.URL.Query().Get("banned") == "true" {
		q = q.Where("banned_at IS NOT NULL")
	} else if r.URL.Query().Get("banned") == "false" {
		q = q.Where("banned_at IS NULL")
	}
	if r.URL.Query().Get("confirmed") == "true" {
		q = q.Where("email_confirmed_at IS NOT NULL")
	} else if r.URL.Query().Get("confirmed") == "false" {
		q = q.Where("email_confirmed_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var users []model.User
	if err := q.Find(&users).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}

	resp := make([]adminUserResponse, len(users))
	for i, u := range users {
		resp[i] = toAdminUserResponse(u)
	}

	result := paginatedResponse[adminUserResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := users[len(users)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}