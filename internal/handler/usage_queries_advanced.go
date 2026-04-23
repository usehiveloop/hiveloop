package handler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (h *UsageHandler) queryLatency(orgID uuid.UUID, last30d time.Time) ([]latencyStats, error) {
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
		return nil, fmt.Errorf("latency: %w", err)
	}
	result := make([]latencyStats, 0, len(rows))
	for _, row := range rows {
		result = append(result, latencyStats{
			Date:      row.Date.Format("2006-01-02"),
			AvgTTFBMs: row.AvgTTFBMs,
			P95TTFBMs: row.P95TTFBMs,
		})
	}
	return result, nil
}

func (h *UsageHandler) queryTopModels(orgID uuid.UUID, last30d time.Time) ([]topModel, error) {
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
		return nil, fmt.Errorf("top_models: %w", err)
	}
	result := make([]topModel, 0, len(rows))
	for _, row := range rows {
		result = append(result, topModel{
			Model:        row.Model,
			ProviderID:   row.ProviderID,
			RequestCount: row.RequestCount,
			TotalCost:    row.TotalCost,
		})
	}
	return result, nil
}

func (h *UsageHandler) queryTopUsers(orgID uuid.UUID, last30d time.Time) ([]topUser, error) {
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
		return nil, fmt.Errorf("top_users: %w", err)
	}
	result := make([]topUser, 0, len(rows))
	for _, row := range rows {
		result = append(result, topUser{
			UserID:       row.UserID,
			RequestCount: row.RequestCount,
			TotalCost:    row.TotalCost,
		})
	}
	return result, nil
}

func (h *UsageHandler) queryErrorRates(orgID uuid.UUID, last30d time.Time) ([]errorRate, error) {
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
		return nil, fmt.Errorf("error_rates: %w", err)
	}
	result := make([]errorRate, 0, len(rows))
	for _, row := range rows {
		result = append(result, errorRate{
			Date:       row.Date.Format("2006-01-02"),
			Total:      row.Total,
			ErrorCount: row.ErrorCount,
		})
	}
	return result, nil
}
