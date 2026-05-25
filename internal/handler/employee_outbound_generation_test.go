package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeOutboundModelUsage_WritesGenerationAndSkipsMemoryEvent(t *testing.T) {
	db := connectEmployeeSkillSyncTestDB(t)
	org := model.Org{Name: "runtime-usage-" + uuid.NewString(), RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	credential := model.Credential{
		Label:        "runtime-usage",
		BaseURL:      "https://openrouter.ai/api/v1",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("encrypted"),
		WrappedDEK:   []byte("wrapped"),
		ProviderID:   "openrouter",
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	credentialID := credential.ID
	agent := model.Employee{OrgID: &org.ID, CredentialID: &credentialID, Name: "Aria", Model: "deepseek-v4-flash", IsEmployee: true}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sandbox := model.Sandbox{OrgID: &org.ID, EmployeeID: &agent.ID, ExternalID: "runtime-usage-sandbox", BridgeURL: "http://localhost:7080", EncryptedBridgeAPIKey: []byte("key"), Status: "running"}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	token := model.Token{
		OrgID:        org.ID,
		CredentialID: credential.ID,
		JTI:          uuid.NewString(),
		ExpiresAt:    time.Now().Add(time.Hour),
		Meta: model.JSON{
			"employee_id": agent.ID.String(),
			"sandbox_id":  sandbox.ID.String(),
			"type":        "employee_proxy",
			"user":        "runtime-test-user",
		},
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Where("id = ?", token.ID).Delete(&model.Token{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Employee{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", credential.ID).Delete(&model.Credential{})
	})

	body, err := json.Marshal(runtimeUsagePayload())
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	h := NewEmployeeOutboundWebhookHandler(db, nil, nil)
	h.storeAndMaybeEnqueue(t.Context(), &sandbox, &employeeOutboundEvent{
		EventType: "agent.run.model.usage",
		Payload:   body,
		At:        time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	})

	var gen model.Generation
	if err := db.Where("org_id = ?", org.ID).First(&gen).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if gen.TokenJTI != token.JTI || gen.CredentialID != credential.ID || gen.ProviderID != "openrouter" {
		t.Fatalf("generation attribution mismatch: %#v", gen)
	}
	if gen.InputTokens != 11 || gen.OutputTokens != 7 || gen.CachedTokens != 5 || gen.ReasoningTokens != 3 {
		t.Fatalf("generation usage mismatch: %#v", gen)
	}
	if gen.Model != "deepseek-v4-flash" || !gen.IsStreaming || !gen.IsSystem || gen.UpstreamStatus != http.StatusOK {
		t.Fatalf("generation metadata mismatch: %#v", gen)
	}
	if gen.Cost != 0.000123 || gen.UserID != "runtime-test-user" {
		t.Fatalf("generation cost/user mismatch: %#v", gen)
	}
	if !strings.Contains(strings.Join(gen.Tags, ","), "session:http-session-1") {
		t.Fatalf("generation tags missing session: %#v", gen.Tags)
	}
	var eventCount int64
	db.Model(&model.EmployeeSessionEvent{}).Where("sandbox_id = ?", sandbox.ID).Count(&eventCount)
	if eventCount != 0 {
		t.Fatalf("model usage memory event count = %d, want 0", eventCount)
	}
}

func runtimeUsagePayload() map[string]any {
	return map[string]any{
		"session_id": "http-session-1",
		"source":     "http",
		"sequence":   float64(42),
		"agent_event": map[string]any{
			"kind":  "run_event",
			"event": "model_usage",
			"payload": map[string]any{
				"model":      "deepseek-v4-flash",
				"session_id": "http-session-1",
				"turn_id":    "turn-1",
				"usage": map[string]any{
					"prompt_tokens":      float64(11),
					"completion_tokens":  float64(7),
					"total_tokens":       float64(18),
					"cached_tokens":      float64(5),
					"reasoning_tokens":   float64(3),
					"cache_write_tokens": float64(0),
					"cost":               0.000123,
				},
			},
		},
	}
}
