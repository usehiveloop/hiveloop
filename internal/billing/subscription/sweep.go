package subscription

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// DueSubscriptionIDs returns subscription IDs that are eligible for a
// renewal attempt right now. Eligibility:
//
//   - status = active (canceled and past_due rows are not retried by sweep)
//   - current_period_end <= now (the period has actually ended)
//   - renewal_attempts < MaxRenewalAttempts (under the per-period cap)
//   - last_renewal_attempt_at is null or older than RenewalRetryInterval
//     (rate-limit so a fast cron doesn't burn the budget in seconds)
//
// The query returns at most `limit` rows so a single sweep tick can't
// fan out more work than the worker can handle. Callers that want to
// sweep everything can page by re-calling until empty.
func DueSubscriptionIDs(ctx context.Context, db *gorm.DB, now time.Time, limit int) ([]uuid.UUID, error) {
	cutoff := now.Add(-RenewalRetryInterval)

	var ids []uuid.UUID
	err := db.WithContext(ctx).Model(&model.Subscription{}).
		Where("status = ?", string(billing.StatusActive)).
		Where("current_period_end <= ?", now).
		Where("renewal_attempts < ?", MaxRenewalAttempts).
		Where("(last_renewal_attempt_at IS NULL OR last_renewal_attempt_at < ?)", cutoff).
		Order("current_period_end ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}
