package hiveloop

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
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

// PickBalancedCredential selects a model that balances recency and cost.
// Prefers modern models at moderate cost — not the cheapest (which may be
// underpowered) and not the most expensive (unnecessary for enrichment).
//
// Scoring: 0.6 * recency_score + 0.4 * cost_efficiency_score
//   - recency_score: days since release, normalized so newer = higher score
//   - cost_efficiency_score: penalizes both extremes (very cheap and very
//     expensive), peaks at the median cost across all candidates
//
// This naturally selects Sonnet-class models over Haiku or Opus when all
// three are available.
func PickBalancedCredential(db *gorm.DB, reg *registry.Registry, orgID uuid.UUID) (*CredentialWithModel, error) {
	var credentials []model.Credential
	if err := db.Where("org_id = ? AND revoked_at IS NULL", orgID).Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}
	if len(credentials) == 0 {
		return nil, fmt.Errorf("no active credentials found for org %s", orgID)
	}

	type candidate struct {
		credential  model.Credential
		modelID     string
		provider    string
		costOutput  float64
		releaseDate time.Time
	}

	now := time.Now()
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
			releaseDate := now.AddDate(-2, 0, 0) // default: treat unknown as old
			if modelDef.ReleaseDate != "" {
				if parsed, parseErr := time.Parse("2006-01-02", modelDef.ReleaseDate); parseErr == nil {
					releaseDate = parsed
				}
			}
			candidates = append(candidates, candidate{
				credential:  cred,
				modelID:     modelID,
				provider:    cred.ProviderID,
				costOutput:  outputCost,
				releaseDate: releaseDate,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no credentials with tool-calling models found for org %s", orgID)
	}

	// Find cost range for normalization.
	minCost, maxCost := candidates[0].costOutput, candidates[0].costOutput
	var oldestRelease, newestRelease time.Time
	oldestRelease = candidates[0].releaseDate
	newestRelease = candidates[0].releaseDate
	costSum := float64(0)
	for _, candidate := range candidates {
		if candidate.costOutput < minCost {
			minCost = candidate.costOutput
		}
		if candidate.costOutput > maxCost {
			maxCost = candidate.costOutput
		}
		if candidate.releaseDate.Before(oldestRelease) {
			oldestRelease = candidate.releaseDate
		}
		if candidate.releaseDate.After(newestRelease) {
			newestRelease = candidate.releaseDate
		}
		costSum += candidate.costOutput
	}
	medianCost := costSum / float64(len(candidates))

	costRange := maxCost - minCost
	dateRange := newestRelease.Sub(oldestRelease).Hours()

	sort.Slice(candidates, func(indexA, indexB int) bool {
		return scoreCandidate(candidates[indexA].costOutput, candidates[indexA].releaseDate, medianCost, costRange, dateRange, oldestRelease) >
			scoreCandidate(candidates[indexB].costOutput, candidates[indexB].releaseDate, medianCost, costRange, dateRange, oldestRelease)
	})

	best := candidates[0]
	return &CredentialWithModel{
		Credential: best.credential,
		ModelID:    best.modelID,
		Provider:   best.provider,
	}, nil
}

// scoreCandidate computes a 0-1 score balancing recency and cost efficiency.
func scoreCandidate(cost float64, releaseDate time.Time, medianCost, costRange, dateRangeHours float64, oldestRelease time.Time) float64 {
	// Recency score: 0 (oldest) to 1 (newest).
	recencyScore := float64(0)
	if dateRangeHours > 0 {
		recencyScore = releaseDate.Sub(oldestRelease).Hours() / dateRangeHours
	}

	// Cost efficiency: peaks at median cost, penalizes extremes.
	// Uses a Gaussian-like curve centered on the median.
	costScore := float64(1)
	if costRange > 0 {
		distanceFromMedian := (cost - medianCost) / costRange
		costScore = math.Exp(-2 * distanceFromMedian * distanceFromMedian)
	}

	return 0.6*recencyScore + 0.4*costScore
}
