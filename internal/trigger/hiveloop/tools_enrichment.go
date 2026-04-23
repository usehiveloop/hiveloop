package hiveloop

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
)

type planEnrichmentArgs struct {
	ConnectionID string         `json:"connection_id"`
	Action       string         `json:"action"`
	As           string         `json:"as"`
	Params       map[string]any `json:"params"`
}

var templateRefPattern = regexp.MustCompile(`\{\{(\w+)\.\w[\w.]*\}\}`)

// NewPlanEnrichmentHandler creates a tool handler that validates enrichment
// plans without executing them. Each call checks connection existence, action
// validity (read-only), param completeness, and {{step.field}} ordering.
// On success, registers the step and returns the action's response schema
// so the LLM can plan chained fetches.
func NewPlanEnrichmentHandler(
	connections []ConnectionWithActions,
	actionsCatalog *catalog.Catalog,
	planned *PlannedStepRegistry,
	enrichments *[]PlannedEnrichment,
) ToolHandler {
	connMap := make(map[string]ConnectionWithActions, len(connections))
	for _, conn := range connections {
		connMap[conn.Connection.ID.String()] = conn
	}

	return func(_ context.Context, _ string, raw json.RawMessage) (string, bool, error) {
		var args planEnrichmentArgs
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", false, fmt.Errorf("invalid arguments: %w", err)
		}

		conn, ok := connMap[args.ConnectionID]
		if !ok {
			var listing []string
			for _, candidate := range connections {
				listing = append(listing, fmt.Sprintf("  - %s (%s)", candidate.Connection.ID, candidate.Provider))
			}
			return "", false, fmt.Errorf("connection %q not found. Available connections:\n%s",
				args.ConnectionID, strings.Join(listing, "\n"))
		}

		actionDef, actionExists := conn.ReadActions[args.Action]
		if !actionExists {
			var listing []string
			for key, action := range conn.ReadActions {
				listing = append(listing, fmt.Sprintf("  - %s: %s", key, truncate(action.Description, 60)))
			}
			return "", false, fmt.Errorf("action %q not found for provider %q. Available read actions:\n%s",
				args.Action, conn.Provider, strings.Join(listing, "\n"))
		}

		if args.As == "" {
			return "", false, fmt.Errorf("'as' (step name) is required")
		}
		if planned.Has(args.As) {
			return "", false, fmt.Errorf("step name %q already used. Planned steps: %v. Pick a unique name.",
				args.As, planned.Names())
		}

		requiredParams := extractRequiredParams(actionDef)
		for _, paramName := range requiredParams {
			val, exists := args.Params[paramName]
			if !exists {
				return "", false, fmt.Errorf("missing required param %q for action %q.\nAction params: %s\nPlanned steps so far: %v\nAvailable refs: use $refs.x for webhook payload refs",
					paramName, args.Action, formatParamSchema(actionDef), planned.Names())
			}

			strVal, isString := val.(string)
			if isString {
				matches := templateRefPattern.FindAllStringSubmatch(strVal, -1)
				for _, match := range matches {
					stepName := match[1]
					if !planned.Has(stepName) {
						return "", false, fmt.Errorf("param %q references step %q which hasn't been planned yet. Planned steps: %v",
							paramName, stepName, planned.Names())
					}
				}
			}
		}

		planned.Add(args.As, args.Action)
		connID, _ := uuid.Parse(args.ConnectionID)
		*enrichments = append(*enrichments, PlannedEnrichment{
			ConnectionID: connID,
			As:           args.As,
			Action:       args.Action,
			Params:       args.Params,
		})

		responseSchemaInfo := ""
		if actionDef.ResponseSchema != "" {
			responseSchemaInfo = fmt.Sprintf("\nResponse schema ref: %s", actionDef.ResponseSchema)
		}

		return fmt.Sprintf("✓ Planned step %q: %s/%s.%s\nYou can reference results from this step as {{%s.field}} in subsequent plan_enrichment params.",
			args.As, conn.Provider, args.Action, responseSchemaInfo, args.As), false, nil
	}
}
