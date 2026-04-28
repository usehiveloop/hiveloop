package model

import (
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug           string    `gorm:"not null;uniqueIndex;size:64"`
	Name           string    `gorm:"not null;size:128"`
	Provider       string    `gorm:"not null;default:'';size:32;index"`
	Features       RawJSON   `gorm:"type:jsonb"`
	MonthlyCredits int64     `gorm:"not null;default:0"`
	WelcomeCredits int64     `gorm:"not null;default:0"`
	PriceCents     int64     `gorm:"not null;default:0"`
	Currency       string    `gorm:"not null;default:'USD';size:8"`
	Active         bool      `gorm:"not null;default:true"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (Plan) TableName() string { return "plans" }
