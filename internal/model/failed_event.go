package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	FailedEventStatusPending   = "pending"
	FailedEventStatusRetried   = "retried"
	FailedEventStatusDiscarded = "discarded"
)

type FailedEvent struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID `gorm:"type:uuid;not null;index"`
	TriggerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	EventType     string    `gorm:"not null;index"`
	Payload       RawJSON   `gorm:"type:jsonb;not null"`
	Error         string    `gorm:"type:text;not null"`
	AttemptCount  int       `gorm:"not null"`
	FailedAt      time.Time `gorm:"not null;index"`
	Status        string    `gorm:"type:text;not null;index;default:'pending'"`
	RetriedAt     *time.Time
	RetriedTaskID *string
}
