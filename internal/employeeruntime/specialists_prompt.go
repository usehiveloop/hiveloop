package employeeruntime

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/specialists"
)

func buildAvailableSpecialistsSection(agent *model.Employee, catalog *specialists.Catalog) PromptSection {
	if agent == nil || catalog == nil {
		return PromptSection{}
	}
	attached := map[string]bool{}
	for _, slug := range agent.AttachedSpecialists {
		slug = strings.TrimSpace(slug)
		if slug != "" {
			attached[slug] = true
		}
	}
	if len(attached) == 0 {
		return PromptSection{}
	}
	lines := []string{
		"Use specialists for bounded work that should run outside the current employee sandbox. Launch them through the Hivy MCP specialist tools, not through local delegation tools.",
		"Attached specialists:",
	}
	for _, def := range catalog.List() {
		if !attached[def.Slug] {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", def.Slug, strings.TrimSpace(def.Description)))
	}
	if len(lines) == 2 {
		return PromptSection{}
	}
	return PromptSection{
		Title:   "Available specialists",
		Content: strings.Join(lines, "\n"),
	}
}
