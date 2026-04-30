package tasks

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/system"
)

type resolvedSkill struct {
	Name        string
	Description string
	SourceType  string
}

type resolvedSubagent struct {
	Name        string
	Description string
	Model       string
}

type resolvedActionRef struct {
	Slug        string
	DisplayName string
	Description string
}

type resolvedIntegration struct {
	Provider        string
	ConnectionLabel string
	Actions         []resolvedActionRef
}

type resolvedTriggerKey struct {
	Display     string
	Description string
}

type resolvedTrigger struct {
	Type         string
	Provider     string
	Keys         []resolvedTriggerKey
	Cron         string
	Instructions string
}

type resolvedTool struct {
	Name        string
	Description string
}

type connRow struct {
	ID          uuid.UUID `gorm:"column:id"`
	Provider    string    `gorm:"column:provider"`
	DisplayName string    `gorm:"column:display_name"`
}

func resolvePromptWriterArgs(ctx context.Context, deps system.ResolveDeps, args map[string]any) (map[string]any, error) {
	// Every key the user template references must be present — render runs
	// with missingkey=error.
	out := map[string]any{
		"name":         stringArg(args, "name"),
		"category":     stringArg(args, "category"),
		"instructions": stringArg(args, "instructions"),
		"skills":       []resolvedSkill{},
		"subagents":    []resolvedSubagent{},
		"integrations": []resolvedIntegration{},
		"triggers":     []resolvedTrigger{},
		"tools":        []resolvedTool{},
		"permissions":  map[string]string{},
	}

	skills, err := resolveSkills(deps, stringSliceArg(args, "skill_ids"))
	if err != nil {
		return nil, err
	}
	if len(skills) > 0 {
		out["skills"] = skills
	}

	subs, err := resolveSubagents(deps, stringSliceArg(args, "subagent_ids"))
	if err != nil {
		return nil, err
	}
	if len(subs) > 0 {
		out["subagents"] = subs
	}

	integrations, err := resolveIntegrations(deps, mapArg(args, "integrations"))
	if err != nil {
		return nil, err
	}
	if len(integrations) > 0 {
		out["integrations"] = integrations
	}

	triggers, err := resolveTriggers(deps, objectListArg(args, "triggers"))
	if err != nil {
		return nil, err
	}
	if len(triggers) > 0 {
		out["triggers"] = triggers
	}

	if tools := resolveBuiltinTools(mapArg(args, "tools"), stringSliceArg(args, "sandbox_tools")); len(tools) > 0 {
		out["tools"] = tools
	}

	if perms := stringMapFromArg(mapArg(args, "permissions")); len(perms) > 0 {
		out["permissions"] = perms
	}

	return out, nil
}

func resolveSkills(deps system.ResolveDeps, rawIDs []string) ([]resolvedSkill, error) {
	ids, err := parseUUIDs(rawIDs, "skill_id", "unknown_skill")
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.Skill
	if err := deps.DB.
		Select("id, name, description, source_type").
		Where("id IN ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
			ids, deps.OrgID, model.SkillStatusPublished).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	if len(rows) != len(ids) {
		return nil, &system.ResolveError{
			Code:    "unknown_skill",
			Message: missingMessage("skill", ids, rowsToIDsSkill(rows)),
		}
	}
	out := make([]resolvedSkill, len(rows))
	for i, row := range rows {
		desc := ""
		if row.Description != nil {
			desc = *row.Description
		}
		out[i] = resolvedSkill{
			Name:        row.Name,
			Description: desc,
			SourceType:  row.SourceType,
		}
	}
	return out, nil
}

func rowsToIDsSkill(rows []model.Skill) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func resolveSubagents(deps system.ResolveDeps, rawIDs []string) ([]resolvedSubagent, error) {
	ids, err := parseUUIDs(rawIDs, "subagent_id", "unknown_subagent")
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.Agent
	if err := deps.DB.
		Select("id, name, description, model").
		Where("id IN ? AND agent_type = ? AND (org_id = ? OR (org_id IS NULL AND status = ?))",
			ids, model.AgentTypeSubagent, deps.OrgID, "active").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load subagents: %w", err)
	}
	if len(rows) != len(ids) {
		return nil, &system.ResolveError{
			Code:    "unknown_subagent",
			Message: missingMessage("subagent", ids, rowsToIDsSubagent(rows)),
		}
	}
	out := make([]resolvedSubagent, len(rows))
	for i, row := range rows {
		desc := ""
		if row.Description != nil {
			desc = *row.Description
		}
		out[i] = resolvedSubagent{
			Name:        row.Name,
			Description: desc,
			Model:       row.Model,
		}
	}
	return out, nil
}

func rowsToIDsSubagent(rows []model.Agent) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

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
			Cron:         stringArg(t, "cron_schedule"),
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
		case "cron":
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
		Table("in_connections AS ic").
		Select("ii.provider, ii.display_name").
		Joins("JOIN in_integrations ii ON ii.id = ic.in_integration_id").
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

func resolveBuiltinTools(toolsObj map[string]any, sandbox []string) []resolvedTool {
	defByID := make(map[string]model.BuiltInToolDefinition, len(model.ValidBuiltInTools))
	for _, t := range model.ValidBuiltInTools {
		defByID[t.ID] = t
	}
	sandboxByID := make(map[string]model.SandboxToolDefinition, len(model.ValidSandboxTools))
	for _, t := range model.ValidSandboxTools {
		sandboxByID[t.ID] = t
	}

	seen := make(map[string]struct{})
	out := make([]resolvedTool, 0, len(toolsObj)+len(sandbox))
	for id := range toolsObj {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if def, ok := defByID[id]; ok {
			out = append(out, resolvedTool{Name: def.Name, Description: def.Description})
		} else {
			out = append(out, resolvedTool{Name: id})
		}
	}
	for _, id := range sandbox {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if def, ok := sandboxByID[id]; ok {
			out = append(out, resolvedTool{Name: def.Name, Description: def.Description})
		} else {
			out = append(out, resolvedTool{Name: id})
		}
	}
	return out
}

func stringArg(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func stringSliceArg(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func mapArg(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, _ := m[key].(map[string]any)
	return v
}

func objectListArg(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

func stringMapFromArg(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func parseUUIDs(rawIDs []string, label, code string) ([]uuid.UUID, error) {
	if len(rawIDs) == 0 {
		return nil, nil
	}
	seen := make(map[uuid.UUID]struct{}, len(rawIDs))
	out := make([]uuid.UUID, 0, len(rawIDs))
	for _, raw := range rawIDs {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, &system.ResolveError{
				Code:    code,
				Message: fmt.Sprintf("invalid %s %q", label, raw),
			}
		}
		if _, dup := seen[parsed]; dup {
			continue
		}
		seen[parsed] = struct{}{}
		out = append(out, parsed)
	}
	return out, nil
}

func missingMessage(label string, requested, found []uuid.UUID) string {
	foundSet := make(map[uuid.UUID]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := foundSet[id]; !ok {
			return fmt.Sprintf("%s %s not found in this workspace", label, id)
		}
	}
	return fmt.Sprintf("one or more %s ids not found in this workspace", label)
}
