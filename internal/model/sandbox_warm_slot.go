package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	SandboxWarmSlotModeEmployee   = "employee"
	SandboxWarmSlotModeSpecialist = "specialist"

	SandboxWarmSlotStatusWarming  = "warming"
	SandboxWarmSlotStatusWarm     = "warm"
	SandboxWarmSlotStatusClaiming = "claiming"
	SandboxWarmSlotStatusClaimed  = "claimed"
	SandboxWarmSlotStatusDeleting = "deleting"
	SandboxWarmSlotStatusError    = "error"
)

type SandboxWarmSlot struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ProviderID             string    `gorm:"not null;index:idx_sandbox_warm_slots_pool_status,priority:1"`
	Mode                   string    `gorm:"not null;index:idx_sandbox_warm_slots_pool_status,priority:2"`
	Status                 string    `gorm:"not null;default:'warming';index:idx_sandbox_warm_slots_pool_status,priority:3"`
	ExternalID             string    `gorm:"not null;uniqueIndex:idx_sandbox_warm_slots_provider_external,priority:2"`
	EndpointURL            string    `gorm:"not null"`
	RuntimeImage           string    `gorm:"not null"`
	RuntimePort            int       `gorm:"not null;default:7080"`
	Region                 string    `gorm:"not null;default:''"`
	ClaimedSandboxID       *uuid.UUID
	ClaimedSandbox         *Sandbox `gorm:"foreignKey:ClaimedSandboxID;constraint:OnDelete:SET NULL"`
	EncryptedRuntimeSecret []byte   `gorm:"type:bytea;not null"`
	ErrorMessage           *string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (SandboxWarmSlot) TableName() string { return "sandbox_warm_slots" }
