package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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

var sharedAgentTools = []string{
	"web_fetch", "web_search", "web_crawl", "web_get_links", "web_screenshot", "web_transform",
	"agent", "sub_agent", "batch",
	"todowrite", "todoread",
	"journal_write", "journal_read",
	"skill",
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

func defaultToolPermissions(sandboxType string) model.JSON {
	tools := dedicatedAgentTools
	if sandboxType == "shared" {
		tools = sharedAgentTools
	}
	perms := model.JSON{}
	for _, tool := range tools {
		perms[tool] = "allow"
	}
	return perms
}

func (h *AgentHandler) ListSandboxTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidSandboxTools)
}

func (h *AgentHandler) ListBuiltInTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.ValidBuiltInTools)
}
