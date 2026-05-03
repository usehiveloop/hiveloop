package model

import (
	"time"

	"github.com/google/uuid"
)

// ConversationAsset is a file uploaded by an agent (or any caller) into the
// conversation's "drive". The S3 key uses the conversation_id + caller-chosen
// folder path, so re-uploading the same path overwrites both the S3 object
// and this row (drive semantics).
type ConversationAsset struct {
	ID             uuid.UUID         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ConversationID uuid.UUID         `gorm:"type:uuid;not null;index:idx_conv_asset_conv_created,priority:1"`
	Conversation   AgentConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	OrgID          uuid.UUID         `gorm:"type:uuid;not null;index"`
	SandboxID      uuid.UUID         `gorm:"type:uuid;not null"`

	Path        string `gorm:"type:text;not null"`             // user-chosen folder, "" = root
	Filename    string `gorm:"type:text;not null"`             // sanitized basename
	Key         string `gorm:"type:text;not null;uniqueIndex"` // full S3 key
	PublicURL   string `gorm:"type:text;not null"`
	ContentType string `gorm:"type:text;not null"`
	Bytes       int64  `gorm:"not null"`

	CreatedAt time.Time `gorm:"index:idx_conv_asset_conv_created,priority:2,sort:desc"`
	UpdatedAt time.Time
}

func (ConversationAsset) TableName() string { return "conversation_assets" }
