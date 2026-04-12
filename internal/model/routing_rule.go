package model

import (
	"time"

	"github.com/google/uuid"
)

// RoutingRule is a deterministic routing rule on a RouterTrigger.
// When RoutingMode="rule", the dispatcher evaluates all rules for
// the trigger in priority order. Multiple rules can match the same
// event — each matching rule's agent is dispatched. Same-priority
// agents run in parallel; lower priority waits for higher.
type RoutingRule struct {
	ID              uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RouterTriggerID uuid.UUID     `gorm:"type:uuid;not null;index"`
	RouterTrigger   RouterTrigger `gorm:"foreignKey:RouterTriggerID;constraint:OnDelete:CASCADE"`
	Priority        int           `gorm:"not null;default:1"` // 1 = highest priority
	Conditions      RawJSON       `gorm:"type:jsonb"`         // nil = always matches (catch-all)
	AgentID         uuid.UUID     `gorm:"type:uuid;not null"`
	Agent           Agent         `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	CreatedAt       time.Time
}

func (RoutingRule) TableName() string { return "routing_rules" }
