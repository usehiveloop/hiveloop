package model

import (
	"time"

	"github.com/google/uuid"
)

type ConversationEvent struct {
	ID                   uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                uuid.UUID         `gorm:"type:uuid;not null;index"`
	ConversationID       uuid.UUID         `gorm:"type:uuid;not null;index:idx_event_conv_created"`
	Conversation         AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	EventID              string            `gorm:"not null"`       // bridge event_id (UUID)
	EventType            string            `gorm:"not null;index"` // bridge event_type
	AgentID              string            `gorm:"not null;index"` // bridge agent_id
	BridgeConversationID string            `gorm:"not null;index"` // bridge conversation_id
	Timestamp            time.Time         `gorm:"not null"`       // bridge timestamp (UTC)
	SequenceNumber       int64             `gorm:"not null"`       // bridge sequence_number (monotonic)
	Data                 RawJSON           `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt            time.Time         `gorm:"index:idx_event_conv_created"`
}

func (ConversationEvent) TableName() string { return "conversation_events" }
