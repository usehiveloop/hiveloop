package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// GetUser handles GET /admin/v1/users/{id}.
// @Summary Get user details
// @Description Returns user details including org memberships.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [get]
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
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

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	type membershipEntry struct {
		OrgID   string `json:"org_id"`
		OrgName string `json:"org_name"`
		Role    string `json:"role"`
	}
	orgs := make([]membershipEntry, len(memberships))
	for i, m := range memberships {
		orgs[i] = membershipEntry{
			OrgID:   m.OrgID.String(),
			OrgName: m.Org.Name,
			Role:    m.Role,
		}
	}

	type detailResponse struct {
		adminUserResponse
		Orgs []membershipEntry `json:"orgs"`
	}

	writeJSON(w, http.StatusOK, detailResponse{
		adminUserResponse: toAdminUserResponse(user),
		Orgs:              orgs,
	})
}

// BanUser handles POST /admin/v1/users/{id}/ban.
// @Summary Ban a user
// @Description Bans a user account and revokes all refresh tokens.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/ban [post]
func (h *AdminHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = ""
	}

	var user model.User
	if err := h.db.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	if user.BannedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user is already banned"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&user).Updates(map[string]any{
		"banned_at": now,
		"ban_reason": req.Reason,
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to ban user"})
		return
	}

	h.db.Model(&model.RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).
		Update("revoked_at", now)

	slog.Info("admin: user banned", "user_id", id, "reason", req.Reason)

	user.BannedAt = &now
	user.BanReason = req.Reason
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// UnbanUser handles POST /admin/v1/users/{id}/unban.
// @Summary Unban a user
// @Description Removes the ban from a user account.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/unban [post]
func (h *AdminHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
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

	if user.BannedAt == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user is not banned"})
		return
	}

	if err := h.db.Model(&user).Updates(map[string]any{
		"banned_at": nil,
		"ban_reason": "",
	}).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unban user"})
		return
	}

	slog.Info("admin: user unbanned", "user_id", id)

	user.BannedAt = nil
	user.BanReason = ""
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// ConfirmUserEmail handles POST /admin/v1/users/{id}/confirm-email.
// @Summary Force-confirm user email
// @Description Administratively confirms a user's email address.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} adminUserResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/confirm-email [post]
func (h *AdminHandler) ConfirmUserEmail(w http.ResponseWriter, r *http.Request) {
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

	if user.EmailConfirmedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already confirmed"})
		return
	}

	now := time.Now()
	if err := h.db.Model(&user).Update("email_confirmed_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to confirm email"})
		return
	}

	slog.Info("admin: user email confirmed", "user_id", id)

	user.EmailConfirmedAt = &now
	writeJSON(w, http.StatusOK, toAdminUserResponse(user))
}

// DeleteUser handles DELETE /admin/v1/users/{id}.
// @Summary Delete a user
// @Description Permanently deletes a user and all associated data.
// @Tags admin
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id} [delete]
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	uid, err := uuid.Parse(id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user ID"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", uid).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get user"})
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&model.OrgMembership{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.RefreshToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.OAuthAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.EmailVerification{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", uid).Delete(&model.PasswordReset{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&user).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete user"})
		return
	}

	slog.Info("admin: user deleted", "user_id", id, "email", user.Email)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}