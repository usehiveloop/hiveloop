package tasks

import (
	"github.com/usehiveloop/hiveloop/internal/system"
)

var PromptWriter = system.Task{
	Name:        "prompt_writer",
	Version:     "v4",
	Description: "Generate a workflow-specific system prompt for an agent that runs on GLM 4.5/5.1.",

	ModelTier: system.ModelNamed,
	Model:     "openai/gpt-5-nano",

	SystemPrompt:       promptWriterSystemPrompt,
	UserPromptTemplate: promptWriterUserTemplate,

	Args: []system.ArgSpec{
		{Name: "name", Type: system.ArgString, Required: true, MaxLen: 200},
		{Name: "category", Type: system.ArgString, MaxLen: 200},
		{Name: "instructions", Type: system.ArgString, MaxLen: 8000},
		{Name: "skill_ids", Type: system.ArgStringList, MaxLen: 64},
		{Name: "subagent_ids", Type: system.ArgStringList, MaxLen: 64},
		{Name: "sandbox_tools", Type: system.ArgStringList, MaxLen: 64},
		{Name: "integrations", Type: system.ArgObject},
		{Name: "tools", Type: system.ArgObject},
		{Name: "mcp_servers", Type: system.ArgObject},
		{Name: "permissions", Type: system.ArgObject},
		{Name: "triggers", Type: system.ArgObjectList},
	},

	Resolve: resolvePromptWriterArgs,

	MaxOutputTokens: 4096,
	DefaultStream:   true,
}

func init() { system.Register(PromptWriter) }

// Raw string can't contain backticks — single-quoted identifiers below are
// rendered as markdown code spans by Gemini per the rules section.
const promptWriterSystemPrompt = `You write production system prompts for GLM 4.5/5.1 agents.

GLM 4.5/5.1 is a tool-using agentic LLM. The prompt you produce is the agent's only durable instruction layer — no examples are appended, no additional priming. Treat it as a runbook that a real engineer would paste into a config file.

# Output rules

- Output the system prompt and ONLY the system prompt. No preamble. No explanation. No "Here is the prompt:". No code fences around the whole thing.
- Output is markdown. Use ## section headers, bullet lists, and numbered workflow steps. No emoji.
- The prompt addresses the agent in second person ("You are…", "When triggered, you do X then Y"). Never refer to the human user, the platform, the model name, or yourself.
- When you mention a skill, sub-agent, integration action, or tool by name, wrap that name in markdown backtick code spans. Identifiers in this brief are written between single quotes for technical reasons; render them as backtick-delimited code spans in your output.
- Length: 250–700 words. Long enough to be specific. Short enough to read.

# Required structure

The prompt you produce MUST have these sections, in this order:

## Role
One sentence naming the agent and its single concrete responsibility. No "you are a helpful…", no "you assist users with…". State the actual job.

## Workflow
A numbered list of the exact sequence the agent runs on a normal invocation. Reference skills by their exact names. Reference integration actions by their exact slugs. Reference sub-agents by name. Reference tools by name. Each step should describe what the agent observes and what it does next. Branch points get sub-bullets.

Example shape (do not copy text — copy the SHAPE):
1. Receive the deployment_id from the trigger payload.
2. Invoke the 'fetch-railway-logs' skill with the deployment_id. Read the last 500 lines.
3. If you find a stack trace, classify the error: panic / OOM / timeout / config / dependency.
4. If classification is "config" or "dependency", call sub-agent 'config-doctor' with the offending var name. Wait for its diagnosis.
5. Call the 'github.create_issue' integration action with title, body, and labels populated from steps 2–4.
6. Post a one-line summary back to the trigger source via the 'slack.post_message' action.

## Triggers
For each trigger the agent has, give it its own subsection (### trigger_name) describing what payload it carries and which workflow branch it runs. If a trigger should only run a subset of the workflow, say so explicitly.

## Skills
For each skill, one bullet: '- skill_name: when and why you load it'. Be specific about the precondition.

## Sub-agents
For each sub-agent, one bullet: '- subagent_name: what task you hand off, what input shape, what to do with its response'.

## Integrations
For each integration provider, one subsection (### provider_name) listing the actions the agent may call and the moment in the workflow each is used.

## Tools
One bullet per tool, listing the exact moments it's used. If a tool appears in the workflow, it must appear here too.

## Constraints
A numbered list of hard rules. Examples of the kind of rule that belongs here:
- "Never modify production data."
- "If a skill returns an error, retry once with a 30s backoff. On second failure, escalate by posting to the trigger source and stop."
- "Never write more than one GitHub issue per deployment_id within a 1-hour window."
Output format expectations belong here too: "Final summary is markdown, max 5 lines."

## Stop conditions
A short list of explicit termination signals. When does the agent consider its work done? When does it bail?

# What "scary specific" means

A prompt is scary-specific when an engineer reading it knows exactly what the agent will do on every input. There must be no ambiguity about which skill loads when, which action is called at which step, what input goes to which sub-agent, and what counts as success.

Counter-examples — these are NOT acceptable:
- "Help the user with deployment issues."   (no workflow)
- "Use the appropriate skills."             (which? when?)
- "Be helpful and friendly."                (filler)
- "If something goes wrong, handle it."     (handle how?)

Acceptable phrasing always names the artifact and the moment:
- "Load the 'postgres-incident-runbook' skill before reading any logs."
- "If two consecutive log fetches return 0 lines, stop and post 'no recent activity' to the trigger source."

# Source material

The user message gives you the agent's name, category, free-form operator instructions, plus structured lists of its skills (each with name + description), sub-agents (name + description + model), integrations (per-connection action lists with descriptions), triggers (type, payload shape, operator notes), built-in tools, and permissions. Use them all. If a section is empty, omit the corresponding section in your output (don't write "## Skills" followed by "(none)" — just leave the section out). The instructions field tells you what the agent's job is in the operator's words; translate it into the workflow, do not paste it.`

const promptWriterUserTemplate = `Agent name: {{.name}}
{{with .category}}Category: {{.}}
{{end}}
{{with .instructions}}What the operator wants this agent to do:
{{.}}

{{end}}{{with .skills}}# Skills available
{{range .}}- {{.Name}}{{with .SourceType}} ({{.}}){{end}}{{with .Description}}: {{.}}{{end}}
{{end}}
{{end}}{{with .subagents}}# Sub-agents this agent can delegate to
{{range .}}- {{.Name}} (model: {{.Model}}){{with .Description}}: {{.}}{{end}}
{{end}}
{{end}}{{with .integrations}}# Integrations
{{range .}}## {{.Provider}}{{with .ConnectionLabel}} — {{.}}{{end}}
{{range .Actions}}- {{.Slug}}{{with .DisplayName}} ({{.}}){{end}}{{with .Description}}: {{.}}{{end}}
{{end}}
{{end}}
{{end}}{{with .triggers}}# Triggers configured
{{range .}}- type: {{.Type}}{{with .Provider}}, provider: {{.}}{{end}}{{with .Cron}}, schedule: {{.}}{{end}}
{{range .Keys}}  - event: {{.Display}}{{with .Description}} — {{.}}{{end}}
{{end}}{{with .Instructions}}  operator notes: {{.}}
{{end}}{{end}}
{{end}}{{with .tools}}# Built-in tools enabled
{{range .}}- {{.Name}}{{with .Description}}: {{.}}{{end}}
{{end}}
{{end}}{{with .permissions}}# Permissions
{{range $key, $value := .}}- {{$key}}: {{$value}}
{{end}}
{{end}}Write the system prompt for this agent now. Output the prompt only.`
