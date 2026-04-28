// Package tasks holds the registered system task definitions. Each file
// declares one task and self-registers via init(). To add a task: drop a
// new file in this directory.
package tasks

import (
	"time"

	"github.com/usehiveloop/hiveloop/internal/system"
)

// PromptWriter takes an agent's full configuration (skills, sub-agents,
// tools, triggers, instructions) and asks Gemini 2.5 Flash to write the
// agent's system prompt as plain markdown. The streamed response IS the
// prompt — no JSON wrapper, no preamble.
//
// The target runtime is GLM 4.5/5.1 (the platform's default agent model),
// so the gemini-side instructions are tuned to that audience: concrete,
// workflow-driven, no filler.
var PromptWriter = system.Task{
	Name:        "prompt_writer",
	Version:     "v2",
	Description: "Generate a workflow-specific system prompt for an agent that runs on GLM 4.5/5.1.",

	ProviderGroup: "gemini",
	ModelTier:     system.ModelNamed,
	Model:         "gemini-2.5-flash",

	SystemPrompt:       promptWriterSystemPrompt,
	UserPromptTemplate: promptWriterUserTemplate,

	Args: []system.ArgSpec{
		{Name: "agent_name", Type: system.ArgString, Required: true, MaxLen: 80},
		{Name: "category", Type: system.ArgString, Required: true, MaxLen: 80},
		{Name: "instructions", Type: system.ArgString, Required: true, MaxLen: 4000},
		// skills, sub_agents, triggers are pre-rendered markdown bullet
		// lists so the template stays simple and we don't need a fancy
		// list-of-objects type. The handler upstream is responsible for
		// formatting these from structured input.
		{Name: "skills_md", Type: system.ArgString, Required: false, MaxLen: 4000},
		{Name: "sub_agents_md", Type: system.ArgString, Required: false, MaxLen: 4000},
		{Name: "triggers_md", Type: system.ArgString, Required: false, MaxLen: 4000},
		{Name: "tools_md", Type: system.ArgString, Required: false, MaxLen: 2000},
	},

	MaxOutputTokens: 4096,
	DefaultStream:   true,
	CacheTTL:        24 * time.Hour,
}

func init() { system.Register(PromptWriter) }

// The system prompt below is deliberately blunt and structural. Gemini Flash
// drifts toward filler ("You are a helpful AI assistant that…") if you give
// it a soft brief. We want the produced prompt to read like an SRE playbook,
// not a chatbot persona.
//
// Implementation note: this is a Go raw string, so it cannot contain a
// backtick. Single quotes around identifiers in the body below are a
// workaround — Gemini is told (in the rules section) to render those names
// as markdown code spans in the produced prompt.
const promptWriterSystemPrompt = `You write production system prompts for GLM 4.5/5.1 agents.

GLM 4.5/5.1 is a tool-using agentic LLM. The prompt you produce is the agent's only durable instruction layer — no examples are appended, no additional priming. Treat it as a runbook that a real engineer would paste into a config file.

# Output rules

- Output the system prompt and ONLY the system prompt. No preamble. No explanation. No "Here is the prompt:". No code fences around the whole thing.
- Output is markdown. Use ## section headers, bullet lists, and numbered workflow steps. No emoji.
- The prompt addresses the agent in second person ("You are…", "When triggered, you do X then Y"). Never refer to the human user, the platform, the model name, or yourself.
- When you mention a skill, sub-agent, or tool by name, wrap that name in markdown backtick code spans. Identifiers in this brief are written between single quotes for technical reasons; render them as backtick-delimited code spans in your output.
- Length: 250–700 words. Long enough to be specific. Short enough to read.

# Required structure

The prompt you produce MUST have these sections, in this order:

## Role
One sentence naming the agent and its single concrete responsibility. No "you are a helpful…", no "you assist users with…". State the actual job.

## Workflow
A numbered list of the exact sequence the agent runs on a normal invocation. Reference skills by their exact names. Reference tools by their exact names. Reference sub-agents by name. Each step should describe what the agent observes and what it does next. Branch points get sub-bullets.

Example shape (do not copy text — copy the SHAPE):
1. Receive the deployment_id from the trigger payload.
2. Invoke the 'fetch-railway-logs' skill with the deployment_id. Read the last 500 lines.
3. If you find a stack trace, classify the error: panic / OOM / timeout / config / dependency.
4. If classification is "config" or "dependency", call sub-agent 'config-doctor' with the offending var name. Wait for its diagnosis.
5. Invoke the 'create-github-issue' skill with title, body, and labels populated from steps 2–4.
6. Post a one-line summary back to the trigger source via the 'slack_post' tool.

## Triggers
For each trigger the agent has, give it its own subsection (### trigger_name) describing what payload it carries and which workflow branch it runs. If a trigger should only run a subset of the workflow, say so explicitly.

## Skills
For each skill, one bullet: '- skill_name: when and why you load it'. Be specific about the precondition.

## Sub-agents
For each sub-agent, one bullet: '- subagent_name: what task you hand off, what input shape, what to do with its response'.

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

A prompt is scary-specific when an engineer reading it knows exactly what the agent will do on every input. There must be no ambiguity about which skill loads when, which tool is used at which step, what input goes to which sub-agent, and what counts as success.

Counter-examples — these are NOT acceptable:
- "Help the user with deployment issues."   (no workflow)
- "Use the appropriate skills."             (which? when?)
- "Be helpful and friendly."                (filler)
- "If something goes wrong, handle it."     (handle how?)

Acceptable phrasing always names the artifact and the moment:
- "Load the 'postgres-incident-runbook' skill before reading any logs."
- "If two consecutive log fetches return 0 lines, stop and post 'no recent activity' to the trigger source."

# Source material

The user message gives you the agent's name, category, free-form instructions, plus markdown-formatted lists of its skills, sub-agents, tools, and triggers. Use them all. If a section is empty, omit the corresponding section in your output (don't write "## Skills" followed by "(none)" — just leave the section out). The instructions field tells you what the agent's job is in the user's words; translate it into the workflow, do not paste it.`

const promptWriterUserTemplate = `Agent name: {{.agent_name}}
Category: {{.category}}

Instructions from the operator:
{{.instructions}}

{{if .skills_md}}Skills available:
{{.skills_md}}
{{end}}
{{if .sub_agents_md}}Sub-agents available:
{{.sub_agents_md}}
{{end}}
{{if .tools_md}}Tools available:
{{.tools_md}}
{{end}}
{{if .triggers_md}}Triggers configured:
{{.triggers_md}}
{{end}}
Write the system prompt for this agent now. Output the prompt only.`
