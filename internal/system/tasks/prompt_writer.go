// Package tasks holds the registered system task definitions. Each file
// declares one task and self-registers via init(). To add a task: drop a
// new file in this directory.
package tasks

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/system"
)

// PromptWriter takes a target model + a high-level goal and returns a
// production-quality system prompt. The prompt the model returns is
// expected to follow a fixed JSON shape (see SystemPrompt) so callers can
// drop it straight into another agent's system field.
var PromptWriter = system.Task{
	Name:        "prompt_writer",
	Version:     "v1",
	Description: "Generate a powerful system prompt for a target model and goal.",

	ProviderGroup:  "openai",
	ModelTier:      system.ModelCheapest,
	ResponseFormat: system.ResponseJSON,

	SystemPrompt: `You write production-grade system prompts.

The user gives you:
- target_model:   the model the new prompt will run on (e.g. "gpt-4.1-mini")
- goal:           one sentence describing the agent's job
- audience:       who the agent's user is
- constraints:    optional list of rules / safety bounds

You return strict JSON with this exact shape:
{
  "title": "<3-7 word label for the prompt>",
  "system_prompt": "<the new system prompt, plain text, addressed to the model>",
  "rationale": "<one short paragraph explaining the structural choices>"
}

Rules for the prompt you produce:
- Open by naming the role and the goal in one sentence.
- State outputs concretely (shape, length, format).
- Enumerate constraints as a numbered list when there are 2+.
- Avoid filler ("you are a helpful AI…"); be specific to the goal.
- No mention of model providers or yourself in the produced prompt.
- Plain text only — no markdown headings, no code fences.

Return JSON only. No prose outside the JSON.`,

	UserPromptTemplate: `target_model: {{.target_model}}
goal: {{.goal}}
audience: {{.audience}}
{{if .constraints}}constraints:
{{range .constraints}}- {{.}}
{{end}}{{end}}`,

	Args: []system.ArgSpec{
		{Name: "target_model", Type: system.ArgString, Required: true, MaxLen: 80},
		{Name: "goal", Type: system.ArgString, Required: true, MaxLen: 1000},
		{Name: "audience", Type: system.ArgString, Required: true, MaxLen: 200},
		{Name: "constraints", Type: system.ArgStringList, Required: false, MaxLen: 200},
	},

	MaxOutputTokens: 1024,
	DefaultStream:   false,
	CacheTTL:        24 * time.Hour,
}

func init() { system.Register(PromptWriter) }
