package model

import (
	"time"

	"github.com/google/uuid"
)

type ChatSession struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          uuid.UUID  `gorm:"type:uuid;not null;index"`
	AgentID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_chat_sessions_user_agent"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;index:idx_chat_sessions_user_agent;index:idx_chat_sessions_user_updated"`
	LastResponseID string     `gorm:"not null;default:''"`
	CreatedAt      time.Time  `gorm:"not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"not null;default:now();index:idx_chat_sessions_user_updated,sort:desc"`
	DeletedAt      *time.Time `gorm:"index"`
}

func (ChatSession) TableName() string { return "chat_sessions" }

type ChatMessage struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID        uuid.UUID `gorm:"type:uuid;not null;index:idx_chat_messages_session_created"`
	Role             string    `gorm:"not null"`
	Content          string    `gorm:"type:text;not null;default:''"`
	HermesResponseID string    `gorm:"not null;default:''"`
	CreatedAt        time.Time `gorm:"not null;default:now();index:idx_chat_messages_session_created"`
}

func (ChatMessage) TableName() string { return "chat_messages" }
