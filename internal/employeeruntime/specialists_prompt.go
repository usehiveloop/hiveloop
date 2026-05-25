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
		"Specialist agents are attached coworkers you can dispatch for focused work that benefits from separate execution.",
		"Use specialist_launch_task when a specialist is the better fit than doing the work directly. Choose based on the names and descriptions below.",
		"Dispatch heavy tasks that may consume computer resources for more than 30 seconds, or tasks that need disk or network reads and writes, when the request gives enough context to act.",
		"When a task matches a dedicated specialist and the objective is clear, use that specialist instead of doing it yourself. The specialist is an expert with tools for that work.",
		"If a request is vague, ask for the missing context before dispatching. Make assumptions only for trivial, low-risk details.",
		"Give the specialist enough context to act independently, then use status tools to follow progress and report verified outcomes.",
		"For specialist work that will take more than a quick check, schedule a wake instead of repeatedly polling status.",
		"Attached specialist agents:",
	}
	for _, def := range catalog.List() {
		if !attached[def.Slug] {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", def.Name, def.Slug, strings.TrimSpace(def.Description)))
	}
	if len(lines) == 8 {
		return PromptSection{}
	}
	return PromptSection{
		Title:   "Specialist agents",
		Content: strings.Join(lines, "\n"),
	}
}
