package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("custom_app = false AND deleted_at IS NULL")

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}

	q = applyPagination(q, cursor, limit)

	var integrations []model.Integration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]integrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toIntegrationResponse(integ)
	}

	result := paginatedResponse[integrationResponse]{
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

func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND custom_app = false AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	nk := nangoProviderConfigKey(integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "failed to fetch nango integration", "error", err, "integration_id", integ.ID)
	} else {
		template, _ := h.nango.GetProviderTemplate(nangoProviderName(integ.Provider))
		integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// @Summary List available platform integrations
// @Description Returns non-deleted platform integrations with safe fields for end users.
// @Tags integrations
// @Produce json
// @Success 200 {array} integrationAvailableResponse
// @Security BearerAuth
// @Router /v1/integrations/available [get]
func (h *IntegrationHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	var integrations []model.Integration
	if err := h.db.Where("custom_app = false AND deleted_at IS NULL").Order("created_at ASC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	resp := make([]integrationAvailableResponse, 0, len(integrations))
	for _, integ := range integrations {
		resp = append(resp, toIntegrationAvailableResponse(integ))
	}

	writeJSON(w, http.StatusOK, resp)
}
