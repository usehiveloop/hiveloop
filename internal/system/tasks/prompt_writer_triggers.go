package tasks

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/system"
)

func resolveTriggers(deps system.ResolveDeps, raw []map[string]any) ([]resolvedTrigger, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]resolvedTrigger, 0, len(raw))
	for i, t := range raw {
		triggerType := stringArg(t, "trigger_type")
		if triggerType == "" {
			triggerType = "webhook"
		}
		trig := resolvedTrigger{
			Type:         triggerType,
			Instructions: stringArg(t, "instructions"),
		}
		switch triggerType {
		case "webhook":
			connID := stringArg(t, "connection_id")
			slug, display, err := lookupConnectionProvider(deps, connID)
			if err != nil {
				return nil, fmt.Errorf("triggers[%d]: %w", i, err)
			}
			trig.Provider = display
			keys := stringSliceArg(t, "trigger_keys")
			trig.Keys = lookupTriggerKeys(deps.ActionsCatalog, slug, keys)
		case "http":
			keys := stringSliceArg(t, "trigger_keys")
			trig.Keys = make([]resolvedTriggerKey, len(keys))
			for j, k := range keys {
				trig.Keys[j] = resolvedTriggerKey{Display: k}
			}
		default:
			return nil, &system.ResolveError{
				Code:    "invalid_trigger_type",
				Message: fmt.Sprintf("triggers[%d]: invalid trigger_type %q", i, triggerType),
			}
		}
		out = append(out, trig)
	}
	return out, nil
}

func lookupConnectionProvider(deps system.ResolveDeps, connID string) (string, string, error) {
	if connID == "" {
		return "", "", &system.ResolveError{Code: "unknown_connection", Message: "connection_id is required for webhook triggers"}
	}
	parsed, err := uuid.Parse(connID)
	if err != nil {
		return "", "", &system.ResolveError{Code: "unknown_connection", Message: fmt.Sprintf("invalid connection_id %q", connID)}
	}
	type row struct {
		Provider    string `gorm:"column:provider"`
		DisplayName string `gorm:"column:display_name"`
	}
	var r row
	if err := deps.DB.
		Table("connections AS ic").
		Select("ii.provider, ii.display_name").
		Joins("JOIN integrations ii ON ii.id = ic.integration_id").
		Where("ic.id = ? AND ic.org_id = ? AND ic.revoked_at IS NULL", parsed, deps.OrgID).
		Scan(&r).Error; err != nil {
		return "", "", fmt.Errorf("load connection: %w", err)
	}
	if r.Provider == "" {
		return "", "", &system.ResolveError{Code: "unknown_connection", Message: fmt.Sprintf("connection %s not found in this workspace", connID)}
	}
	display := r.DisplayName
	if display == "" {
		display = r.Provider
	}
	return r.Provider, display, nil
}

func lookupTriggerKeys(cat *catalog.Catalog, providerDisplay string, keys []string) []resolvedTriggerKey {
	out := make([]resolvedTriggerKey, len(keys))
	for i, k := range keys {
		out[i] = resolvedTriggerKey{Display: k}
		if cat == nil {
			continue
		}
		if def, ok := cat.GetTrigger(providerDisplay, k); ok {
			out[i].Display = def.DisplayName
			out[i].Description = def.Description
		}
	}
	return out
}
