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

	data := make([]ragSourceResponse, len(rows))
	for i := range rows {
		data[i] = toRAGSourceResponse(&rows[i])
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
