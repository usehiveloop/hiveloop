package credentials_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_Resolve_BYOKPath(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	cred := seedBYOKCred(t, db, orgID, "anthropic")

	agent := &model.Employee{
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
	if got.OrgID == nil {
		t.Errorf("BYOK cred has nil OrgID")
	}
}

func TestIntegration_Resolve_PlatformPath(t *testing.T) {
	db := connectTestDB(t)
	sys := seedSystemCred(t, db, "moonshotai", false)

	orgID := seedBYOKOrg(t, db)
	agent := &model.Employee{
		ID:           uuid.New(),
		OrgID:        &orgID,
		CredentialID: nil,
		Model:        "kimi-k2.5",
	}

	got, err := credentials.Resolve(context.Background(), db, credentials.NewPicker(db), agent)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.ID != sys.ID {
		t.Errorf("resolved %s, want system cred %s", got.ID, sys.ID)
	}
	if got.OrgID != nil {
		t.Errorf("picked system credential OrgID = %v, should be nil", got.OrgID)
	}
}

func TestIntegration_Resolve_MissingBYOKCredErrors(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)

	// Point at a credential that doesn't exist.
	ghostID := uuid.New()
	agent := &model.Employee{
		ID:           uuid.New(),
		OrgID:        &orgID,
		CredentialID: &ghostID,
	}

	_, err := credentials.Resolve(context.Background(), db, credentials.NewPicker(db), agent)
	if err == nil {
		t.Fatal("expected error when BYOK credential is missing from DB")
	}
}
