package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestClaimWarmRuntimeSlotWaitsAndDispatchesReconcile(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	if err := db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{}).Error; err != nil {
		t.Fatalf("clean warm slots: %v", err)
	}
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
		SandboxesRuntimeBaseImagePrefix: "runtime:test",
	})
	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})
	orch.warmPool = pool
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
	t.Cleanup(func() { db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}) })

	oldMaxWait := warmRuntimeClaimMaxWait
	oldInitialDelay := warmRuntimeClaimInitialDelay
	oldMaxDelay := warmRuntimeClaimMaxDelay
	warmRuntimeClaimMaxWait = 2 * time.Second
	warmRuntimeClaimInitialDelay = 10 * time.Millisecond
	warmRuntimeClaimMaxDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		warmRuntimeClaimMaxWait = oldMaxWait
		warmRuntimeClaimInitialDelay = oldInitialDelay
		warmRuntimeClaimMaxDelay = oldMaxDelay
	})

	var mu sync.Mutex
	var once sync.Once
	reconcileCalls := 0
	orch.SetWarmPoolReconciler(func(ctx context.Context, providerID, mode string) error {
		mu.Lock()
		reconcileCalls++
		mu.Unlock()
		once.Do(func() {
			go func() {
				time.Sleep(25 * time.Millisecond)
				created, err := pool.Reconcile(context.Background(), mode, nil)
				if err != nil {
					t.Errorf("reconcile: %v", err)
					return
				}
				for _, slotID := range created {
					if _, err := pool.CheckWarmSlot(context.Background(), slotID); err != nil {
						t.Errorf("check warm slot: %v", err)
					}
				}
			}()
		})
		return nil
	})

	claimed, err := orch.claimWarmRuntimeSlot(context.Background(), model.SandboxWarmSlotModeEmployee, sb.ID)
	if err != nil {
		t.Fatalf("claim warm runtime slot: %v", err)
	}
	if claimed.ExternalID == "" {
		t.Fatal("claimed slot missing external id")
	}
	mu.Lock()
	defer mu.Unlock()
	if reconcileCalls == 0 {
		t.Fatal("expected reconcile to be dispatched while waiting for warm slot")
	}
}
