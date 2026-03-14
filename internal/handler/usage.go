package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
)

// UsageHandler serves org usage and stats.
type UsageHandler struct {
	db *gorm.DB
}

// NewUsageHandler creates a new usage handler.
func NewUsageHandler(db *gorm.DB) *UsageHandler {
	return &UsageHandler{db: db}
}

type credentialStats struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Revoked int64 `json:"revoked"`
}

type tokenStats struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Expired int64 `json:"expired"`
	Revoked int64 `json:"revoked"`
}

type apiKeyStats struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Revoked int64 `json:"revoked"`
}

type identityStats struct {
	Total int64 `json:"total"`
}

type requestStats struct {
	Total     int64 `json:"total"`
	Today     int64 `json:"today"`
	Yesterday int64 `json:"yesterday"`
	Last7d    int64 `json:"last_7d"`
	Last30d   int64 `json:"last_30d"`
}

type dailyRequests struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type topCredential struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ProviderID   string `json:"provider_id"`
	RequestCount int64  `json:"request_count"`
}

type usageResponse struct {
	Credentials    credentialStats `json:"credentials"`
	Tokens         tokenStats      `json:"tokens"`
	APIKeys        apiKeyStats     `json:"api_keys"`
	Identities     identityStats   `json:"identities"`
	Requests       requestStats    `json:"requests"`
	DailyRequests  []dailyRequests `json:"daily_requests"`
	TopCredentials []topCredential `json:"top_credentials"`
}

// Get handles GET /v1/usage.
// @Summary Get organization usage
// @Description Returns aggregated usage statistics for the current organization.
// @Tags usage
// @Produce json
// @Success 200 {object} usageResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/usage [get]
func (h *UsageHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	last7d := today.AddDate(0, 0, -7)
	last30d := today.AddDate(0, 0, -30)
	orgID := org.ID

	var resp usageResponse
	var wg sync.WaitGroup

	// Query 1: Credentials (single query with conditional aggregation)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var row struct {
			Total   int64
			Active  int64
			Revoked int64
		}
		h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE revoked_at IS NULL) AS active,
				COUNT(*) FILTER (WHERE revoked_at IS NOT NULL) AS revoked
			FROM credentials WHERE org_id = ?`, orgID).Scan(&row)
		resp.Credentials = credentialStats{Total: row.Total, Active: row.Active, Revoked: row.Revoked}
	}()

	// Query 2: Tokens (single query with conditional aggregation)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var row struct {
			Total   int64
			Active  int64
			Expired int64
			Revoked int64
		}
		h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE revoked_at IS NULL AND expires_at > ?) AS active,
				COUNT(*) FILTER (WHERE revoked_at IS NULL AND expires_at <= ?) AS expired,
				COUNT(*) FILTER (WHERE revoked_at IS NOT NULL) AS revoked
			FROM tokens WHERE org_id = ?`, now, now, orgID).Scan(&row)
		resp.Tokens = tokenStats{Total: row.Total, Active: row.Active, Expired: row.Expired, Revoked: row.Revoked}
	}()

	// Query 3: API Keys + Identities (single query, two subselects)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var row struct {
			AKTotal      int64
			AKActive     int64
			IdentTotal   int64
		}
		h.db.Raw(`
			SELECT
				(SELECT COUNT(*) FROM api_keys WHERE org_id = ?) AS ak_total,
				(SELECT COUNT(*) FROM api_keys WHERE org_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)) AS ak_active,
				(SELECT COUNT(*) FROM identities WHERE org_id = ?) AS ident_total`,
			orgID, orgID, now, orgID).Scan(&row)
		resp.APIKeys = apiKeyStats{Total: row.AKTotal, Active: row.AKActive, Revoked: row.AKTotal - row.AKActive}
		resp.Identities = identityStats{Total: row.IdentTotal}
	}()

	// Query 4: Request counts (single query with conditional aggregation)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var row struct {
			Total     int64 `gorm:"column:total"`
			Today     int64 `gorm:"column:today"`
			Yesterday int64 `gorm:"column:yesterday"`
			Last7d    int64 `gorm:"column:last_7d"`
			Last30d   int64 `gorm:"column:last_30d"`
		}
		h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE created_at >= ?) AS today,
				COUNT(*) FILTER (WHERE created_at >= ? AND created_at < ?) AS yesterday,
				COUNT(*) FILTER (WHERE created_at >= ?) AS last_7d,
				COUNT(*) FILTER (WHERE created_at >= ?) AS last_30d
			FROM audit_log WHERE org_id = ? AND action = 'proxy.request'`,
			today, yesterday, today, last7d, last30d, orgID).Scan(&row)
		resp.Requests = requestStats{
			Total:     row.Total,
			Today:     row.Today,
			Yesterday: row.Yesterday,
			Last7d:    row.Last7d,
			Last30d:   row.Last30d,
		}
	}()

	// Query 5: Daily request counts (last 30 days)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var dailyRows []struct {
			Date  time.Time
			Count int64
		}
		h.db.Raw(`
			SELECT DATE(created_at) AS date, COUNT(*) AS count
			FROM audit_log
			WHERE org_id = ? AND action = 'proxy.request' AND created_at >= ?
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&dailyRows)

		resp.DailyRequests = make([]dailyRequests, 0, len(dailyRows))
		for _, row := range dailyRows {
			resp.DailyRequests = append(resp.DailyRequests, dailyRequests{
				Date:  row.Date.Format("2006-01-02"),
				Count: row.Count,
			})
		}
	}()

	// Query 6: Top credentials by request count (last 30 days)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var topRows []struct {
			CredentialID uuid.UUID
			Label        string
			ProviderID   string
			RequestCount int64
		}
		h.db.Raw(`
			SELECT a.credential_id, c.label, c.provider_id, COUNT(*) AS request_count
			FROM audit_log a
			JOIN credentials c ON c.id = a.credential_id
			WHERE a.org_id = ? AND a.action = 'proxy.request' AND a.created_at >= ? AND a.credential_id IS NOT NULL
			GROUP BY a.credential_id, c.label, c.provider_id
			ORDER BY request_count DESC
			LIMIT 5`, orgID, last30d).Scan(&topRows)

		resp.TopCredentials = make([]topCredential, 0, len(topRows))
		for _, row := range topRows {
			resp.TopCredentials = append(resp.TopCredentials, topCredential{
				ID:           row.CredentialID.String(),
				Label:        row.Label,
				ProviderID:   row.ProviderID,
				RequestCount: row.RequestCount,
			})
		}
	}()

	wg.Wait()

	// Ensure non-null arrays
	if resp.DailyRequests == nil {
		resp.DailyRequests = []dailyRequests{}
	}
	if resp.TopCredentials == nil {
		resp.TopCredentials = []topCredential{}
	}

	writeJSON(w, http.StatusOK, resp)
}
