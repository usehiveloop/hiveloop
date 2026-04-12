package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RoutingDecision is an auditable record of every routing decision Zira makes.
// Stored as structured data (not LLM memory) so it can be queried, inspected,
// and injected as few-shot examples into future triage prompts.
type RoutingDecision struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID           uuid.UUID      `gorm:"type:uuid;not null;index"`
	RouterTriggerID uuid.UUID      `gorm:"type:uuid;not null;index"`
	RoutingMode     string         `gorm:"not null"` // "rule" or "triage"
	EventType       string         `gorm:"not null"` // e.g. "app_mention", "pull_request.opened"
	ResourceKey     string         `gorm:"not null;default:''"`
	IntentSummary   string         `gorm:"type:text"`       // LLM-generated summary of intent (triage only)
	SelectedAgents  pq.StringArray `gorm:"type:text[]"`     // agent IDs that were dispatched
	EnrichmentSteps int            `gorm:"not null;default:0"`
	TurnCount       int            `gorm:"not null;default:0"` // LLM turns used (triage only)
	LatencyMs       int            `gorm:"not null;default:0"`
	CreatedAt       time.Time
}

func (RoutingDecision) TableName() string { return "routing_decisions" }
