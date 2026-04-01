package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomDomain represents a user-configured preview domain for sandbox URLs.
// Users create two CNAMEs:
//  1. *.{Domain} → CNAMETarget (traffic routing)
//  2. _acme-challenge.{Domain} → {AcmeDNSSubdomain}.acme.llmvault.dev (cert challenges)
type CustomDomain struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrgID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"org_id"`
	Domain           string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"domain"`
	Verified         bool       `gorm:"default:false" json:"verified"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	CNAMETarget      string     `gorm:"type:varchar(255);not null" json:"cname_target"`
	AcmeDNSSubdomain string     `gorm:"type:varchar(255)" json:"acme_dns_subdomain"`
	AcmeDNSUsername  string     `gorm:"type:varchar(255)" json:"-"`
	AcmeDNSPassword  string     `gorm:"type:varchar(255)" json:"-"`
	AcmeDNSServerURL string     `gorm:"type:varchar(255)" json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (d *CustomDomain) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

// AcmeChallengeCNAME returns the CNAME record users need to create for cert challenges.
func (d *CustomDomain) AcmeChallengeCNAME() string {
	return d.AcmeDNSSubdomain + ".acme.llmvault.dev"
}
