package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Aliases so swagger emits BuiltInToolDefinition / SandboxToolDefinition keys
// in the schema rather than internal-package-prefixed ones.
type BuiltInToolDefinition = model.BuiltInToolDefinition
type SandboxToolDefinition = model.SandboxToolDefinition

func defaultJSON(j model.JSON) model.JSON {
	if j == nil {
		return model.JSON{}
	}
	return j
}

func ensureStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

var dedicatedAgentTools = []string{
	"Read", "write", "edit", "multiedit", "apply_patch", "Glob", "RipGrep", "AstGrep", "LS",
	"bash",
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	"agent", "sub_agent", "batch",
	"todowrite", "todoread",
	"journal_write", "journal_read",
	"lsp", "skill",
}

func defaultToolPermissions() model.JSON {
	perms := model.JSON{}
	for _, tool := range dedicatedAgentTools {
		perms[tool] = "allow"
	}
	return perms
}

// @Summary List sandbox tools
// @Description Returns the catalog of sandbox tools that can be granted to an agent.
// @Tags agents
// @Produce json
// @Success 200 {array} SandboxToolDefinition
// @Security BearerAuth
// @Router /v1/agents/sandbox-tools [get]
func (h *AgentHandler) ListSandboxTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidSandboxTools)
}

// @Summary List built-in tools
// @Description Returns the catalog of built-in tools that can be granted to an agent.
// @Tags agents
// @Produce json
// @Success 200 {array} BuiltInToolDefinition
// @Security BearerAuth
// @Router /v1/agents/built-in-tools [get]
func (h *AgentHandler) ListBuiltInTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidBuiltInTools)
}
