package credentials_test

import (
	"context"
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/credentials"
)

func TestIntegration_Picker_ReturnsSystemCredentialForMatchingGroup(t *testing.T) {
	db := connectTestDB(t)

	// Two system credentials for kimi group ("moonshotai" maps to "kimi")
	// plus one for a different group — picker must never return the latter.
	a := seedSystemCred(t, db, "moonshotai", false)
	b := seedSystemCred(t, db, "kimi", false)
	seedSystemCred(t, db, "openai", false) // decoy

	picker := credentials.NewPicker(db)
	got, err := picker.Pick(context.Background(), "kimi")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got.ID != a.ID && got.ID != b.ID {
		t.Fatalf("picked credential %s, expected one of %s/%s", got.ID, a.ID, b.ID)
	}
	if got.OrgID != nil {
		t.Errorf("picked system credential OrgID = %v, should be nil", got.OrgID)
	}
}

func TestIntegration_Picker_FiltersRevokedCredentials(t *testing.T) {
	db := connectTestDB(t)

	active := seedSystemCred(t, db, "moonshotai", false)
	seedSystemCred(t, db, "kimi", true) // revoked — must be filtered out

	picker := credentials.NewPicker(db)
	// Many iterations because selection is randomised — the revoked one must
	// never be returned.
	for range 10 {
		got, err := picker.Pick(context.Background(), "kimi")
		if err != nil {
			t.Fatalf("Pick: %v", err)
		}
		if got.ID != active.ID {
			t.Fatalf("picked revoked credential")
		}
	}
}

func TestIntegration_Picker_IgnoresNonSystemCredentials(t *testing.T) {
	db := connectTestDB(t)
	orgID := seedBYOKOrg(t, db)
	seedBYOKCred(t, db, orgID, "moonshotai") // user-owned, must be ignored

	picker := credentials.NewPicker(db)
	_, err := picker.Pick(context.Background(), "kimi")
	if !errors.Is(err, credentials.ErrNoSystemCredential) {
		t.Fatalf("expected ErrNoSystemCredential with only BYOK creds present, got %v", err)
	}
}

func TestIntegration_Picker_NoMatchReturnsSentinel(t *testing.T) {
	db := connectTestDB(t)

	picker := credentials.NewPicker(db)
	_, err := picker.Pick(context.Background(), "gemini")
	if !errors.Is(err, credentials.ErrNoSystemCredential) {
		t.Fatalf("expected ErrNoSystemCredential, got %v", err)
	}
}

func TestIntegration_Picker_PickByModelUsesCanonicalRoutes(t *testing.T) {
	db := connectTestDB(t)
	anthropic := seedSystemCred(t, db, "anthropic", false)
	seedSystemCred(t, db, "openrouter", false)

	picker := credentials.NewPicker(db)
	got, err := picker.PickByModel(context.Background(), "claude-sonnet-4.6")
	if err != nil {
		t.Fatalf("PickByModel: %v", err)
	}
	if got.ID != anthropic.ID {
		t.Fatalf("picked %s, want first matching anthropic credential %s", got.ID, anthropic.ID)
	}
}

func TestIntegration_Picker_PickByModelHonorsRoutePreference(t *testing.T) {
	db := connectTestDB(t)
	seedSystemCred(t, db, "openrouter", false)
	openai := seedSystemCred(t, db, "openai", false)

	picker := credentials.NewPicker(db)
	got, err := picker.PickByModel(context.Background(), "gpt-4o-mini")
	if err != nil {
		t.Fatalf("PickByModel: %v", err)
	}
	if got.ID != openai.ID {
		t.Fatalf("picked %s, want preferred openai credential %s", got.ID, openai.ID)
	}
}

func TestIntegration_Picker_PickByModelUsesOpenRouterWhenDirectProviderMissing(t *testing.T) {
	db := connectTestDB(t)
	openrouter := seedSystemCred(t, db, "openrouter", false)
	seedSystemCred(t, db, "openai", false)

	picker := credentials.NewPicker(db)
	got, err := picker.PickByModel(context.Background(), "claude-sonnet-4.6")
	if err != nil {
		t.Fatalf("PickByModel: %v", err)
	}
	if got.ID != openrouter.ID {
		t.Fatalf("picked %s, want openrouter credential %s", got.ID, openrouter.ID)
	}
}
