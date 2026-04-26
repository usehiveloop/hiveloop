package handler

import (
	"net/http"

	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
)

// ListIntegrations returns the InIntegration rows whose
// `supports_rag_source` flag is true. The admin UI uses this to
// populate the "Add RAG source" picker. No org scoping needed because
// integrations are platform-level, not org-level.
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
	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}
