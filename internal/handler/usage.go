package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
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

type spendOverTime struct {
	Date      string  `json:"date"`
	TotalCost float64 `json:"total_cost"`
}

type tokenVolumes struct {
	Date         string `json:"date"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CachedTokens int64  `json:"cached_tokens"`
}

type latencyStats struct {
	Date      string  `json:"date"`
	AvgTTFBMs float64 `json:"avg_ttfb_ms"`
	P95TTFBMs float64 `json:"p95_ttfb_ms"`
}

type topModel struct {
	Model        string  `json:"model"`
	ProviderID   string  `json:"provider_id"`
	RequestCount int64   `json:"request_count"`
	TotalCost    float64 `json:"total_cost"`
}

type topUser struct {
	UserID       string  `json:"user_id"`
	RequestCount int64   `json:"request_count"`
	TotalCost    float64 `json:"total_cost"`
}

type errorRate struct {
	Date       string `json:"date"`
	Total      int64  `json:"total"`
	ErrorCount int64  `json:"error_count"`
}

type usageResponse struct {
	Credentials    credentialStats `json:"credentials"`
	Tokens         tokenStats      `json:"tokens"`
	APIKeys        apiKeyStats     `json:"api_keys"`
	Identities     identityStats   `json:"identities"`
	Requests       requestStats    `json:"requests"`
	DailyRequests  []dailyRequests `json:"daily_requests"`
	TopCredentials []topCredential `json:"top_credentials"`

	// Generation-based analytics
	SpendOverTime []spendOverTime `json:"spend_over_time"`
	TokenVolumes  []tokenVolumes  `json:"token_volumes"`
	Latency       []latencyStats  `json:"latency"`
	TopModels     []topModel      `json:"top_models"`
	TopUsers      []topUser       `json:"top_users"`
	ErrorRates    []errorRate     `json:"error_rates"`
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
	p := pool.New().WithErrors().WithMaxGoroutines(12)

	// Query 1: Credentials (single query with conditional aggregation)
	p.Go(func() error {
		var row struct {
			Total   int64
			Active  int64
			Revoked int64
		}
		if err := h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE revoked_at IS NULL) AS active,
				COUNT(*) FILTER (WHERE revoked_at IS NOT NULL) AS revoked
			FROM credentials WHERE org_id = ?`, orgID).Scan(&row).Error; err != nil {
			return fmt.Errorf("credentials: %w", err)
		}
		resp.Credentials = credentialStats{Total: row.Total, Active: row.Active, Revoked: row.Revoked}
		return nil
	})

	// Query 2: Tokens (single query with conditional aggregation)
	p.Go(func() error {
		var row struct {
			Total   int64
			Active  int64
			Expired int64
			Revoked int64
		}
		if err := h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE revoked_at IS NULL AND expires_at > ?) AS active,
				COUNT(*) FILTER (WHERE revoked_at IS NULL AND expires_at <= ?) AS expired,
				COUNT(*) FILTER (WHERE revoked_at IS NOT NULL) AS revoked
			FROM tokens WHERE org_id = ?`, now, now, orgID).Scan(&row).Error; err != nil {
			return fmt.Errorf("tokens: %w", err)
		}
		resp.Tokens = tokenStats{Total: row.Total, Active: row.Active, Expired: row.Expired, Revoked: row.Revoked}
		return nil
	})

	// Query 3: API Keys + Identities (single query, two subselects)
	p.Go(func() error {
		var row struct {
			AKTotal    int64
			AKActive   int64
			IdentTotal int64
		}
		if err := h.db.Raw(`
			SELECT
				(SELECT COUNT(*) FROM api_keys WHERE org_id = ?) AS ak_total,
				(SELECT COUNT(*) FROM api_keys WHERE org_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)) AS ak_active,
				(SELECT COUNT(*) FROM identities WHERE org_id = ?) AS ident_total`,
			orgID, orgID, now, orgID).Scan(&row).Error; err != nil {
			return fmt.Errorf("api_keys_identities: %w", err)
		}
		resp.APIKeys = apiKeyStats{Total: row.AKTotal, Active: row.AKActive, Revoked: row.AKTotal - row.AKActive}
		resp.Identities = identityStats{Total: row.IdentTotal}
		return nil
	})

	// Query 4: Request counts (single query with conditional aggregation)
	p.Go(func() error {
		var row struct {
			Total     int64 `gorm:"column:total"`
			Today     int64 `gorm:"column:today"`
			Yesterday int64 `gorm:"column:yesterday"`
			Last7d    int64 `gorm:"column:last_7d"`
			Last30d   int64 `gorm:"column:last_30d"`
		}
		if err := h.db.Raw(`
			SELECT
				COUNT(*) AS total,
				COUNT(*) FILTER (WHERE created_at >= ?) AS today,
				COUNT(*) FILTER (WHERE created_at >= ? AND created_at < ?) AS yesterday,
				COUNT(*) FILTER (WHERE created_at >= ?) AS last_7d,
				COUNT(*) FILTER (WHERE created_at >= ?) AS last_30d
			FROM audit_log WHERE org_id = ? AND action = 'proxy.request'`,
			today, yesterday, today, last7d, last30d, orgID).Scan(&row).Error; err != nil {
			return fmt.Errorf("request_counts: %w", err)
		}
		resp.Requests = requestStats{
			Total:     row.Total,
			Today:     row.Today,
			Yesterday: row.Yesterday,
			Last7d:    row.Last7d,
			Last30d:   row.Last30d,
		}
		return nil
	})

	// Query 5: Daily request counts (last 30 days)
	p.Go(func() error {
		var dailyRows []struct {
			Date  time.Time
			Count int64
		}
		if err := h.db.Raw(`
			SELECT DATE(created_at) AS date, COUNT(*) AS count
			FROM audit_log
			WHERE org_id = ? AND action = 'proxy.request' AND created_at >= ?
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&dailyRows).Error; err != nil {
			return fmt.Errorf("daily_requests: %w", err)
		}
		resp.DailyRequests = make([]dailyRequests, 0, len(dailyRows))
		for _, row := range dailyRows {
			resp.DailyRequests = append(resp.DailyRequests, dailyRequests{
				Date:  row.Date.Format("2006-01-02"),
				Count: row.Count,
			})
		}
		return nil
	})

	// Query 6: Top credentials by request count (last 30 days)
	p.Go(func() error {
		var topRows []struct {
			CredentialID uuid.UUID
			Label        string
			ProviderID   string
			RequestCount int64
		}
		if err := h.db.Raw(`
			SELECT a.credential_id, c.label, c.provider_id, COUNT(*) AS request_count
			FROM audit_log a
			JOIN credentials c ON c.id = a.credential_id
			WHERE a.org_id = ? AND a.action = 'proxy.request' AND a.created_at >= ? AND a.credential_id IS NOT NULL
			GROUP BY a.credential_id, c.label, c.provider_id
			ORDER BY request_count DESC
			LIMIT 5`, orgID, last30d).Scan(&topRows).Error; err != nil {
			return fmt.Errorf("top_credentials: %w", err)
		}
		resp.TopCredentials = make([]topCredential, 0, len(topRows))
		for _, row := range topRows {
			resp.TopCredentials = append(resp.TopCredentials, topCredential{
				ID:           row.CredentialID.String(),
				Label:        row.Label,
				ProviderID:   row.ProviderID,
				RequestCount: row.RequestCount,
			})
		}
		return nil
	})

	// Query 7: Spend over time (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			Date      time.Time
			TotalCost float64
		}
		if err := h.db.Raw(`
			SELECT DATE(created_at) AS date, COALESCE(SUM(cost), 0) AS total_cost
			FROM generations
			WHERE org_id = ? AND created_at >= ?
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("spend_over_time: %w", err)
		}
		resp.SpendOverTime = make([]spendOverTime, 0, len(rows))
		for _, row := range rows {
			resp.SpendOverTime = append(resp.SpendOverTime, spendOverTime{
				Date:      row.Date.Format("2006-01-02"),
				TotalCost: row.TotalCost,
			})
		}
		return nil
	})

	// Query 8: Token volumes (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			Date         time.Time
			InputTokens  int64
			OutputTokens int64
			CachedTokens int64
		}
		if err := h.db.Raw(`
			SELECT DATE(created_at) AS date,
				COALESCE(SUM(input_tokens), 0) AS input_tokens,
				COALESCE(SUM(output_tokens), 0) AS output_tokens,
				COALESCE(SUM(cached_tokens), 0) AS cached_tokens
			FROM generations
			WHERE org_id = ? AND created_at >= ?
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("token_volumes: %w", err)
		}
		resp.TokenVolumes = make([]tokenVolumes, 0, len(rows))
		for _, row := range rows {
			resp.TokenVolumes = append(resp.TokenVolumes, tokenVolumes{
				Date:         row.Date.Format("2006-01-02"),
				InputTokens:  row.InputTokens,
				OutputTokens: row.OutputTokens,
				CachedTokens: row.CachedTokens,
			})
		}
		return nil
	})

	// Query 9: Latency stats (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			Date      time.Time
			AvgTTFBMs float64
			P95TTFBMs float64
		}
		if err := h.db.Raw(`
			SELECT DATE(created_at) AS date,
				COALESCE(AVG(ttfb_ms), 0) AS avg_ttfb_ms,
				COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY ttfb_ms), 0) AS p95_ttfb_ms
			FROM generations
			WHERE org_id = ? AND created_at >= ? AND ttfb_ms IS NOT NULL
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("latency: %w", err)
		}
		resp.Latency = make([]latencyStats, 0, len(rows))
		for _, row := range rows {
			resp.Latency = append(resp.Latency, latencyStats{
				Date:      row.Date.Format("2006-01-02"),
				AvgTTFBMs: row.AvgTTFBMs,
				P95TTFBMs: row.P95TTFBMs,
			})
		}
		return nil
	})

	// Query 10: Top models (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			Model        string
			ProviderID   string
			RequestCount int64
			TotalCost    float64
		}
		if err := h.db.Raw(`
			SELECT model, provider_id, COUNT(*) AS request_count, COALESCE(SUM(cost), 0) AS total_cost
			FROM generations
			WHERE org_id = ? AND created_at >= ? AND model != ''
			GROUP BY model, provider_id
			ORDER BY request_count DESC
			LIMIT 10`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("top_models: %w", err)
		}
		resp.TopModels = make([]topModel, 0, len(rows))
		for _, row := range rows {
			resp.TopModels = append(resp.TopModels, topModel{
				Model:        row.Model,
				ProviderID:   row.ProviderID,
				RequestCount: row.RequestCount,
				TotalCost:    row.TotalCost,
			})
		}
		return nil
	})

	// Query 11: Top users by cost (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			UserID       string
			RequestCount int64
			TotalCost    float64
		}
		if err := h.db.Raw(`
			SELECT user_id, COUNT(*) AS request_count, COALESCE(SUM(cost), 0) AS total_cost
			FROM generations
			WHERE org_id = ? AND created_at >= ? AND user_id != ''
			GROUP BY user_id
			ORDER BY total_cost DESC
			LIMIT 10`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("top_users: %w", err)
		}
		resp.TopUsers = make([]topUser, 0, len(rows))
		for _, row := range rows {
			resp.TopUsers = append(resp.TopUsers, topUser{
				UserID:       row.UserID,
				RequestCount: row.RequestCount,
				TotalCost:    row.TotalCost,
			})
		}
		return nil
	})

	// Query 12: Error rates (last 30 days, from generations)
	p.Go(func() error {
		var rows []struct {
			Date       time.Time
			Total      int64
			ErrorCount int64
		}
		if err := h.db.Raw(`
			SELECT DATE(created_at) AS date, COUNT(*) AS total,
				COUNT(*) FILTER (WHERE error_type != '' AND error_type IS NOT NULL) AS error_count
			FROM generations
			WHERE org_id = ? AND created_at >= ?
			GROUP BY DATE(created_at)
			ORDER BY date ASC`, orgID, last30d).Scan(&rows).Error; err != nil {
			return fmt.Errorf("error_rates: %w", err)
		}
		resp.ErrorRates = make([]errorRate, 0, len(rows))
		for _, row := range rows {
			resp.ErrorRates = append(resp.ErrorRates, errorRate{
				Date:       row.Date.Format("2006-01-02"),
				Total:      row.Total,
				ErrorCount: row.ErrorCount,
			})
		}
		return nil
	})

	if err := p.Wait(); err != nil {
		slog.Error("usage queries failed", "org_id", orgID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage data"})
		return
	}

	// Ensure non-null arrays
	if resp.DailyRequests == nil {
		resp.DailyRequests = []dailyRequests{}
	}
	if resp.TopCredentials == nil {
		resp.TopCredentials = []topCredential{}
	}
	if resp.SpendOverTime == nil {
		resp.SpendOverTime = []spendOverTime{}
	}
	if resp.TokenVolumes == nil {
		resp.TokenVolumes = []tokenVolumes{}
	}
	if resp.Latency == nil {
		resp.Latency = []latencyStats{}
	}
	if resp.TopModels == nil {
		resp.TopModels = []topModel{}
	}
	if resp.TopUsers == nil {
		resp.TopUsers = []topUser{}
	}
	if resp.ErrorRates == nil {
		resp.ErrorRates = []errorRate{}
	}

	writeJSON(w, http.StatusOK, resp)
}
