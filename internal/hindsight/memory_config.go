package hindsight

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/usehivy/hivy/internal/model"
)

// MemoryConfig is the customer-facing memory configuration for an identity.
// Stored as JSONB in Identity.MemoryConfig and exposed via the Identity API.
type MemoryConfig struct {
	Enabled               *bool  `json:"enabled,omitempty"`
	RetainMission         string `json:"retain_mission,omitempty"`
	ReflectMission        string `json:"reflect_mission,omitempty"`
	ObservationsMission   string `json:"observations_mission,omitempty"`
	DispositionSkepticism *int   `json:"disposition_skepticism,omitempty"`
	DispositionLiteralism *int   `json:"disposition_literalism,omitempty"`
	DispositionEmpathy    *int   `json:"disposition_empathy,omitempty"`
}

const (
	defaultRetainMission = `You are retaining memory for an AI employee embedded inside a real company.

Extract only durable company memory that will help the employee make better decisions in future conversations.

Keep:
- company identity, positioning, business model, customers, market, goals, and constraints
- team responsibilities, ownership areas, rituals, working norms, and collaboration preferences
- people, roles, areas of ownership, and who should be consulted for what
- active projects, initiatives, milestones, blockers, risks, and historical context only when they are expected to matter after this conversation
- explicit decisions, the reasons behind them, tradeoffs discussed, and the final outcome
- policies, standards, operating procedures, and recurring workflows
- technical context: architecture, repositories, stack choices, deploy practices, testing norms, incidents, and durable operational constraints
- customer context: segments, important accounts, recurring feedback, objections, feature requests, and durable sentiment
- explicit feedback about how the AI employee should communicate or behave

Ignore:
- greetings, jokes, filler, reactions without substance, and ordinary small talk
- transient task status unless it establishes a durable project fact or decision
- routine task execution details such as "checked file", "ran command", "updated page", "looked at logs", or "used tool"
- one-off debugging steps, status updates, and implementation actions unless the result establishes a reusable rule, decision, owner, policy, or technical constraint
- vague feedback unless it is resolved into a durable preference, expectation, or operating rule
- raw logs, raw transcripts, raw code dumps, and large source dumps
- duplicate facts already established unless the new evidence changes or contradicts them
- secrets, credentials, private tokens, or sensitive personal data

Write memories as concise facts the employee should know later.
Do not write memories about the employee merely doing work.
Preserve source, speaker, time, and channel/project context only when it helps explain authority, ownership, recency, or why the memory matters.`

	defaultObservationsMission = `Identify durable patterns and evolving company knowledge from retained memories.

Create or update observations only when they synthesize durable patterns across retained memories.

Good observations:
- repeated company/team preferences
- recurring decisions, policies, or operating norms
- changes in strategy, priorities, ownership, or technical direction
- recurring customer feedback, objections, and product requests
- recurring incidents, blockers, engineering risks, or process failures
- repeated feedback about how the AI employee should behave

Bad observations:
- summaries of a single task being done
- tool-use summaries
- "the AI agent checked/updated/ran/fixed" statements
- one-off requests that are already complete
- raw status updates or implementation steps

Prefer stable patterns over one-off statements.
When new evidence contradicts older memory, preserve the transition: what changed, when, who said it, and why.
Do not create observations from filler, transient task chatter, or isolated low-confidence comments.`

	defaultReflectMission = `You are the long-term memory of an AI employee working inside this company.

When reflecting, synthesize company, team, project, technical, customer, people, policy, preference, and decision memories into a concise, useful answer.

Prioritize:
- current company/team context
- durable decisions and their reasoning
- known ownership and people context
- team policies, preferences, and communication norms
- technical and project constraints
- customer or business context when relevant

Do not invent facts outside memory.
If memory is incomplete, stale, or conflicting, say so clearly.
When useful, explain the evidence or source pattern behind the answer.
Keep the answer practical and employee-like.`

	defaultSkepticism = 3
	defaultLiteralism = 4
	defaultEmpathy    = 2
)

var SupportedMemoryTypes = []string{
	"company_context",
	"people",
	"project",
	"decision",
	"policy",
	"preference",
	"technical_context",
	"customer_context",
	"bot_feedback",
}

// DefaultMemoryConfig returns sensible defaults for general-purpose employees.
func DefaultMemoryConfig() MemoryConfig {
	enabled := true
	skep, lit, emp := defaultSkepticism, defaultLiteralism, defaultEmpathy
	return MemoryConfig{
		Enabled:               &enabled,
		RetainMission:         defaultRetainMission,
		ReflectMission:        defaultReflectMission,
		ObservationsMission:   defaultObservationsMission,
		DispositionSkepticism: &skep,
		DispositionLiteralism: &lit,
		DispositionEmpathy:    &emp,
	}
}

// ParseMemoryConfig parses the JSONB field from Identity.MemoryConfig into a MemoryConfig.
// Returns the default config if the JSONB is nil or empty.
func ParseMemoryConfig(j model.JSON) MemoryConfig {
	if len(j) == 0 {
		return DefaultMemoryConfig()
	}
	b, err := json.Marshal(j)
	if err != nil {
		return DefaultMemoryConfig()
	}
	var mc MemoryConfig
	if err := json.Unmarshal(b, &mc); err != nil {
		return DefaultMemoryConfig()
	}
	return mc
}

// IsEnabled returns true unless Enabled is explicitly set to false.
func (m MemoryConfig) IsEnabled() bool {
	return m.Enabled == nil || *m.Enabled
}

// Hash returns a SHA256 hex string of the config for change detection.
func (m MemoryConfig) Hash() string {
	b, _ := json.Marshal(m)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func (m MemoryConfig) ToBankConfigUpdate() *BankConfigUpdate {
	retain := m.RetainMission
	if retain == "" {
		retain = defaultRetainMission
	}
	reflect := m.ReflectMission
	if reflect == "" {
		reflect = defaultReflectMission
	}
	observations := m.ObservationsMission
	if observations == "" {
		observations = defaultObservationsMission
	}
	skep := defaultSkepticism
	if m.DispositionSkepticism != nil {
		skep = *m.DispositionSkepticism
	}
	lit := defaultLiteralism
	if m.DispositionLiteralism != nil {
		lit = *m.DispositionLiteralism
	}
	emp := defaultEmpathy
	if m.DispositionEmpathy != nil {
		emp = *m.DispositionEmpathy
	}

	return &BankConfigUpdate{Updates: map[string]any{
		"retain_mission":         retain,
		"reflect_mission":        reflect,
		"observations_mission":   observations,
		"entity_labels":          memoryEntityLabels(),
		"disposition_skepticism": skep,
		"disposition_literalism": lit,
		"disposition_empathy":    emp,
	}}
}

func memoryEntityLabels() []map[string]any {
	return []map[string]any{
		{
			"key":         "memory_type",
			"description": "Durable business-memory category.",
			"type":        "value",
			"tag":         true,
			"optional":    false,
			"values":      memoryTypeEntityValues(),
		},
		{
			"key":         "visibility",
			"description": "Whether the memory is company-wide.",
			"type":        "value",
			"tag":         true,
			"optional":    false,
			"values":      entityValues([]string{"company"}),
		},
	}
}

func memoryTypeEntityValues() []map[string]string {
	descriptions := map[string]string{
		"company_context":   "Company identity, market, business model, goals, constraints, or operating context.",
		"people":            "People, roles, ownership areas, expertise, and who should be consulted for what.",
		"project":           "Projects, initiatives, milestones, blockers, risks, and historical context.",
		"decision":          "Explicit decisions, reasons, tradeoffs, and changes in direction.",
		"policy":            "Rules, standards, procedures, recurring workflows, and company conventions.",
		"preference":        "Durable preferences about communication, execution, process, or collaboration.",
		"technical_context": "Architecture, repositories, stack choices, deploy practices, testing norms, incidents, and operational facts.",
		"customer_context":  "Customer segments, accounts, feedback, objections, feature requests, and sentiment.",
		"bot_feedback":      "Explicit feedback about how the AI employee should communicate, decide, or behave.",
	}
	values := make([]map[string]string, 0, len(SupportedMemoryTypes))
	for _, value := range SupportedMemoryTypes {
		values = append(values, map[string]string{"value": value, "description": descriptions[value]})
	}
	return values
}

func entityValues(values []string) []map[string]string {
	out := make([]map[string]string, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]string{"value": value})
	}
	return out
}

func IsSupportedMemoryType(value string) bool {
	for _, supported := range SupportedMemoryTypes {
		if value == supported {
			return true
		}
	}
	return false
}
