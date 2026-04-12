package model

import (
	"time"

	"github.com/google/uuid"
)

// RouterConversation tracks an active conversation between a routed agent
// and a resource (Slack thread, GitHub issue, Linear ticket, etc.).
// Used for thread affinity: when a follow-up event arrives for the same
// resource_key, the dispatcher skips routing and continues the existing
// conversation instead.
type RouterConversation struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID `gorm:"type:uuid;not null;index"`
	RouterTriggerID      uuid.UUID `gorm:"type:uuid;not null"`
	AgentID              uuid.UUID `gorm:"type:uuid;not null;index"`
	ConnectionID         uuid.UUID `gorm:"type:uuid;not null"`
	ResourceKey          string    `gorm:"not null;index:idx_rconv_lookup"`
	BridgeConversationID string    `gorm:"not null"`
	SandboxID            uuid.UUID `gorm:"type:uuid;not null"`
	Status               string    `gorm:"not null;default:'active'"` // active, closed
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (RouterConversation) TableName() string { return "router_conversations" }
