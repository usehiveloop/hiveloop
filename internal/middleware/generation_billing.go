package middleware

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

// enqueueBillingTokenSpend submits the async deduction for a platform-keys
// LLM call. Enqueue failures are logged but don't affect the proxy response
// — the user got their inference; a missed deduction is a billing leak we'd
// rather catch and fix than block traffic on.
//
// Split out of generation.go so the observability flow stays readable and
// the file stays under the 300-line ceiling.
func enqueueBillingTokenSpend(ctx context.Context, enqueuer enqueue.TaskEnqueuer, orgID string, gen model.Generation) {
	parsedOrgID, err := uuid.Parse(orgID)
	if err != nil {
		slog.ErrorContext(ctx, "generation: invalid org id in claims, skipping spend enqueue",
			"org_id", orgID, "generation_id", gen.ID, "error", err)
		return
	}
	task, err := tasks.NewBillingTokenSpendTask(tasks.BillingTokenSpendPayload{
		OrgID:        parsedOrgID,
		GenerationID: gen.ID,
		Model:        gen.Model,
		InputTokens:  int64(gen.InputTokens),
		OutputTokens: int64(gen.OutputTokens),
	})
	if err != nil {
		slog.ErrorContext(ctx, "generation: failed to build billing task",
			"generation_id", gen.ID, "error", err)
		return
	}
	if _, err := enqueuer.Enqueue(task); err != nil {
		slog.ErrorContext(ctx, "generation: failed to enqueue billing task",
			"generation_id", gen.ID, "error", err)
	}
}
