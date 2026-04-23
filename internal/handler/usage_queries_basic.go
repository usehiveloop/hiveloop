package handler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (h *UsageHandler) queryCredentials(orgID uuid.UUID) (credentialStats, error) {
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
		return credentialStats{}, fmt.Errorf("credentials: %w", err)
	}
	return credentialStats{Total: row.Total, Active: row.Active, Revoked: row.Revoked}, nil
}

func (h *UsageHandler) queryTokens(orgID uuid.UUID, now time.Time) (tokenStats, error) {
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
		return tokenStats{}, fmt.Errorf("tokens: %w", err)
	}
	return tokenStats{Total: row.Total, Active: row.Active, Expired: row.Expired, Revoked: row.Revoked}, nil
}

func (h *UsageHandler) queryAPIKeys(orgID uuid.UUID, now time.Time) (apiKeyStats, error) {
	var row struct {
		AKTotal  int64
		AKActive int64
	}
	if err := h.db.Raw(`
		SELECT
			(SELECT COUNT(*) FROM api_keys WHERE org_id = ?) AS ak_total,
			(SELECT COUNT(*) FROM api_keys WHERE org_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)) AS ak_active`,
		orgID, orgID, now).Scan(&row).Error; err != nil {
		return apiKeyStats{}, fmt.Errorf("api_keys: %w", err)
	}
	return apiKeyStats{Total: row.AKTotal, Active: row.AKActive, Revoked: row.AKTotal - row.AKActive}, nil
}

func (h *UsageHandler) queryRequests(orgID uuid.UUID, today, yesterday, last7d, last30d time.Time) (requestStats, error) {
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
		return requestStats{}, fmt.Errorf("request_counts: %w", err)
	}
	return requestStats{
		Total:     row.Total,
		Today:     row.Today,
		Yesterday: row.Yesterday,
		Last7d:    row.Last7d,
		Last30d:   row.Last30d,
	}, nil
}
