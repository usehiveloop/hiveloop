package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Disconnect an in-connection
// @Description Revokes a user's platform integration connection and removes it from Nango.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id} [delete]
func (h *InConnectionHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}
	nk := inNangoKey(conn.InIntegration.UniqueKey)
	if err := h.nango.DeleteConnection(r.Context(), conn.NangoConnectionID, nk); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango: delete connection failed, proceeding with local revocation",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	}

	now := time.Now()
	result := h.db.Model(&model.InConnection{}).
		Where("id = ? AND revoked_at IS NULL", connID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found or already revoked"})
		return
	}
	if err := h.afterConnectionRevoked(r, org.ID, conn.InIntegration.Provider); err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "post-revoke employee connection cleanup failed",
			"error", err, "connection_id", conn.ID, "org_id", org.ID, "provider", conn.InIntegration.Provider)
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "in-connection revoked", "connection_id", conn.ID, "org_id", org.ID, "provider", conn.InIntegration.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *InConnectionHandler) afterConnectionRevoked(r *http.Request, orgID uuid.UUID, provider string) error {
	employee, err := ensureHivyEmployee(r.Context(), h.db, orgID)
	if err != nil {
		return err
	}

	revokedProviderSkills, err := loadPublishedGlobalSkillsByIntegrationIDs(r.Context(), h.db, []string{provider})
	if err != nil {
		return err
	}
	stillRequired, _, err := employeeRequiredSkills(r.Context(), h.db, orgID)
	if err != nil {
		return err
	}
	for skillID, skill := range revokedProviderSkills {
		if _, required := stillRequired[skillID]; required {
			continue
		}
		if err := h.db.WithContext(r.Context()).
			Where("agent_id = ? AND skill_id = ?", employee.ID, skill.ID).
			Delete(&model.AgentSkill{}).Error; err != nil {
			return err
		}
	}
	if provider == "slack" {
		providers, _, err := activeEmployeeConnectionProviders(r.Context(), h.db, orgID)
		if err != nil {
			return err
		}
		for _, activeProvider := range providers {
			if activeProvider == "slack" {
				return nil
			}
		}
		return h.db.WithContext(r.Context()).Model(&model.Org{}).Where("id = ?", orgID).Update("onboarded", false).Error
	}
	return nil
}
