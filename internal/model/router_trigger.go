package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RouterTrigger connects a router to a webhook connection with specific
// event keys. When a matching webhook arrives, the trigger fires and the
// routing pipeline runs — either deterministic rules or LLM triage.
type RouterTrigger struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID      `gorm:"type:uuid;not null;index"`
	RouterID       uuid.UUID      `gorm:"type:uuid;not null;index"`
	Router         Router         `gorm:"foreignKey:RouterID;constraint:OnDelete:CASCADE"`
	ConnectionID   uuid.UUID      `gorm:"type:uuid;not null;index"`
	InConnection   InConnection   `gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
	TriggerKeys    pq.StringArray `gorm:"type:text[];not null"`
	Enabled        bool           `gorm:"not null;default:true"`
	RoutingMode    string         `gorm:"not null;default:'triage'"` // "rule" or "triage"
	ContextActions RawJSON        `gorm:"type:jsonb"`                // base context actions run before routing
	EnrichCrossReferences bool    `gorm:"not null;default:false"`    // enable LLM cross-connection enrichment
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (RouterTrigger) TableName() string { return "router_triggers" }
