package handler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (h *UsageHandler) queryDailyRequests(orgID uuid.UUID, last30d time.Time) ([]dailyRequests, error) {
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
		return nil, fmt.Errorf("daily_requests: %w", err)
	}
	result := make([]dailyRequests, 0, len(dailyRows))
	for _, row := range dailyRows {
		result = append(result, dailyRequests{
			Date:  row.Date.Format("2006-01-02"),
			Count: row.Count,
		})
	}
	return result, nil
}

func (h *UsageHandler) queryTopCredentials(orgID uuid.UUID, last30d time.Time) ([]topCredential, error) {
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
		return nil, fmt.Errorf("top_credentials: %w", err)
	}
	result := make([]topCredential, 0, len(topRows))
	for _, row := range topRows {
		result = append(result, topCredential{
			ID:           row.CredentialID.String(),
			Label:        row.Label,
			ProviderID:   row.ProviderID,
			RequestCount: row.RequestCount,
		})
	}
	return result, nil
}

func (h *UsageHandler) querySpendOverTime(orgID uuid.UUID, last30d time.Time) ([]spendOverTime, error) {
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
		return nil, fmt.Errorf("spend_over_time: %w", err)
	}
	result := make([]spendOverTime, 0, len(rows))
	for _, row := range rows {
		result = append(result, spendOverTime{
			Date:      row.Date.Format("2006-01-02"),
			TotalCost: row.TotalCost,
		})
	}
	return result, nil
}

func (h *UsageHandler) queryTokenVolumes(orgID uuid.UUID, last30d time.Time) ([]tokenVolumes, error) {
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
		return nil, fmt.Errorf("token_volumes: %w", err)
	}
	result := make([]tokenVolumes, 0, len(rows))
	for _, row := range rows {
		result = append(result, tokenVolumes{
			Date:         row.Date.Format("2006-01-02"),
			InputTokens:  row.InputTokens,
			OutputTokens: row.OutputTokens,
			CachedTokens: row.CachedTokens,
		})
	}
	return result, nil
}
