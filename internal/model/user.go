package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email            string     `gorm:"not null;uniqueIndex"`
	PasswordHash     string
	Name             string
	EmailConfirmedAt *time.Time
	BannedAt         *time.Time
	BanReason        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (User) TableName() string { return "users" }
