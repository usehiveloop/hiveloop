package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentTriggerDelivery records a successful trigger injection into an employee
// runtime. AgentTrigger remains the source of truth for trigger configuration;
// this table stores only per-run correlation data.
type AgentTriggerDelivery struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_agent_created"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	AgentID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_agent_created"`
	Agent   Agent     `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE"`

	TriggerID uuid.UUID    `gorm:"type:uuid;not null;index"`
	Trigger   AgentTrigger `gorm:"foreignKey:TriggerID;constraint:OnDelete:CASCADE"`

	ConnectionID *uuid.UUID   `gorm:"type:uuid;index"`
	Connection   *InConnection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:SET NULL"`

	DeliveryID string  `gorm:"type:text;not null;index"`
	EventKey   string  `gorm:"type:text;not null;default:'';index"`
	ResourceKey string `gorm:"type:text;not null;default:'';index"`

	ConversationID        uuid.UUID         `gorm:"type:uuid;not null;index"`
	Conversation          AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	RuntimeConversationID string            `gorm:"type:text;not null;default:'';index"`
	RuntimeSessionID      string            `gorm:"type:text;not null;default:'';index"`
	RuntimeStreamID       string            `gorm:"type:text;not null;default:''"`
	RuntimeTraceID        string            `gorm:"type:text;not null;default:''"`
	RuntimeTurnID         string            `gorm:"type:text;not null;default:''"`

	Payload RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time `gorm:"index:idx_trigger_delivery_org_agent_created"`
}

func (AgentTriggerDelivery) TableName() string { return "agent_trigger_deliveries" }
