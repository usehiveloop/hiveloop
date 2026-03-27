package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
)

// ReportingHandler serves analytics reporting from the generations table.
type ReportingHandler struct {
	db *gorm.DB
}

// NewReportingHandler creates a new reporting handler.
func NewReportingHandler(db *gorm.DB) *ReportingHandler {
	return &ReportingHandler{db: db}
}

type reportRow struct {
	Period          string  `json:"period"`
	Model           string  `json:"model,omitempty"`
	ProviderID      string  `json:"provider_id,omitempty"`
	CredentialID    string  `json:"credential_id,omitempty"`
	UserID          string  `json:"user_id,omitempty"`
	IdentityID      string  `json:"identity_id,omitempty"`
	RequestCount    int64   `json:"request_count"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	TotalCost       float64 `json:"total_cost"`
	AvgTTFBMs       float64 `json:"avg_ttfb_ms"`
	P50TTFBMs       float64 `json:"p50_ttfb_ms"`
	P95TTFBMs       float64 `json:"p95_ttfb_ms"`
	ErrorCount      int64   `json:"error_count"`
}

// validGroupBys defines allowed group_by values and their SQL expressions.
var validGroupBys = map[string]string{
	"model":      "model",
	"provider":   "provider_id",
	"credential": "credential_id",
	"user":       "user_id",
	"identity":   "identity_id",
}

// Get handles GET /v1/reporting.
// @Summary Get analytics report
// @Description Returns aggregated analytics from generations with flexible grouping and filtering.
// @Tags reporting
// @Produce json
// @Param group_by query string false "Comma-separated grouping dimensions: model, provider, credential, user, identity"
// @Param date_part query string false "Time granularity: hour or day (default: day)"
// @Param start_date query string false "Start date inclusive (YYYY-MM-DD)"
// @Param end_date query string false "End date inclusive (YYYY-MM-DD)"
// @Param model query string false "Filter by model name"
// @Param provider_id query string false "Filter by provider ID"
// @Param credential_id query string false "Filter by credential ID"
// @Param user_id query string false "Filter by user ID"
// @Param tags query string false "Filter by tag (comma-separated, OR)"
// @Success 200 {array} reportRow
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/reporting [get]
func (h *ReportingHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization context"})
		return
	}

	// Parse date_part
	datePart := r.URL.Query().Get("date_part")
	if datePart == "" {
		datePart = "day"
	}
	if datePart != "hour" && datePart != "day" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date_part must be 'hour' or 'day'"})
		return
	}

	// Parse group_by
	var groupCols []string
	var selectGroupCols []string
	if gb := r.URL.Query().Get("group_by"); gb != "" {
		for _, g := range strings.Split(gb, ",") {
			g = strings.TrimSpace(g)
			col, ok := validGroupBys[g]
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid group_by: %q", g)})
				return
			}
			groupCols = append(groupCols, col)
			selectGroupCols = append(selectGroupCols, col)
		}
	}

	// Build SELECT
	periodExpr := fmt.Sprintf("date_trunc('%s', created_at) AS period", datePart)

	selectParts := []string{
		periodExpr,
	}
	selectParts = append(selectParts, selectGroupCols...)
	selectParts = append(selectParts,
		"COUNT(*) AS request_count",
		"COALESCE(SUM(input_tokens), 0) AS input_tokens",
		"COALESCE(SUM(output_tokens), 0) AS output_tokens",
		"COALESCE(SUM(cached_tokens), 0) AS cached_tokens",
		"COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens",
		"COALESCE(SUM(cost), 0) AS total_cost",
		"COALESCE(AVG(ttfb_ms), 0) AS avg_ttfb_ms",
		"COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY ttfb_ms), 0) AS p50_ttfb_ms",
		"COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY ttfb_ms), 0) AS p95_ttfb_ms",
		"COUNT(*) FILTER (WHERE error_type != '' AND error_type IS NOT NULL) AS error_count",
	)

	// Build GROUP BY
	groupParts := []string{"period"}
	groupParts = append(groupParts, groupCols...)

	query := fmt.Sprintf(
		"SELECT %s FROM generations WHERE org_id = ?",
		strings.Join(selectParts, ", "),
	)
	args := []any{org.ID}

	// Date range filters
	if sd := r.URL.Query().Get("start_date"); sd != "" {
		t, err := time.Parse("2006-01-02", sd)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start_date format (YYYY-MM-DD)"})
			return
		}
		query += " AND created_at >= ?"
		args = append(args, t)
	}
	if ed := r.URL.Query().Get("end_date"); ed != "" {
		t, err := time.Parse("2006-01-02", ed)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid end_date format (YYYY-MM-DD)"})
			return
		}
		// End date is inclusive — add 1 day
		query += " AND created_at < ?"
		args = append(args, t.AddDate(0, 0, 1))
	}

	// Dimension filters
	if m := r.URL.Query().Get("model"); m != "" {
		query += " AND model = ?"
		args = append(args, m)
	}
	if p := r.URL.Query().Get("provider_id"); p != "" {
		query += " AND provider_id = ?"
		args = append(args, p)
	}
	if c := r.URL.Query().Get("credential_id"); c != "" {
		query += " AND credential_id = ?"
		args = append(args, c)
	}
	if u := r.URL.Query().Get("user_id"); u != "" {
		query += " AND user_id = ?"
		args = append(args, u)
	}
	if t := r.URL.Query().Get("tags"); t != "" {
		tags := strings.Split(t, ",")
		query += " AND tags && ARRAY[" + strings.Repeat("?,", len(tags)-1) + "?]::text[]"
		for _, tag := range tags {
			args = append(args, strings.TrimSpace(tag))
		}
	}

	query += fmt.Sprintf(
		" GROUP BY %s ORDER BY period DESC",
		strings.Join(groupParts, ", "),
	)

	// Limit results
	query += " LIMIT 1000"

	type rawRow struct {
		Period          time.Time `gorm:"column:period"`
		Model           string    `gorm:"column:model"`
		ProviderID      string    `gorm:"column:provider_id"`
		CredentialID    string    `gorm:"column:credential_id"`
		UserID          string    `gorm:"column:user_id"`
		IdentityID      string    `gorm:"column:identity_id"`
		RequestCount    int64     `gorm:"column:request_count"`
		InputTokens     int64     `gorm:"column:input_tokens"`
		OutputTokens    int64     `gorm:"column:output_tokens"`
		CachedTokens    int64     `gorm:"column:cached_tokens"`
		ReasoningTokens int64     `gorm:"column:reasoning_tokens"`
		TotalCost       float64   `gorm:"column:total_cost"`
		AvgTTFBMs       float64   `gorm:"column:avg_ttfb_ms"`
		P50TTFBMs       float64   `gorm:"column:p50_ttfb_ms"`
		P95TTFBMs       float64   `gorm:"column:p95_ttfb_ms"`
		ErrorCount      int64     `gorm:"column:error_count"`
	}

	var rows []rawRow
	if err := h.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reporting query failed"})
		return
	}

	// Format response
	format := "2006-01-02"
	if datePart == "hour" {
		format = "2006-01-02T15:00:00Z"
	}

	result := make([]reportRow, len(rows))
	for i, row := range rows {
		result[i] = reportRow{
			Period:          row.Period.Format(format),
			Model:           row.Model,
			ProviderID:      row.ProviderID,
			CredentialID:    row.CredentialID,
			UserID:          row.UserID,
			IdentityID:      row.IdentityID,
			RequestCount:    row.RequestCount,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			CachedTokens:    row.CachedTokens,
			ReasoningTokens: row.ReasoningTokens,
			TotalCost:       row.TotalCost,
			AvgTTFBMs:       row.AvgTTFBMs,
			P50TTFBMs:       row.P50TTFBMs,
			P95TTFBMs:       row.P95TTFBMs,
			ErrorCount:      row.ErrorCount,
		}
	}

	if result == nil {
		result = []reportRow{}
	}

	writeJSON(w, http.StatusOK, result)
}
