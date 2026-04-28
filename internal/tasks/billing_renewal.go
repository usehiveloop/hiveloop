package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing/subscription"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
)

// SweepBatchSize caps how many due subscriptions one sweep tick will pull
// off the queue. Renewal is rate-limited per-sub via RenewalRetryInterval,
// so the cap mostly bounds peak concurrency through asynq.
const SweepBatchSize = 200

// BillingRenewSweepHandler runs hourly. It finds subscriptions whose
// current_period_end has passed and enqueues per-subscription renewal
// tasks. The actual charging happens in BillingRenewSubscriptionHandler.
type BillingRenewSweepHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

// NewBillingRenewSweepHandler wires the sweep handler.
func NewBillingRenewSweepHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *BillingRenewSweepHandler {
	return &BillingRenewSweepHandler{db: db, enqueuer: enqueuer}
}

// Handle implements asynq.HandlerFunc.
func (h *BillingRenewSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	now := time.Now()
	ids, err := subscription.DueSubscriptionIDs(ctx, h.db, now, SweepBatchSize)
	if err != nil {
		return fmt.Errorf("billing sweep: load due subs: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	slog.InfoContext(ctx, "billing sweep: enqueuing renewals", "count", len(ids))
	var firstErr error
	for _, id := range ids {
		payload, err := json.Marshal(BillingRenewSubscriptionPayload{SubscriptionID: id})
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		task := asynq.NewTask(TypeBillingRenewSubscription, payload)
		// Unique on subscription id so a duplicate sweep tick can't enqueue
		// two simultaneous attempts for the same subscription.
		_, err = h.enqueuer.Enqueue(task,
			asynq.Queue(QueueDefault),
			asynq.MaxRetry(0), // Renew owns its retry semantics via row counter.
			asynq.Timeout(2*time.Minute),
			asynq.Unique(RenewalRetryInterval/2),
			asynq.TaskID("billing-renew-"+id.String()),
		)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RenewalRetryInterval mirrors subscription.RenewalRetryInterval so this
// package doesn't import the subscription constant directly in the dedup
// window calculation.
const RenewalRetryInterval = subscription.RenewalRetryInterval

// BillingRenewSubscriptionPayload identifies the subscription to renew.
type BillingRenewSubscriptionPayload struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

// BillingRenewSubscriptionHandler runs one renewal attempt for the
// subscription named in the payload. It owns the retry budget itself: an
// error returned here is logged but asynq does not retry — the next
// sweep tick re-enqueues if the row is still due and under the cap.
type BillingRenewSubscriptionHandler struct {
	service *subscription.Service
}

// NewBillingRenewSubscriptionHandler wires the per-sub handler.
func NewBillingRenewSubscriptionHandler(service *subscription.Service) *BillingRenewSubscriptionHandler {
	return &BillingRenewSubscriptionHandler{service: service}
}

// Handle implements asynq.HandlerFunc.
func (h *BillingRenewSubscriptionHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload BillingRenewSubscriptionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal renew payload: %w", err)
	}

	action, err := h.service.Renew(ctx, payload.SubscriptionID)
	if err != nil {
		// Returned errors are charge declines or DB faults. The row's
		// renewal_attempts counter has already been incremented (or the
		// row moved to past_due) — asynq retrying here would just bump it
		// again with the same back-off.
		slog.WarnContext(ctx, "billing renew: attempt failed",
			"subscription_id", payload.SubscriptionID, "action", action, "error", err)
		return nil
	}
	slog.InfoContext(ctx, "billing renew: applied",
		"subscription_id", payload.SubscriptionID, "action", action)
	return nil
}
