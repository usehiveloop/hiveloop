package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestWarmPoolReconcileCreatesWarmSlotAndClaimMarksClaiming(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(health.Close)
	provider.warmEndpoint = health.URL

	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxWarmPoolEmployeeSize:     1,
		RailwayRuntimePort:              7080,
		SandboxesRuntimeBaseImage: "runtime:test",
	})
	if pool == nil {
		t.Fatal("warm pool is nil")
	}

	created, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeEmployee, nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created slots = %d, want 1", len(created))
	}
	if len(provider.warmCreateCalls) != 1 {
		t.Fatalf("warm create calls = %d, want 1", len(provider.warmCreateCalls))
	}
	result, err := pool.CheckWarmSlot(context.Background(), created[0])
	if err != nil {
		t.Fatalf("check warm slot: %v", err)
	}
	if result == nil || !result.Ready {
		t.Fatalf("check result = %#v, want ready", result)
	}

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "pending",
		RuntimeURL:             "pending",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	claimed, err := pool.Claim(context.Background(), model.SandboxWarmSlotModeEmployee, sb.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.EndpointURL != health.URL {
		t.Fatalf("endpoint = %q, want %q", claimed.EndpointURL, health.URL)
	}

	var slot model.SandboxWarmSlot
	if err := db.First(&slot, "id = ?", claimed.ID).Error; err != nil {
		t.Fatalf("load slot: %v", err)
	}
	if slot.Status != model.SandboxWarmSlotStatusClaiming {
		t.Fatalf("slot status = %q, want claiming", slot.Status)
	}
	if slot.ClaimedSandboxID == nil || *slot.ClaimedSandboxID != sb.ID {
		t.Fatalf("claimed sandbox id = %v, want %s", slot.ClaimedSandboxID, sb.ID)
	}
}
