package tasks

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/system"
)

func resolveIntegrations(deps system.ResolveDeps, raw map[string]any) ([]resolvedIntegration, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	connIDs := make([]uuid.UUID, 0, len(raw))
	connActionsBySlug := make(map[string][]string, len(raw))
	for connID, value := range raw {
		parsed, err := uuid.Parse(connID)
		if err != nil {
			return nil, &system.ResolveError{
				Code:    "unknown_connection",
				Message: fmt.Sprintf("invalid connection_id %q", connID),
			}
		}
		connIDs = append(connIDs, parsed)
		obj, _ := value.(map[string]any)
		actions := stringSliceArg(obj, "actions")
		connActionsBySlug[connID] = actions
	}

	var rows []connRow
	if err := deps.DB.
		Table("in_connections AS ic").
		Select("ic.id, ii.provider, ii.display_name").
		Joins("JOIN in_integrations ii ON ii.id = ic.in_integration_id").
		Where("ic.id IN ? AND ic.org_id = ? AND ic.revoked_at IS NULL", connIDs, deps.OrgID).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load connections: %w", err)
	}
	if len(rows) != len(connIDs) {
		foundIDs := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			foundIDs[i] = r.ID
		}
		return nil, &system.ResolveError{
			Code:    "unknown_connection",
			Message: missingMessage("connection", connIDs, foundIDs),
		}
	}

	out := make([]resolvedIntegration, 0, len(rows))
	for _, row := range rows {
		actions := connActionsBySlug[row.ID.String()]
		resolvedActions, err := lookupActions(deps.ActionsCatalog, row.Provider, actions)
		if err != nil {
			return nil, err
		}
		display := row.DisplayName
		if display == "" {
			display = row.Provider
		}
		out = append(out, resolvedIntegration{
			Provider:        display,
			ConnectionLabel: "",
			Actions:         resolvedActions,
		})
	}
	return out, nil
}

func lookupActions(cat *catalog.Catalog, provider string, slugs []string) ([]resolvedActionRef, error) {
	if cat == nil || len(slugs) == 0 {
		out := make([]resolvedActionRef, len(slugs))
		for i, s := range slugs {
			out[i] = resolvedActionRef{Slug: s}
		}
		return out, nil
	}
	out := make([]resolvedActionRef, 0, len(slugs))
	for _, slug := range slugs {
		def, ok := cat.GetAction(provider, slug)
		if !ok {
			return nil, &system.ResolveError{
				Code:    "unknown_action",
				Message: fmt.Sprintf("unknown action %q for provider %q", slug, provider),
			}
		}
		out = append(out, resolvedActionRef{
			Slug:        slug,
			DisplayName: def.DisplayName,
			Description: def.Description,
		})
	}
	return out, nil
}
