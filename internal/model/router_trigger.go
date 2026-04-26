package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RouterTrigger connects a router to an event source — a webhook connection,
// an HTTP endpoint, or a cron schedule. When the trigger fires, the routing
// pipeline runs — either deterministic rules or LLM triage — and dispatches
// matching agents.
//
// TriggerType determines how the trigger is activated:
//   - "webhook" (default): fires when a matching webhook event arrives on the
//     linked connection. Requires ConnectionID and TriggerKeys.
//   - "http": fires when an HTTP request arrives at the trigger's unique URL
//     (/incoming/triggers/{id}). No connection required. The request body
//     becomes the payload for rule evaluation.
//   - "cron": fires on a cron schedule. No connection or webhook involved.
//     Requires CronSchedule. The poller advances NextRunAt after each fire.
type RouterTrigger struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	RouterID       uuid.UUID      `gorm:"type:uuid;not null;index"`
	Router         Router         `gorm:"foreignKey:RouterID;constraint:OnDelete:CASCADE"`
	ConnectionID   *uuid.UUID     `gorm:"type:uuid;index"`           // nil for http/cron triggers
	InConnection   InConnection   `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	TriggerKeys    pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	Enabled        bool           `gorm:"not null;default:true"`
	TriggerType    string         `gorm:"not null;default:'webhook'"` // "webhook", "http", "cron"
	RoutingMode    string         `gorm:"not null;default:'triage'"` // "rule" or "triage"
	ContextActions RawJSON        `gorm:"type:jsonb"`                // base context actions run before routing
	EnrichCrossReferences bool    `gorm:"not null;default:false"`    // enable LLM cross-connection enrichment

	// Cron-specific fields (only used when TriggerType = "cron").
	CronSchedule string     `gorm:"not null;default:''"` // standard cron expression, e.g. "0 9 * * 1-5"
	NextRunAt    *time.Time `gorm:"index"`               // pre-computed next fire time; poller queries this
	LastRunAt    *time.Time                               // last successful fire

	// HTTP-specific fields (only used when TriggerType = "http").
	// If empty, the trigger relies on the unguessable UUID for security.
	// When set, stores a bcrypt hash of the user-supplied shared secret. The
	// HTTP handler accepts the plaintext secret in any of:
	//   Authorization: Bearer <secret>, X-Api-Key, X-Webhook-Secret, ?secret=
	// and verifies via bcrypt.CompareHashAndPassword.
	SecretKey string `gorm:"not null;default:''"`

	// Instructions sent to the agent when the trigger fires (cron/http only).
	// For webhook triggers, instructions come from enrichment. For cron/http,
	// this field provides the base prompt template. Supports $refs.x substitution.
	Instructions string `gorm:"type:text;not null;default:''"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RouterTrigger) TableName() string { return "router_triggers" }
