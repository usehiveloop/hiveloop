package credentials

import (
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// PlatformOrgID is the fixed UUID of the internal org that owns every system
// credential. Using a well-known id (rather than looking it up by name each
// call) keeps queries cheap and makes cross-database seeding deterministic.
var PlatformOrgID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Platform org canonical name. Double underscores keep it out of any
// org-name collision with real customer orgs.
const platformOrgName = "__platform__"

// SeedPlatformOrg ensures the platform org row exists. Idempotent: safe to
// call on every boot. System credentials FK to this row.
func SeedPlatformOrg(db *gorm.DB) error {
	org := model.Org{
		ID:     PlatformOrgID,
		Name:   platformOrgName,
		Active: true,
	}
	// FirstOrCreate: look up by ID; insert with the supplied fields when missing.
	if err := db.FirstOrCreate(&org, model.Org{ID: PlatformOrgID}).Error; err != nil {
		return fmt.Errorf("seed platform org: %w", err)
	}
	return nil
}
