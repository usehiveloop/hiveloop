package model

import (
	"time"

	"github.com/google/uuid"
)

type ConversationEvent struct {
	ID             uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID         `gorm:"type:uuid;not null;index"`
	ConversationID uuid.UUID         `gorm:"type:uuid;not null;index:idx_event_conv_created"`
	Conversation   AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	EventType      string            `gorm:"not null"` // message_received, response_completed, turn_completed, conversation_ended, etc.
	Payload        JSON              `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt      time.Time         `gorm:"index:idx_event_conv_created"`
}

func (ConversationEvent) TableName() string { return "conversation_events" }
