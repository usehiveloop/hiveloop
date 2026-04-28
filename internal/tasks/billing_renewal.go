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

// Cap on rows enqueued per sweep tick. Per-sub rate-limiting via
// RenewalRetryInterval bounds the actual concurrency below this.
const SweepBatchSize = 200

type BillingRenewSweepHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewBillingRenewSweepHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *BillingRenewSweepHandler {
	return &BillingRenewSweepHandler{db: db, enqueuer: enqueuer}
}

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
		// MaxRetry=0: Service.Renew owns retry semantics via row counter.
		// Unique on sub id so duplicate sweep ticks can't double-enqueue.
		_, err = h.enqueuer.Enqueue(task,
			asynq.Queue(QueueDefault),
			asynq.MaxRetry(0),
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

const RenewalRetryInterval = subscription.RenewalRetryInterval

type BillingRenewSubscriptionPayload struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

type BillingRenewSubscriptionHandler struct {
	service *subscription.Service
}

func NewBillingRenewSubscriptionHandler(service *subscription.Service) *BillingRenewSubscriptionHandler {
	return &BillingRenewSubscriptionHandler{service: service}
}

// We swallow charge errors here so asynq doesn't double-attempt within
// the same hour; the next sweep tick re-enqueues if still due + under cap.
func (h *BillingRenewSubscriptionHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload BillingRenewSubscriptionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal renew payload: %w", err)
	}

	action, err := h.service.Renew(ctx, payload.SubscriptionID)
	if err != nil {
		slog.WarnContext(ctx, "billing renew: attempt failed",
			"subscription_id", payload.SubscriptionID, "action", action, "error", err)
		return nil
	}
	slog.InfoContext(ctx, "billing renew: applied",
		"subscription_id", payload.SubscriptionID, "action", action)
	return nil
}
