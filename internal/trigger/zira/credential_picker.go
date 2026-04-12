package zira

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/registry"
)

// CredentialWithModel pairs a credential with the selected model ID.
type CredentialWithModel struct {
	Credential model.Credential
	ModelID    string
	Provider   string
}

// PickBestCredential scans the org's active credentials and selects the
// cheapest model that supports tool calling (required for Zira's routing
// agent loop). Returns the credential, model ID, and provider name.
//
// Preference order: cheapest output cost among models with ToolCall=true.
// Falls back across providers — if OpenAI is cheapest, picks OpenAI; if
// Anthropic is cheapest, picks Anthropic.
func PickBestCredential(db *gorm.DB, reg *registry.Registry, orgID uuid.UUID) (*CredentialWithModel, error) {
	var credentials []model.Credential
	if err := db.Where("org_id = ? AND revoked_at IS NULL", orgID).Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf("no active credentials found for org %s", orgID)
	}

	type candidate struct {
		credential model.Credential
		modelID    string
		provider   string
		costOutput float64
	}

	var candidates []candidate
	for _, cred := range credentials {
		provider, ok := reg.GetProvider(cred.ProviderID)
		if !ok {
			continue
		}
		for modelID, modelDef := range provider.Models {
			if !modelDef.ToolCall {
				continue
			}
			outputCost := float64(0)
			if modelDef.Cost != nil {
				outputCost = modelDef.Cost.Output
			}
			candidates = append(candidates, candidate{
				credential: cred,
				modelID:    modelID,
				provider:   cred.ProviderID,
				costOutput: outputCost,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no credentials with tool-calling models found for org %s", orgID)
	}

	sort.Slice(candidates, func(indexA, indexB int) bool {
		return candidates[indexA].costOutput < candidates[indexB].costOutput
	})

	best := candidates[0]
	return &CredentialWithModel{
		Credential: best.credential,
		ModelID:    best.modelID,
		Provider:   best.provider,
	}, nil
}
