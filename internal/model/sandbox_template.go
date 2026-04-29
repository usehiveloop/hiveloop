package model

import (
	"time"

	"github.com/google/uuid"
)

type SandboxTemplate struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID          *uuid.UUID `gorm:"type:uuid;index"`                           // nil = public/platform-wide template
	Org            *Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Name           string     `gorm:"not null"`                                  // display name for users
	Description    string     `gorm:"type:text;not null;default:''"`
	Slug           string     `gorm:"not null;uniqueIndex"`                      // Daytona snapshot name
	Tags           JSON       `gorm:"type:jsonb;not null;default:'[]'"`          // user-facing tags, e.g. ["python","ml"]
	Size           string     `gorm:"not null;default:'medium'"`                 // small, medium, large, xlarge
	BaseTemplateID *uuid.UUID `gorm:"type:uuid;index"`                           // optional FK to public template used as base
	BaseTemplate   *SandboxTemplate `gorm:"foreignKey:BaseTemplateID"`
	BuildCommands  string     `gorm:"type:text;not null;default:''"`             // user's commands to run on base image
	ExternalID     *string    // provider's template/snapshot ID once built
	BaseImageRef   *string    // OCI image ref used as FROM when a child template builds on top of this row
	BuildStatus    string     `gorm:"not null;default:'pending'"`                // pending, building, ready, failed
	BuildError     *string
	BuildLogs      string `gorm:"type:text;not null;default:''"`                 // accumulated build logs (newline separated)
	Config         JSON   `gorm:"type:jsonb;not null;default:'{}'"`              // resources, env vars, etc.
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (SandboxTemplate) TableName() string { return "sandbox_templates" }
