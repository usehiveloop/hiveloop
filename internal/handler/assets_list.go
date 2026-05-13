package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type assetListItem struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	AgentID        string `json:"agent_id"`
	Path           string `json:"path"`
	Filename       string `json:"filename"`
	Key            string `json:"key"`
	PublicURL      string `json:"asset_url"`
	ContentType    string `json:"content_type"`
	Bytes          int64  `json:"bytes"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ListAssets handles GET /v1/assets.
//
// Returns assets owned by the caller's org, optionally filtered by:
//
//	?agent_id={uuid}        — assets uploaded inside conversations of this agent
//	?conversation_id={uuid} — assets uploaded inside this conversation
//	?path={folder}          — exact match on the asset's logical folder label
//
// Filters combine. Ordered by created_at desc, cursor-paginated.
//
// @Summary List org assets
// @Description Lists conversation assets owned by the caller's org. Optional filters: agent_id, conversation_id, path. Ordered by created_at desc, cursor-paginated.
// @Tags assets
// @Produce json
// @Param agent_id query string false "Filter to assets uploaded inside conversations of this agent"
// @Param conversation_id query string false "Filter to assets uploaded inside this conversation"
// @Param path query string false "Filter by exact folder label (empty = root)"
// @Param limit query int false "Page size (default 50, max 200)"
// @Param cursor query string false "Pagination cursor — created_at unix-nanos from the previous page's tail"
// @Success 200 {object} paginatedResponse[assetListItem]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/assets [get]
func (h *UploadsHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
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

	q := h.db.
		Table("conversation_assets AS ca").
		Select("ca.*, ac.agent_id AS agent_id_join").
		Joins("JOIN agent_conversations AS ac ON ac.id = ca.conversation_id").
		Where("ca.org_id = ?", org.ID)

	if v := r.URL.Query().Get("conversation_id"); v != "" {
		convID, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid conversation_id"})
			return
		}
		q = q.Where("ca.conversation_id = ?", convID)
	}
	if v := r.URL.Query().Get("agent_id"); v != "" {
		agentID, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			return
		}
		q = q.Where("ac.agent_id = ?", agentID)
	}
	if v, ok := r.URL.Query()["path"]; ok && len(v) > 0 {
		// Path is a labelled folder — exact match. Empty value matches root.
		q = q.Where("ca.path = ?", v[0])
	}

	if c := r.URL.Query().Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		q = q.Where("ca.created_at < ?", time.Unix(0, n))
	}

	type row struct {
		model.ConversationAsset
		AgentIDJoin uuid.UUID `gorm:"column:agent_id_join"`
	}
	var rows []row
	if err := q.Order("ca.created_at DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list assets"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	out := make([]assetListItem, len(rows))
	for i, r := range rows {
		out[i] = assetListItem{
			ID:             r.ID.String(),
			ConversationID: r.ConversationID.String(),
			AgentID:        r.AgentIDJoin.String(),
			Path:           r.Path,
			Filename:       r.Filename,
			Key:            r.Key,
			PublicURL:      r.PublicURL,
			ContentType:    r.ContentType,
			Bytes:          r.Bytes,
			CreatedAt:      r.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      r.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}

	result := paginatedResponse[assetListItem]{Data: out, HasMore: hasMore}
	if hasMore {
		last := rows[len(rows)-1]
		c := strconv.FormatInt(last.CreatedAt.UnixNano(), 10)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}
