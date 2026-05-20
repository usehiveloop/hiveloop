package sandbox

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestPusher_NeedsTokenRotation(t *testing.T) {
	db := setupPusherTestDB(t)

	org := model.Org{ID: uuid.New(), Name: "token-rotation-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	cred := model.Credential{
		ID: uuid.New(), OrgID: org.ID, ProviderID: "openai", Label: "OpenAI",
		BaseURL: "https://api.openai.com", AuthScheme: "bearer",
		EncryptedKey: []byte("enc"), WrappedDEK: []byte("dek"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.Token{})
		db.Unscoped().Where("id = ?", cred.ID).Delete(&model.Credential{})
		db.Unscoped().Where("id = ?", org.ID).Delete(&model.Org{})
	})

	pusher := NewPusher(db, nil, nil, nil, nil)
	now := time.Now()

	for _, tc := range []struct {
		name      string
		expiresIn *time.Duration
		revoked   bool
		want      bool
	}{
		{name: "no token", want: true},
		{name: "expires inside rotation window", expiresIn: durationPtr(2 * time.Hour), want: true},
		{name: "fresh token", expiresIn: durationPtr(20 * time.Hour), want: false},
		{name: "revoked token ignored", expiresIn: durationPtr(20 * time.Hour), revoked: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentID := uuid.New()
			if tc.expiresIn != nil {
				var revokedAt *time.Time
				if tc.revoked {
					revoked := now
					revokedAt = &revoked
				}
				tok := model.Token{
					ID:           uuid.New(),
					OrgID:        org.ID,
					CredentialID: cred.ID,
					JTI:          uuid.NewString(),
					ExpiresAt:    now.Add(*tc.expiresIn),
					Meta:         model.JSON{"agent_id": agentID.String(), "type": "agent_proxy"},
					RevokedAt:    revokedAt,
				}
				if err := db.Create(&tok).Error; err != nil {
					t.Fatalf("create token: %v", err)
				}
			}

			if got := pusher.NeedsTokenRotation(agentID.String()); got != tc.want {
				t.Fatalf("NeedsTokenRotation() = %v, want %v", got, tc.want)
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
