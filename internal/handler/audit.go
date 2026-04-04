package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
)

// AuditHandler serves audit log entries.
type AuditHandler struct {
	db *gorm.DB
}

// NewAuditHandler creates a new audit handler.
func NewAuditHandler(db *gorm.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

type auditEntryResponse struct {
	ID           int64   `json:"id"`
	Action       string  `json:"action"`
	Method       string  `json:"method,omitempty"`
	Path         string  `json:"path,omitempty"`
	Status       int     `json:"status,omitempty"`
	LatencyMs    int64   `json:"latency_ms,omitempty"`
	CredentialID *string `json:"credential_id,omitempty"`
	IdentityID   *string `json:"identity_id,omitempty"`
	IPAddress    *string `json:"ip_address,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// List handles GET /v1/audit.
// @Summary List audit log entries
// @Description Returns audit log entries for the current organization with cursor pagination. Cursor is the last-seen entry ID.
// @Tags audit
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor (entry ID) from previous response"
// @Param action query string false "Filter by action (e.g. proxy.request, api.request)"
// @Success 200 {object} paginatedResponse[auditEntryResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/audit [get]
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
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
		if n > 100 {
			n = 100
		}
		limit = n
	}

	q := h.db.Table("audit_log").Where("org_id = ?", org.ID)

	if action := r.URL.Query().Get("action"); action != "" {
		q = q.Where("action = ?", action)
	}

	if c := r.URL.Query().Get("cursor"); c != "" {
		cursorID, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
		q = q.Where("id < ?", cursorID)
	}

	q = q.Order("id DESC").Limit(limit + 1)

	type row struct {
		ID           int64      `gorm:"column:id"`
		Action       string     `gorm:"column:action"`
		CredentialID *string    `gorm:"column:credential_id"`
		IdentityID   *string    `gorm:"column:identity_id"`
		IPAddress    *string    `gorm:"column:ip_address"`
		Metadata     []byte     `gorm:"column:metadata"`
		CreatedAt    time.Time  `gorm:"column:created_at"`
	}

	var rows []row
	if err := q.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list audit entries"})
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]auditEntryResponse, len(rows))
	for i, r := range rows {
		items[i] = auditEntryResponse{
			ID:           r.ID,
			Action:       r.Action,
			CredentialID: r.CredentialID,
			IdentityID:   r.IdentityID,
			IPAddress:    r.IPAddress,
			CreatedAt:    r.CreatedAt.Format(time.RFC3339),
		}

		// Parse flattened fields from JSONB metadata
		meta := parseJSONB(r.Metadata)
		if m, ok := meta["method"].(string); ok {
			items[i].Method = m
		}
		if p, ok := meta["path"].(string); ok {
			items[i].Path = p
		}
		if s, ok := meta["status"].(float64); ok {
			items[i].Status = int(s)
		}
		if l, ok := meta["latency_ms"].(float64); ok {
			items[i].LatencyMs = int64(l)
		}
	}

	resp := paginatedResponse[auditEntryResponse]{
		Data:    items,
		HasMore: hasMore,
	}
	if hasMore {
		last := rows[len(rows)-1]
		c := strconv.FormatInt(last.ID, 10)
		resp.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseJSONB(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}
