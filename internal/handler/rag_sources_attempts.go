package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// @Summary List index attempts for a RAG source
// @Description Paginated, most-recent-first. Each row covers one ingest / perm-sync / prune attempt with status, doc counts, and error summary.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Param page query int false "Page number (0-indexed)"
// @Param page_size query int false "Page size, max 100"
// @Success 200 {object} ragAttemptsListResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/attempts [get]
func (h *RAGSourceHandler) ListAttempts(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}

	if _, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	page, size, err := parseAttemptsPagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	rows, total, err := ragdb.ListAttemptsForSource(h.db, org.ID, srcID, page, size)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list attempts"})
		return
	}

	data := make([]ragIndexAttemptResponse, len(rows))
	for i := range rows {
		data[i] = toRAGAttemptResponse(&rows[i])
	}

	writeJSON(w, http.StatusOK, ragAttemptsListResponse{
		Data:  data,
		Total: total,
		Page:  page,
		Size:  size,
	})
}

// Port of cc_pair.py:499 (errors) folded into the per-attempt detail.
// @Summary Get an index-attempt with per-doc errors
// @Description Returns the attempt's status / counts / window plus the first page of per-doc failure rows. The dedicated errors page can be used to walk later pages if needed.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Param attempt_id path string true "Attempt ID"
// @Success 200 {object} ragAttemptDetailResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/attempts/{attempt_id} [get]
func (h *RAGSourceHandler) GetAttempt(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}
	attemptID, ok := parseSourceID(chi.URLParam(r, "attempt_id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid attempt id"})
		return
	}

	attempt, err := ragdb.GetAttemptForSource(h.db, org.ID, srcID, attemptID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "attempt not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load attempt"})
		return
	}

	page, size, err := parseAttemptsPagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	errs, total, err := ragdb.ListAttemptErrors(h.db, attempt.ID, page, size)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load errors"})
		return
	}

	resp := ragAttemptDetailResponse{
		ragIndexAttemptResponse: toRAGAttemptResponse(attempt),
		FullExceptionTrace:      attempt.FullExceptionTrace,
		Errors:                  make([]ragAttemptErrorPayload, len(errs)),
		ErrorCount:              int(total),
	}
	for i := range errs {
		resp.Errors[i] = toAttemptErrorPayload(&errs[i])
	}

	writeJSON(w, http.StatusOK, resp)
}

func toAttemptErrorPayload(e *ragmodel.RAGIndexAttemptError) ragAttemptErrorPayload {
	return ragAttemptErrorPayload{
		ID:                   e.ID.String(),
		DocumentID:           e.DocumentID,
		DocumentLink:         e.DocumentLink,
		EntityID:             e.EntityID,
		FailedTimeRangeStart: e.FailedTimeRangeStart,
		FailedTimeRangeEnd:   e.FailedTimeRangeEnd,
		FailureMessage:       e.FailureMessage,
		IsResolved:           e.IsResolved,
		ErrorType:            e.ErrorType,
		TimeCreated:          e.TimeCreated,
	}
}

func parseAttemptsPagination(r *http.Request) (int, int, error) {
	page := 0
	size := ragdb.DefaultPageSize
	q := r.URL.Query()
	if p := q.Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, errInvalidPage
		}
		page = n
	}
	if ps := q.Get("page_size"); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n < 1 || n > ragdb.MaxPageSize {
			return 0, 0, errInvalidPageSize
		}
		size = n
	}
	return page, size, nil
}
