package tasks

import "github.com/usehivy/hivy/internal/model"

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
