package handler

import "net/http"

// agentCategory is one entry in the platform's static catalog of work
// categories an agent can be tagged with.
type agentCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// agentCategories is the canonical list of work categories. Kept broad so
// the same taxonomy describes internal tools, customer-facing assistants,
// and personal productivity agents.
var agentCategories = []agentCategory{
	{ID: "engineering", Name: "Software engineering", Description: "Building, debugging, reviewing, refactoring, and shipping software."},
	{ID: "devops", Name: "DevOps & infrastructure", Description: "CI/CD pipelines, deployments, cloud resources, monitoring, and on-call."},
	{ID: "security", Name: "Security", Description: "Vulnerability scanning, secret hygiene, audits, and incident response."},
	{ID: "data", Name: "Data & analytics", Description: "SQL, ETL, dashboards, ad-hoc analysis, and BI reporting."},
	{ID: "research", Name: "Research", Description: "Market research, competitor analysis, literature reviews, and synthesis."},
	{ID: "product", Name: "Product", Description: "Specs, PRDs, prioritization, roadmap planning, and discovery."},
	{ID: "design", Name: "Design", Description: "UX writing, visual review, design-system upkeep, and asset generation."},
	{ID: "marketing", Name: "Marketing", Description: "Campaigns, copywriting, SEO, ads, and social."},
	{ID: "content", Name: "Content & writing", Description: "Blog posts, documentation, newsletters, scripts, and editing."},
	{ID: "sales", Name: "Sales", Description: "Lead enrichment, outbound, deal triage, and CRM hygiene."},
	{ID: "support", Name: "Customer support", Description: "Ticket triage, replies, escalations, and knowledge-base upkeep."},
	{ID: "success", Name: "Customer success", Description: "Onboarding, retention, account health, and renewal prep."},
	{ID: "operations", Name: "Operations", Description: "Process automation, internal tooling, vendor admin, and back-office."},
	{ID: "finance", Name: "Finance & accounting", Description: "Bookkeeping, expense triage, reporting, and forecasting."},
	{ID: "legal", Name: "Legal & compliance", Description: "Contract review, policy drafting, audits, and regulatory checks."},
	{ID: "hr", Name: "HR & people", Description: "Recruiting, onboarding, reviews, and employee experience."},
	{ID: "it", Name: "IT & help desk", Description: "User provisioning, device management, and internal triage."},
	{ID: "project-management", Name: "Project management", Description: "Status updates, planning, retros, and stakeholder comms."},
	{ID: "education", Name: "Education & training", Description: "Tutoring, course generation, study aids, and assessments."},
	{ID: "personal", Name: "Personal productivity", Description: "Email triage, scheduling, reminders, and everyday errands."},
	{ID: "other", Name: "Other", Description: "General-purpose agents that do not fit the categories above."},
}

// ListCategories handles GET /v1/agents/categories.
// @Summary List agent categories
// @Description Returns the static catalog of work categories an agent can be tagged with.
// @Tags agents
// @Produce json
// @Success 200 {array} agentCategory
// @Security BearerAuth
// @Router /v1/agents/categories [get]
func (h *AgentHandler) ListCategories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, agentCategories)
}
