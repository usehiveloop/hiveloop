package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *InIntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("deleted_at IS NULL")

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}

	q = applyPagination(q, cursor, limit)

	var integrations []model.InIntegration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]inIntegrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toInIntegrationResponse(integ)
	}

	result := paginatedResponse[inIntegrationResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := integrations[len(integrations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *InIntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango in-integration", "error", err, "integration_id", integ.ID)
	} else {
		template, _ := h.nango.GetProviderTemplate(integ.Provider)
		integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	writeJSON(w, http.StatusOK, toInIntegrationResponse(integ))
}

// @Summary List available platform integrations
// @Description Returns non-deleted platform integrations with safe fields for end users.
// @Tags in-integrations
// @Produce json
// @Success 200 {array} inIntegrationAvailableResponse
// @Security BearerAuth
// @Router /v1/in/integrations/available [get]
func (h *InIntegrationHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	var integrations []model.InIntegration
	if err := h.db.Where("deleted_at IS NULL").Order("created_at ASC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	resp := make([]inIntegrationAvailableResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toInIntegrationAvailableResponse(integ)
	}

	writeJSON(w, http.StatusOK, resp)
}
