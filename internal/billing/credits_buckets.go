package billing

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// bucket tracks the residual amount of a single positive ledger entry as
// FIFO replay attributes spends and prior expiry offsets.
type bucket struct {
	entryID   uuid.UUID
	expiresAt *time.Time
	remaining int64
	createdAt time.Time
}

// replayBuckets walks the ledger in chronological order and returns one
// bucket per positive entry with its post-attribution remaining amount.
//
// Spend attribution rule: deduct from the bucket with the earliest ExpiresAt
// first (NULL — permanent — last); tiebreak on oldest CreatedAt. Cascades
// across buckets if a single spend exceeds one bucket's remaining.
//
// Existing expiry entries (reason="expiry", ref_type="expired_grant") are
// applied to their referenced bucket, modelling the effect of a prior sweep.
func replayBuckets(entries []model.CreditLedgerEntry) []*bucket {
	var buckets []*bucket
	byID := make(map[uuid.UUID]*bucket)

	for _, e := range entries {
		switch {
		case e.Amount > 0:
			b := &bucket{
				entryID:   e.ID,
				expiresAt: e.ExpiresAt,
				remaining: e.Amount,
				createdAt: e.CreatedAt,
			}
			buckets = append(buckets, b)
			byID[e.ID] = b
		case e.Amount < 0 && e.Reason == ReasonExpiry && e.RefType == RefTypeExpiredGrant:
			refID, err := uuid.Parse(e.RefID)
			if err != nil {
				continue
			}
			if b, ok := byID[refID]; ok {
				b.remaining += e.Amount // amount is negative
				if b.remaining < 0 {
					b.remaining = 0
				}
			}
		case e.Amount < 0:
			deductSpend(buckets, -e.Amount)
		}
	}
	return buckets
}

// deductSpend takes |amount| from the eligible buckets in attribution order
// (earliest expiry first, NULL last; tiebreak oldest createdAt). Cascades.
func deductSpend(buckets []*bucket, amount int64) {
	if amount <= 0 {
		return
	}
	eligible := make([]*bucket, 0, len(buckets))
	for _, b := range buckets {
		if b.remaining > 0 {
			eligible = append(eligible, b)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		ei, ej := eligible[i].expiresAt, eligible[j].expiresAt
		switch {
		case ei != nil && ej == nil:
			return true
		case ei == nil && ej != nil:
			return false
		case ei != nil && ej != nil && !ei.Equal(*ej):
			return ei.Before(*ej)
		default:
			return eligible[i].createdAt.Before(eligible[j].createdAt)
		}
	})
	left := amount
	for _, b := range eligible {
		if left == 0 {
			return
		}
		take := b.remaining
		if take > left {
			take = left
		}
		b.remaining -= take
		left -= take
	}
}
