package model

import (
	"time"

	"github.com/google/uuid"
)

type OrgMembership struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_user_org"`
	OrgID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_user_org"`
	Role      string    `gorm:"not null;default:'admin'"` // "admin" or "viewer"
	User      User      `gorm:"foreignKey:UserID"`
	Org       Org       `gorm:"foreignKey:OrgID"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (OrgMembership) TableName() string { return "org_memberships" }
