package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// @Summary List RAG sources
// @Description Returns the org's RAG sources. Supports pagination and optional status / kind filters.
// @Tags rag
// @Produce json
// @Param status query string false "Filter by status"
// @Param kind query string false "Filter by kind"
// @Param page query int false "Page number (0-indexed)"
// @Param page_size query int false "Page size, max 100"
// @Success 200 {object} ragListResponse
// @Security BearerAuth
// @Router /v1/rag/sources [get]
func (h *RAGSourceHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	opts, err := parseListOptions(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	rows, total, err := ragdb.ListSourcesForOrg(h.db, org.ID, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list sources"})
		return
	}

	srcIDs := make([]uuid.UUID, len(rows))
	for i := range rows {
		srcIDs[i] = rows[i].ID
	}
	latest, err := ragdb.LatestAttemptsBySource(h.db, org.ID, srcIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load latest attempts"})
		return
	}

	data := make([]ragSourceResponse, len(rows))
	for i := range rows {
		data[i] = toRAGSourceResponse(&rows[i])
		if a, ok := latest[rows[i].ID]; ok {
			data[i].LatestAttempt = toRAGLatestAttemptStatus(&a)
		}
	}

	page := opts.Page
	size := opts.PageSize
	if size <= 0 {
		size = ragdb.DefaultPageSize
	}
	writeJSON(w, http.StatusOK, ragListResponse{
		Data:  data,
		Total: total,
		Page:  page,
		Size:  size,
	})
}

// Port of backend/onyx/server/documents/cc_pair.py:156 detail endpoint.
// Inlines the most-recent attempts so the admin UI doesn't need a
// second round-trip.
// @Summary Get a RAG source
// @Description Returns one RAG source by ID with the last 5 index attempts inlined. 404 on cross-org access by design.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 200 {object} ragSourceDetailResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id} [get]
func (h *RAGSourceHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	src, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	attempts, err := ragdb.ListRecentAttempts(h.db, org.ID, srcID, 5)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load recent attempts"})
		return
	}

	resp := ragSourceDetailResponse{
		ragSourceResponse: toRAGSourceResponse(src),
		RecentAttempts:    make([]ragIndexAttemptResponse, len(attempts)),
	}
	for i := range attempts {
		resp.RecentAttempts[i] = toRAGAttemptResponse(&attempts[i])
	}
	if len(attempts) > 0 {
		resp.LatestAttempt = toRAGLatestAttemptStatus(&attempts[0])
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseListOptions(r *http.Request) (ragdb.ListOptions, error) {
	opts := ragdb.ListOptions{}
	q := r.URL.Query()

	if p := q.Get("page"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return opts, errInvalidPage
		}
		opts.Page = n
	}
	if ps := q.Get("page_size"); ps != "" {
		n, err := strconv.Atoi(ps)
		if err != nil || n < 1 || n > ragdb.MaxPageSize {
			return opts, errInvalidPageSize
		}
		opts.PageSize = n
	}
	if s := q.Get("status"); s != "" {
		st := ragmodel.RAGSourceStatus(s)
		if !st.IsValid() {
			return opts, errInvalidStatus
		}
		opts.StatusFilter = &st
	}
	if k := q.Get("kind"); k != "" {
		kk := ragmodel.RAGSourceKind(k)
		if !kk.IsValid() {
			return opts, errInvalidKind
		}
		opts.KindFilter = &kk
	}
	return opts, nil
}

var (
	errInvalidPage     = listOptErr("invalid page")
	errInvalidPageSize = listOptErr("invalid page_size (1-100)")
	errInvalidStatus   = listOptErr("invalid status filter")
	errInvalidKind     = listOptErr("invalid kind filter")
)

type listOptErr string

func (e listOptErr) Error() string { return string(e) }
