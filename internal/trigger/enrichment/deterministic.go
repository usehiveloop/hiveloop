package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/mcpserver"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

const maxSectionChars = 8000

// DeterministicEnricher pre-fetches context from provider APIs using the
// enrichment actions defined in the trigger catalog. No LLM involved — it
// runs each action in parallel, collects results, and composes a structured
// markdown message for the agent.
type DeterministicEnricher struct {
	nangoClient *nango.Client
	catalog     *catalog.Catalog
	db          *gorm.DB
}

// NewDeterministicEnricher creates a deterministic enricher.
func NewDeterministicEnricher(nangoClient *nango.Client, cat *catalog.Catalog, db *gorm.DB) *DeterministicEnricher {
	return &DeterministicEnricher{nangoClient: nangoClient, catalog: cat, db: db}
}

// DeterministicEnrichInput carries everything the enricher needs.
type DeterministicEnrichInput struct {
	Provider     string
	EventType    string
	EventAction  string
	OrgID        uuid.UUID
	ConnectionID uuid.UUID
	Refs         map[string]string
}

type enrichmentResult struct {
	As     string
	Action string
	Data   map[string]any
	Err    error
}

// Enrich runs all enrichment actions for the trigger and returns a composed
// markdown message. Returns empty string if the trigger has no enrichment
// actions or if the connection cannot be resolved.
func (enricher *DeterministicEnricher) Enrich(ctx context.Context, input DeterministicEnrichInput, logger *slog.Logger) (string, error) {
	// Build event key.
	eventKey := input.EventType
	if input.EventAction != "" {
		eventKey = input.EventType + "." + input.EventAction
	}

	// Look up trigger definition from catalog.
	triggerDef, ok := enricher.catalog.GetTrigger(input.Provider, eventKey)
	if !ok {
		// Try variant fallback (e.g. github-app → github).
		providerTriggers, variantOK := enricher.catalog.GetProviderTriggersForVariant(input.Provider)
		if variantOK {
			if def, defOK := providerTriggers.Triggers[eventKey]; defOK {
				triggerDef = &def
				ok = true
			}
		}
	}
	if !ok || len(triggerDef.Enrichment) == 0 {
		logger.Info("deterministic enrichment: no enrichment actions defined",
			"provider", input.Provider,
			"event_key", eventKey,
		)
		return "", nil
	}

	logger.Info("deterministic enrichment: starting",
		"provider", input.Provider,
		"event_key", eventKey,
		"action_count", len(triggerDef.Enrichment),
	)

	// Load InConnection + InIntegration for Nango credentials.
	var inConn model.InConnection
	if err := enricher.db.Preload("InIntegration").
		Where("id = ? AND revoked_at IS NULL", input.ConnectionID).
		First(&inConn).Error; err != nil {
		logger.Warn("deterministic enrichment: connection not found",
			"connection_id", input.ConnectionID,
			"error", err,
		)
		return "", nil
	}

	providerCfgKey := "in_" + inConn.InIntegration.UniqueKey
	nangoConnID := inConn.NangoConnectionID
	providerName := inConn.InIntegration.Provider

	// Load provider schemas for GraphQL selection set building.
	var providerSchemas map[string]catalog.SchemaDefinition
	if providerDef, providerOK := enricher.catalog.GetProvider(providerName); providerOK {
		providerSchemas = providerDef.Schemas
	}

	// Run all enrichment actions in parallel.
	results := make([]enrichmentResult, len(triggerDef.Enrichment))
	var waitGroup sync.WaitGroup

	for index, enrichAction := range triggerDef.Enrichment {
		waitGroup.Add(1)
		go func(idx int, action catalog.EnrichmentAction) {
			defer waitGroup.Done()

			result := enrichmentResult{As: action.As, Action: action.Action}

			// Substitute $refs.xxx in params.
			params := substituteRefsInParams(action.Params, input.Refs)

			// Look up action definition.
			actionDef, actionOK := enricher.catalog.GetAction(providerName, action.Action)
			if !actionOK {
				result.Err = fmt.Errorf("action %q not found in catalog for provider %q", action.Action, providerName)
				results[idx] = result
				return
			}

			// Execute the action directly via Nango proxy.
			data, err := mcpserver.ExecuteAction(
				ctx,
				enricher.nangoClient,
				providerName,
				providerCfgKey,
				nangoConnID,
				actionDef,
				params,
				nil, // no resource scoping for enrichment
				providerSchemas,
			)
			result.Data = data
			result.Err = err
			results[idx] = result
		}(index, enrichAction)
	}

	waitGroup.Wait()

	// Log summary.
	successCount := 0
	var failedActions []string
	for _, result := range results {
		if result.Err != nil {
			failedActions = append(failedActions, result.Action+": "+result.Err.Error())
		} else {
			successCount++
		}
	}

	if len(failedActions) > 0 {
		logger.Warn("enrichment actions failed",
			"failed", failedActions,
		)
	}

	logger.Info("enrichment complete",
		"total", len(results),
		"succeeded", successCount,
		"failed", len(results)-successCount,
	)

	// Compose the markdown message.
	return composeEnrichedMessage(input, results), nil
}

// composeEnrichedMessage builds a structured markdown message from the
// enrichment results and webhook refs.
func composeEnrichedMessage(input DeterministicEnrichInput, results []enrichmentResult) string {
	var builder strings.Builder

	// Header with event summary.
	eventKey := input.EventType
	if input.EventAction != "" {
		eventKey = input.EventType + "." + input.EventAction
	}
	builder.WriteString(fmt.Sprintf("## %s\n\n", eventKey))

	// Refs table.
	builder.WriteString("| Field | Value |\n|---|---|\n")
	for key, value := range input.Refs {
		builder.WriteString(fmt.Sprintf("| %s | %s |\n", key, value))
	}
	builder.WriteString("\n---\n\n")

	// One section per enrichment result.
	for _, result := range results {
		builder.WriteString(fmt.Sprintf("### %s\n\n", result.As))

		if result.Err != nil {
			builder.WriteString(fmt.Sprintf("> **Error:** %s\n\n", result.Err.Error()))
			continue
		}

		jsonBytes, err := json.MarshalIndent(result.Data, "", "  ")
		if err != nil {
			builder.WriteString(fmt.Sprintf("> **Error:** failed to marshal result: %s\n\n", err.Error()))
			continue
		}

		section := string(jsonBytes)
		if len(section) > maxSectionChars {
			section = section[:maxSectionChars] + "\n... (truncated)"
		}

		builder.WriteString("```json\n")
		builder.WriteString(section)
		builder.WriteString("\n```\n\n")
	}

	return builder.String()
}

// substituteRefsInParams deep-copies params and replaces all "$refs.xxx"
// string values with the corresponding ref value. Works recursively on
// nested maps and slices.
func substituteRefsInParams(params map[string]any, refs map[string]string) map[string]any {
	if params == nil {
		return nil
	}
	result := make(map[string]any, len(params))
	for key, value := range params {
		result[key] = substituteRefsInValue(value, refs)
	}
	return result
}

func substituteRefsInValue(value any, refs map[string]string) any {
	switch typedValue := value.(type) {
	case string:
		if strings.HasPrefix(typedValue, "$refs.") {
			refKey := strings.TrimPrefix(typedValue, "$refs.")
			if refValue, ok := refs[refKey]; ok {
				return refValue
			}
		}
		return typedValue
	case map[string]any:
		return substituteRefsInParams(typedValue, refs)
	case []any:
		result := make([]any, len(typedValue))
		for index, item := range typedValue {
			result[index] = substituteRefsInValue(item, refs)
		}
		return result
	default:
		return value
	}
}
