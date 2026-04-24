package credentials_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_Resolve_BYOKPath(t *testing.T) {
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	orgID := seedBYOKOrg(t, db)
	cred := seedBYOKCred(t, db, orgID, "anthropic")

	agent := &model.Agent{
		ID:           uuid.New(),
		OrgID:        &orgID,
		CredentialID: &cred.ID,
	}

	got, err := credentials.Resolve(context.Background(), db, credentials.NewPicker(db), agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != cred.ID {
		t.Errorf("resolved %s, want BYOK cred %s", got.ID, cred.ID)
	}
	if got.IsSystem {
		t.Errorf("BYOK cred reported IsSystem = true")
	}
}

func TestIntegration_Resolve_PlatformPath(t *testing.T) {
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sys := seedSystemCred(t, db, "moonshotai", false)

	orgID := seedBYOKOrg(t, db)
	agent := &model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		CredentialID:  nil,
		ProviderGroup: "kimi",
	}

	got, err := credentials.Resolve(context.Background(), db, credentials.NewPicker(db), agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != sys.ID {
		t.Errorf("resolved %s, want system cred %s", got.ID, sys.ID)
	}
	if !got.IsSystem {
		t.Errorf("picked credential IsSystem = false, should be true")
	}
}

func TestIntegration_Resolve_MissingBYOKCredErrors(t *testing.T) {
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	orgID := seedBYOKOrg(t, db)

	// Point at a credential that doesn't exist.
	ghostID := uuid.New()
	agent := &model.Agent{
		ID:           uuid.New(),
		OrgID:        &orgID,
		CredentialID: &ghostID,
	}

	_, err := credentials.Resolve(context.Background(), db, credentials.NewPicker(db), agent)
	if err == nil {
		t.Fatal("expected error when BYOK credential is missing from DB")
	}
}
