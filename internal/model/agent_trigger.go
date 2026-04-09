package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// AgentTrigger links an agent to one or more webhook event triggers on a specific connection.
// When a trigger fires and conditions match, context actions are gathered and
// the agent is kicked off with the enriched payload.
type AgentTrigger struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org            Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	AgentID        uuid.UUID      `gorm:"type:uuid;not null;index"`
	Agent          Agent          `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`
	ConnectionID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	Connection     Connection     `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	TriggerKeys    pq.StringArray `gorm:"type:text[];not null"` // e.g. {"issues.opened","issues.reopened"}, validated against catalog
	Enabled        bool           `gorm:"not null;default:true"`
	Conditions     RawJSON        `gorm:"type:jsonb"` // TriggerMatch JSON
	ContextActions RawJSON        `gorm:"type:jsonb"` // []ContextAction JSON
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (AgentTrigger) TableName() string { return "agent_triggers" }

// TriggerMatch defines filtering conditions on the webhook payload.
type TriggerMatch struct {
	Mode       string             `json:"mode"`       // "all" (AND) or "any" (OR)
	Conditions []TriggerCondition `json:"conditions"`
}

// TriggerCondition is a single filter rule applied to the webhook payload.
type TriggerCondition struct {
	Path     string `json:"path"`     // dot-path into payload, e.g. "repository.full_name"
	Operator string `json:"operator"` // equals, not_equals, one_of, not_one_of, contains, not_contains, matches, exists, not_exists
	Value    any    `json:"value"`    // string or []string depending on operator
}

// ContextAction defines a READ action to execute for gathering context before triggering the agent.
// Params support two resolution modes:
//   - "$refs.x" — static entity ref extracted from the webhook payload (resolved before any fetches)
//   - "{{step_name.field}}" — interpolated from a previously fetched context step (resolved after earlier steps)
type ContextAction struct {
	As       string         `json:"as"`                  // name in the context bag (used in prompt template + referenced by later steps)
	Action   string         `json:"action"`              // catalog action key, e.g. "issues_get"
	Ref      string         `json:"ref,omitempty"`       // resource ref — auto-fills params from resource's ref_bindings
	Params   map[string]any `json:"params,omitempty"`    // explicit/override params (supports $refs.x and {{step.field}} templates)
	Optional bool           `json:"optional,omitempty"`  // if true, failure doesn't block the trigger
	OnlyWhen []string       `json:"only_when,omitempty"` // only run when the event matches these trigger keys
}
