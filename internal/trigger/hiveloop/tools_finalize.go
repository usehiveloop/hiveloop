package hiveloop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
)

// NewFinalizeHandler creates a tool handler that signals the routing session
// is complete. The agent loop stops and collects all routed agents and
// planned enrichments.
func NewFinalizeHandler() ToolHandler {
	return func(_ context.Context, _ string, _ json.RawMessage) (string, bool, error) {
		return "✓ Routing complete.", true, nil
	}
}

func extractRequiredParams(action catalog.ActionDef) []string {
	if action.Parameters == nil {
		return nil
	}
	var schema struct {
		Required []string `json:"required"`
	}
	json.Unmarshal(action.Parameters, &schema)
	return schema.Required
}

func formatParamSchema(action catalog.ActionDef) string {
	if action.Parameters == nil {
		return "(no parameters)"
	}
	var schema struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	json.Unmarshal(action.Parameters, &schema)

	requiredSet := make(map[string]bool)
	for _, name := range schema.Required {
		requiredSet[name] = true
	}

	var parts []string
	for name, prop := range schema.Properties {
		req := ""
		if requiredSet[name] {
			req = " [required]"
		}
		parts = append(parts, fmt.Sprintf("  %s (%s%s): %s", name, prop.Type, req, truncate(prop.Description, 60)))
	}
	return strings.Join(parts, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
