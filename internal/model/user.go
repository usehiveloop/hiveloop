package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email            string     `gorm:"not null;uniqueIndex"`
	PasswordHash     string     `gorm:"not null"`
	Name             string
	EmailConfirmedAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (User) TableName() string { return "users" }
