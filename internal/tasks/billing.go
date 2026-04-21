package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	polargo "github.com/polarsource/polar-go"
	"github.com/polarsource/polar-go/models/components"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// BillingUsageEventHandler sends agent run events to Polar for metered billing.
type BillingUsageEventHandler struct {
	db    *gorm.DB
	polar *polargo.Polar
}

// NewBillingUsageEventHandler creates a billing usage event handler.
func NewBillingUsageEventHandler(db *gorm.DB, polar *polargo.Polar) *BillingUsageEventHandler {
	return &BillingUsageEventHandler{db: db, polar: polar}
}

// Handle processes a billing usage event task.
func (handler *BillingUsageEventHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload BillingUsageEventPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal billing usage event payload: %w", err)
	}

	// Look up org to check if they have a Polar customer
	var org model.Org
	if err := handler.db.Where("id = ?", payload.OrgID).First(&org).Error; err != nil {
		return fmt.Errorf("load org: %w", err)
	}

	// Free users don't have Polar customers — skip silently
	if org.PolarCustomerID == nil {
		return nil
	}

	// Send usage event to Polar
	externalID := payload.RunID.String()
	_, err := handler.polar.Events.Ingest(ctx, components.EventsIngest{
		Events: []components.Events{
			components.CreateEventsEventCreateExternalCustomer(
				components.EventCreateExternalCustomer{
					Name:               "agent_run",
					ExternalCustomerID: org.ID.String(),
					ExternalID:         &externalID,
					Metadata: map[string]components.EventMetadataInput{
						"sandbox_type": components.CreateEventMetadataInputStr(payload.SandboxType),
						"agent_id":     components.CreateEventMetadataInputStr(payload.AgentID.String()),
					},
				},
			),
		},
	})
	if err != nil {
		slog.Error("billing: failed to ingest usage event",
			"org_id", payload.OrgID,
			"run_id", payload.RunID,
			"error", err,
		)
		return fmt.Errorf("ingest usage event: %w", err)
	}

	slog.Info("billing: usage event sent",
		"org_id", payload.OrgID,
		"agent_id", payload.AgentID,
		"sandbox_type", payload.SandboxType,
		"run_id", payload.RunID,
	)

	return nil
}
