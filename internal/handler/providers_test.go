package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestProviderHandler_AllModelsReturnsCanonicalModels(t *testing.T) {
	db := connectTestDB(t)
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}

	creds := []model.Credential{
		{
			OrgID:        credentials.PlatformOrgID,
			Label:        "test-anthropic",
			BaseURL:      "https://api.anthropic.com",
			AuthScheme:   "bearer",
			ProviderID:   "anthropic",
			EncryptedKey: []byte("enc"),
			WrappedDEK:   []byte("dek"),
			IsSystem:     true,
		},
		{
			OrgID:        credentials.PlatformOrgID,
			Label:        "test-openrouter",
			BaseURL:      "https://openrouter.ai/api/v1",
			AuthScheme:   "bearer",
			ProviderID:   "openrouter",
			EncryptedKey: []byte("enc"),
			WrappedDEK:   []byte("dek"),
			IsSystem:     true,
		},
	}
	if err := db.Create(&creds).Error; err != nil {
		t.Fatalf("create credentials: %v", err)
	}
	t.Cleanup(func() {
		for _, cred := range creds {
			db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	handler.NewProviderHandler(registry.Global(), db).AllModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var models []struct {
		ID          string   `json:"id"`
		ProviderIDs []string `json:"provider_ids"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	foundCanonical := false
	for _, model := range models {
		if model.ID == "anthropic/claude-sonnet-4.6" {
			t.Fatalf("response exposed upstream route alias: %#v", model)
		}
		if model.ID == "claude-sonnet-4.6" {
			foundCanonical = true
			if len(model.ProviderIDs) == 0 {
				t.Fatalf("canonical model missing provider_ids: %#v", model)
			}
		}
	}
	if !foundCanonical {
		t.Fatal("response did not include canonical claude-sonnet-4.6")
	}
}
