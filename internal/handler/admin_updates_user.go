package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)


// UpdateUser handles PUT /admin/v1/users/{id}.
// @Summary Update a user
// @Description Updates user name and/or email with validation.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param body body adminUpdateUserRequest true "Fields to update"
// @Success 200 {object} adminUserResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [put]
func (h *AdminHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	var req struct {
		Name  *string `json:"name,omitempty"`
		Email *string `json:"email,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}

	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if email == "" || !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email format"})
			return
		}
		// Check uniqueness
		var existing model.User
		if err := h.db.Where("email = ? AND id != ?", email, user.ID).First(&existing).Error; err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use by another user"})
			return
		}
		updates["email"] = email
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	// Compute diff for audit (only what actually changed)
	old := map[string]any{"name": user.Name, "email": user.Email}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&user).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
		return
	}

	h.db.Where("id = ?", id).First(&user)
	slog.Info("admin: user updated", "user_id", id)
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}