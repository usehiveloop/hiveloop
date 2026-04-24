package credentials_test

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_SeedPlatformOrg_CreatesRow(t *testing.T) {
	db := connectTestDB(t)

	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("SeedPlatformOrg: %v", err)
	}

	var org model.Org
	if err := db.Where("id = ?", credentials.PlatformOrgID).First(&org).Error; err != nil {
		t.Fatalf("platform org not found after seed: %v", err)
	}
	if org.Name != "__platform__" {
		t.Errorf("platform org name = %q, want __platform__", org.Name)
	}
}

func TestIntegration_SeedPlatformOrg_Idempotent(t *testing.T) {
	db := connectTestDB(t)

	// Three times — any error on later calls would indicate a missing
	// FirstOrCreate or a unique-index collision.
	for range 3 {
		if err := credentials.SeedPlatformOrg(db); err != nil {
			t.Fatalf("SeedPlatformOrg repeat call: %v", err)
		}
	}

	var count int64
	if err := db.Model(&model.Org{}).
		Where("id = ?", credentials.PlatformOrgID).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 platform org row after repeated seeds, got %d", count)
	}
}
