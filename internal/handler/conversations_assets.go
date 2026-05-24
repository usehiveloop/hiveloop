package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type conversationAssetResponse struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	Key         string `json:"key"`
	PublicURL   string `json:"asset_url"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListAssets handles GET /v1/conversations/{convID}/assets.
//
// Returns conversation assets ordered by created_at desc, paginated by an
// opaque cursor (created_at unix-nanos of the last row in the page).
func (h *ConversationHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	convID := chi.URLParam(r, "convID")
	var conv model.EmployeeConversation
	if err := h.db.Where("id = ? AND org_id = ?", convID, org.ID).First(&conv).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load conversation"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		if n > 200 {
			n = 200
		}
		limit = n
	}

	q := h.db.Where("conversation_id = ?", conv.ID)
	if c := r.URL.Query().Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		q = q.Where("created_at < ?", time.Unix(0, n))
	}

	var assets []model.ConversationAsset
	if err := q.Order("created_at DESC").Limit(limit + 1).Find(&assets).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	hasMore := len(assets) > limit
	if hasMore {
		assets = assets[:limit]
	}

	out := make([]conversationAssetResponse, len(assets))
	for i, a := range assets {
		out[i] = conversationAssetResponse{
			ID:          a.ID.String(),
			Path:        a.Path,
			Filename:    a.Filename,
			Key:         a.Key,
			PublicURL:   a.PublicURL,
			ContentType: a.ContentType,
			Bytes:       a.Bytes,
			CreatedAt:   a.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:   a.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}

	result := paginatedResponse[conversationAssetResponse]{Data: out, HasMore: hasMore}
	if hasMore {
		last := assets[len(assets)-1]
		c := strconv.FormatInt(last.CreatedAt.UnixNano(), 10)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}
