package handler

import (
	"net/http"

	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
)

// @Summary List RAG-supported integrations
// @Description Returns the platform integrations that can be used as RAG sources (i.e. their `supports_rag_source` flag is true). The admin UI uses this to filter the connection picker for the Knowledge Base.
// @Tags rag
// @Produce json
// @Success 200 {object} ragIntegrationsListResponse
// @Security BearerAuth
// @Router /v1/rag/integrations [get]
func (h *RAGSourceHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	rows, err := ragdb.ListSupportedIntegrations(h.db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list integrations"})
		return
	}
	resp := make([]ragIntegrationResponse, len(rows))
	for i := range rows {
		resp[i] = toRAGIntegrationResponse(&rows[i])
	}
	writeJSON(w, http.StatusOK, ragIntegrationsListResponse{Data: resp})
}
