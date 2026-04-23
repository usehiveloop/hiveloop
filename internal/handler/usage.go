package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
)

// UsageHandler serves org usage and stats.
type UsageHandler struct {
	db *gorm.DB
}

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
	Requests       requestStats    `json:"requests"`
	DailyRequests  []dailyRequests `json:"daily_requests"`
	TopCredentials []topCredential `json:"top_credentials"`
	SpendOverTime  []spendOverTime `json:"spend_over_time"`
	TokenVolumes   []tokenVolumes  `json:"token_volumes"`
	Latency        []latencyStats  `json:"latency"`
	TopModels      []topModel      `json:"top_models"`
	TopUsers       []topUser       `json:"top_users"`
	ErrorRates     []errorRate     `json:"error_rates"`
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

	p.Go(func() error {
		var err error
		resp.Credentials, err = h.queryCredentials(orgID)
		return err
	})
	p.Go(func() error {
		var err error
		resp.Tokens, err = h.queryTokens(orgID, now)
		return err
	})
	p.Go(func() error {
		var err error
		resp.APIKeys, err = h.queryAPIKeys(orgID, now)
		return err
	})
	p.Go(func() error {
		var err error
		resp.Requests, err = h.queryRequests(orgID, today, yesterday, last7d, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.DailyRequests, err = h.queryDailyRequests(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.TopCredentials, err = h.queryTopCredentials(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.SpendOverTime, err = h.querySpendOverTime(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.TokenVolumes, err = h.queryTokenVolumes(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.Latency, err = h.queryLatency(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.TopModels, err = h.queryTopModels(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.TopUsers, err = h.queryTopUsers(orgID, last30d)
		return err
	})
	p.Go(func() error {
		var err error
		resp.ErrorRates, err = h.queryErrorRates(orgID, last30d)
		return err
	})

	if err := p.Wait(); err != nil {
		slog.Error("usage queries failed", "org_id", orgID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load usage data"})
		return
	}

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
