package model

import (
	"time"

	"github.com/google/uuid"
)

// Plan is a billing plan. Plans are seeded and managed administratively — the
// catalog is small and change-controlled. A subscription references a plan
// by PlanID; the plan.Slug is the stable identifier used in UIs.
//
// ProviderPlanID is nullable now: we manage the recurring lifecycle ourselves
// and only use the provider to charge a saved payment method, so most plans
// have no upstream plan_code at all.
type Plan struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Slug           string    `gorm:"not null;uniqueIndex;size:64"`
	Name           string    `gorm:"not null;size:128"`
	Provider       string    `gorm:"not null;default:'';size:32;index"` // empty = provider-agnostic
	ProviderPlanID *string   `gorm:"size:128;index"`
	Features       RawJSON   `gorm:"type:jsonb"`
	MonthlyCredits int64     `gorm:"not null;default:0"`
	WelcomeCredits int64     `gorm:"not null;default:0"`
	PriceCents     int64     `gorm:"not null;default:0"`
	Currency       string    `gorm:"not null;default:'USD';size:8"`
	Active         bool      `gorm:"not null;default:true"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TableName returns the plans table name.
func (Plan) TableName() string { return "plans" }
