package model

import (
	"time"

	"github.com/google/uuid"
)

// DriveAsset tracks a file stored in S3 that belongs to an agent's drive.
type DriveAsset struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index:idx_drive_asset_org" json:"org_id"`
	Org         *Org      `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE" json:"-"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;index:idx_drive_asset_agent" json:"agent_id"`
	Agent       *Agent    `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`

	Filename    string `gorm:"not null" json:"filename"`
	ContentType string `gorm:"not null" json:"content_type"`
	Size        int64  `gorm:"not null" json:"size"`
	S3Key       string `gorm:"not null;uniqueIndex" json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DriveAsset) TableName() string { return "drive_assets" }
