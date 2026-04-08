# The Prompt Engineering Framework

> Drop this file into any AI agent's context when you need to generate a system prompt for Claude, GPT, or Gemini. Tell the agent your use case and which provider you're targeting, and it will use this framework to produce a production-grade prompt.

---

## How to use this document

1. **Pick your provider** (Anthropic, OpenAI, or Google)
2. **Describe your use case** to the agent
3. The agent follows the provider-specific flow below to generate a system prompt
4. The agent applies the universal principles + provider-specific rules + anti-patterns to avoid
5. You get a ready-to-deploy prompt

---

# UNIVERSAL PRINCIPLES (Apply to ALL providers)

These principles are consistent across Anthropic, OpenAI, and Google's official documentation. Apply them regardless of which model you're targeting.

## 1. Clarity over cleverness
- State the goal in one sentence before adding detail
- Use plain language. If a human would find the instruction ambiguous, the model will too
- Every instruction should map to a specific observable behavior in the output
- If you can't explain what a line in your prompt does, delete it

## 2. Positive instructions over negative constraints
- Tell the model what TO DO, not what NOT to do
- "Write in flowing prose paragraphs" beats "Don't use bullet points"
- If you must use a negative, pair it with a positive alternative: "Don't guess. Instead, say 'I don't have enough information to answer this.'"

## 3. Examples > rules
- 3 well-chosen examples teach behavior more reliably than 15 rules
- Examples define format, tone, length, and reasoning depth simultaneously
- Include edge cases in your examples, not as separate rules
- Ensure perfect formatting consistency across all examples (whitespace, delimiters, casing)

## 4. Structure with delimiters
- Use XML tags (`<role>`, `<instructions>`, `<context>`) or Markdown headers (`# Role`, `## Instructions`) — pick one and be consistent
- Never mix XML and Markdown structure in the same prompt
- Delimiters separate "what the model should read" from "what the model should do"

## 5. Start minimal, iterate on failures
- Begin with the smallest prompt that could work
- Run it against test cases
- Add instructions ONLY when you observe a specific failure
- Every line in your production prompt should trace to a real failure in testing
- Remove instructions that don't measurably improve output

## 6. Context placement matters
- Separate "reference data" from "instructions"
- Label data clearly so the model knows it's input, not instructions to follow
- For large context: test both orders (data-first vs. instructions-first) — results vary by model

## 7. Output contracts
- Define the exact output format: JSON schema, prose structure, section headers
- Specify length constraints when they matter
- Provide a response template or skeleton when format precision is critical
- If the output will be parsed programmatically, say so — the model will be more careful

## 8. Reasoning scaffolding
- For complex tasks: ask the model to reason before answering
- For simple tasks: skip reasoning to save latency and tokens
- Match reasoning depth to task complexity — don't over-think classification, don't under-think strategy

## 9. Evaluation mindset
- Define what "good" looks like before writing the prompt
- Create 5-10 test cases covering normal, edge, and adversarial inputs
- Measure before and after every prompt change
- Track failure modes systematically — they tell you exactly what to add

## 10. Prompt hygiene
- Review for contradictions — two instructions that conflict will degrade all models
- Remove redundant instructions — saying the same thing twice wastes attention budget
- Check for implicit assumptions — if you assume the model knows something, make it explicit
- Version control your prompts — they are code

---

# PROVIDER-SPECIFIC FRAMEWORKS

---

## ANTHROPIC (Claude 4.x) — The XML Contract Framework

### Architecture

```
SYSTEM PROMPT
├── 1. Role & Identity
├── 2. Context & Motivation
├── 3. Instructions (positive, action-oriented)
├── 4. Examples (diverse, canonical)
├── 5. Output Format (XML-tagged)
├── 6. Thinking & Reasoning Rules
├── 7. Guardrails & Edge Cases
└── 8. Tool Behavior (if agentic)

USER PROMPT
├── Input data (XML-wrapped)
└── Optional: prefill assistant response to lock format
```

### System Prompt Template

```xml
<role>
You are a [specific role] with expertise in [domain].
You are [personality traits relevant to the task: precise, thorough, concise, creative].
[One sentence on what success looks like for this role.]
</role>

<context>
[Why this task matters. What the user is trying to achieve. What will happen
with the output. This helps Claude calibrate tone, depth, and format.]

[Any background information, domain knowledge, or constraints the model
needs to understand before receiving instructions.]
</context>

<instructions>
[Numbered, action-oriented steps. Each step = one observable behavior.]

1. [First action — verb-first: "Analyze...", "Extract...", "Generate..."]
2. [Second action]
3. [Third action]

[For conditional logic:]
- If [condition A]: [specific action]
- If [condition B]: [specific action]
- If [neither]: [fallback action with explicit behavior]
</instructions>

<examples>
[2-4 input/output pairs. Diverse scenarios. Consistent formatting.
Include at least one edge case.]

<example>
<input>[Representative input 1]</input>
<output>[Exact desired output 1]</output>
</example>

<example>
<input>[Edge case input]</input>
<output>[Desired edge case handling]</output>
</example>
</examples>

<output_format>
[Exact structure of the response. Use XML tags to enforce sections.]

Structure your response as:
<analysis>[Your analysis here]</analysis>
<recommendation>[Your recommendation here]</recommendation>
<confidence>[high/medium/low]</confidence>

[Or for JSON:]
Return a JSON object with this schema:
{
  "field_1": "string — description",
  "field_2": "number — description",
  "field_3": ["string — description"]
}
</output_format>

<thinking_instructions>
[Only include if the task requires multi-step reasoning.]

Before providing your final answer, work through the problem step by step
inside <thinking> tags. Consider:
- [Specific thing to evaluate #1]
- [Specific thing to evaluate #2]
Then provide your final answer outside the thinking tags.

[If extended thinking is DISABLED, replace "think" with "evaluate/consider/assess":]
Before providing your final answer, evaluate the problem step by step.
</thinking_instructions>

<constraints>
[What to do in ambiguous situations. Escalation rules. Safety boundaries.]

- If you lack sufficient information to complete the task, say so explicitly
  and list what additional information you need.
- [Domain-specific constraint]
- [Quality bar: "Ensure all code compiles", "Verify all citations exist", etc.]
</constraints>

<!-- AGENTIC ADDITIONS (only if using tools) -->

<tool_behavior>
[How to use tools. When to use them. When NOT to use them.]

- Use [tool_name] when [specific condition].
- Do NOT use [tool_name] for [specific anti-pattern].
- When multiple tools could apply, prefer [tool_name] because [reason].

[Parallel tool calling:]
If you need to call multiple tools with no dependencies between them,
call all independent tools in parallel. If a call depends on a previous
result, call it sequentially. Never guess missing parameters.
</tool_behavior>

<action_mode>
[Choose ONE of the following blocks based on your use case:]

<!-- For agents that should ACT by default: -->
By default, implement changes rather than only suggesting them.
If the user's intent is unclear, infer the most useful likely action
and proceed, using tools to discover any missing details instead of guessing.

<!-- For agents that should ADVISE by default: -->
Do not jump into implementation unless clearly instructed.
Default to providing information, analysis, and recommendations.
Only proceed with edits when the user explicitly requests them.
</action_mode>

<memory_and_context>
[Only if the agent runs over long sessions or multiple context windows.]

Your context window will be automatically compacted as it approaches
its limit. Do not stop tasks early due to token budget concerns.
As you approach your limit, save progress and state to memory
before the context window refreshes.

After completing each logical unit of work, commit/save state.
When starting a new session, read progress notes and git log first.
</memory_and_context>
```

### Claude-Specific Rules

| Rule | Details |
|------|---------|
| **Formatting control** | Match your prompt style to desired output. If you write in prose, Claude responds in prose. If you use bullets, Claude uses bullets. |
| **Prefill trick** | Start the assistant's response to lock format: `{"role": "assistant", "content": '{"category": "'}` |
| **Tool triggering** | Dial BACK aggressive language. "Use this tool when..." not "CRITICAL: You MUST use this tool." Claude 4.x is already responsive. |
| **"Think" sensitivity** | When extended thinking is disabled, avoid the word "think." Use "evaluate," "consider," "assess" instead. |
| **Verbosity** | Claude 4.x is naturally concise. If you want detailed output, say: "Provide a comprehensive, detailed response." If you want summaries after tool use, say: "After tool use, summarize what you did." |
| **Subagent orchestration** | Claude 4.5 naturally delegates to subagents. Don't over-instruct — just make tools available and well-described. |
| **Frontend design** | Add an `<frontend_aesthetics>` block to prevent "AI slop" — specify fonts, color philosophy, animation approach, and explicitly ban Inter/Roboto/purple gradients. |
| **Code exploration** | Add: "ALWAYS read and understand relevant files before proposing edits. Do not speculate about code you have not opened." |
| **Overengineering prevention** | Add: "The right amount of complexity is the minimum needed for the current task. Don't add features, abstractions, or error handling beyond what was asked." |
| **State tracking** | Use git as the primary state mechanism. "After each unit of work, commit. When resuming, read git log and progress.txt." |

### Claude Anti-Patterns to Avoid
- ❌ "CRITICAL: You MUST ALWAYS..." — overtriggers in Claude 4.x
- ❌ "Don't use markdown" — say "Write in flowing prose paragraphs" instead
- ❌ Hardcoding if-else logic in prompts — use heuristics and principles instead
- ❌ Listing 15+ rules when 3 examples would teach the same behavior
- ❌ Using the word "think" with extended thinking disabled
- ❌ Assuming Claude will be verbose — explicitly request detail when needed

---

## OPENAI (GPT-5 / GPT-5.4) — The Steerable Agent Framework

### Architecture

```
SYSTEM PROMPT
├── 1. Agent Identity & Responsibilities
├── 2. Tool Preamble Rules
├── 3. Eagerness Calibration (search depth, persistence, stop conditions)
├── 4. Tool Safety Tiers
├── 5. Domain Rules & Code Standards
├── 6. Output Contract (verbosity, markdown, format)
└── 7. Self-Reflection (optional, for quality-critical tasks)

API PARAMETERS (set alongside the prompt)
├── model: gpt-5 / gpt-5.4
├── reasoning_effort: none / low / medium / high / xhigh
├── verbosity: low / medium / high
├── previous_response_id: [for multi-turn reasoning persistence]
└── tools: [function definitions]

USER PROMPT
├── Input data + task
└── Re-inject formatting reminders every 3-5 turns in long conversations
```

### System Prompt Template

```xml
<agent_identity>
You are [role] — a [type of agent: software engineer, research analyst, support agent].
Your responsibilities:
- [Primary responsibility]
- [Secondary responsibility]
You operate as an autonomous agent that [completes tasks / advises / implements].
</agent_identity>

<tool_preambles>
[Controls how the model narrates its work to the user.]

Always begin by rephrasing the user's goal in a clear, concise manner
before calling any tools. Then outline a structured plan detailing each
logical step. As you execute, narrate each step succinctly. Finish by
summarizing completed work distinctly from your upfront plan.
</tool_preambles>

<context_gathering>
[Controls how aggressively the model searches for information.]

[FOR LESS EAGERNESS — fast, focused:]
Goal: Get enough context fast. Parallelize discovery and stop as soon
as you can act.
- Start broad, then fan out to focused subqueries.
- Avoid over-searching. If needed, run one targeted parallel batch.
- Early stop: you can name exact content to change, or top hits
  converge (~70%) on one area.
- Maximum of [N] tool calls for context gathering.
- If you need more, update the user with findings and open questions.

[FOR MORE EAGERNESS — thorough, autonomous:]
You are an agent — keep going until the user's query is completely
resolved before ending your turn. Only terminate when you are sure
the problem is solved. Never stop when uncertain — deduce the most
reasonable approach and continue. Do not ask to confirm assumptions —
decide, proceed, and document for reference after.
</context_gathering>

<tool_safety>
[Risk-tiered tool calling rules.]

Low risk (proceed freely, no confirmation needed):
- search, grep, read_file, list_directory, web_search

Medium risk (proceed but log the action):
- update_record, send_notification, create_file

High risk (ALWAYS confirm with user before executing):
- delete_file, process_payment, cancel_subscription, send_email
- Any action that is irreversible or has financial impact

For low-risk tools, never ask — just act.
For high-risk tools, explain what you're about to do and why, then ask.
</tool_safety>

<domain_rules>
[Task-specific rules, code standards, business logic.]

[FOR CODING USE CASES:]
<code_editing_rules>
<guiding_principles>
- Clarity and reuse: modular, reusable components. No duplication.
- Consistency: unified design system — color tokens, typography, spacing.
- Simplicity: small focused components. No unnecessary complexity.
- Match existing style: read the codebase before writing. Blend in.
</guiding_principles>

<stack_defaults>
- Framework: [e.g., Next.js TypeScript]
- Styling: [e.g., TailwindCSS]
- UI Components: [e.g., shadcn/ui]
- Icons: [e.g., Lucide]
- State: [e.g., Zustand]
</stack_defaults>

<directory_structure>
[Paste your actual project structure here]
</directory_structure>
</code_editing_rules>

[FOR NON-CODING USE CASES:]
[Paste relevant business rules, policies, SOPs, or domain knowledge here.]
</domain_rules>

<output_contract>
[Controls format, length, and structure of the final answer.]

[Verbosity is also controlled via the API `verbosity` parameter.
Use this section for context-specific overrides:]

- Default: concise status updates and progress narration.
- For code output: use high verbosity. Write readable code with clear
  variable names, comments where needed, and straightforward control flow.
  Do not produce code-golf or overly clever one-liners.
- For user-facing messages: keep brief and actionable.

[Markdown control:]
Use Markdown only where semantically correct:
- `inline code` for file names, functions, and class names
- ```code fences``` for code blocks
- ### headers for major sections (only when explicitly useful)
- Lists only when presenting truly discrete items
Do not use bold/italic formatting unless specifically requested.

[Structured output:]
[If the output will be parsed, define the schema here:]
Return a JSON object:
{
  "field": "type — description",
  ...
}
</output_contract>

<self_reflection>
[Optional. For quality-critical tasks like zero-to-one app generation.]

Before producing your final output:
1. Think deeply about what makes a world-class result for this task.
2. Create an internal rubric with 5-7 quality categories. Do not show
   this rubric to the user.
3. Use the rubric to evaluate your draft. If it doesn't hit top marks
   across all categories, revise before presenting.
</self_reflection>

<persistence_and_planning>
[For agentic tasks that span multiple turns.]

Decompose the user's query into all required sub-requests. Confirm that
each is completed before stopping. Do not stop after completing only
part of the request.

You must plan extensively before making function calls, and reflect
extensively on the outcomes of each call, ensuring the user's query
and all sub-requests are completely resolved.
</persistence_and_planning>
```

### GPT-Specific Rules

| Rule | Details |
|------|---------|
| **Reasoning effort** | The single highest-leverage parameter. `none` for extraction/triage, `medium` for synthesis, `high` for complex decisions. Default is `medium`. |
| **previous_response_id** | Always use the Responses API and pass this back to persist reasoning across turns. ~4 point eval improvement for free. |
| **Contradiction sensitivity** | GPT-5 burns reasoning tokens trying to reconcile contradictions. Audit your prompt — if two instructions conflict, it will struggle. |
| **Verbosity parameter** | Set globally via API. Override in prompt for specific contexts (e.g., "high verbosity for code, low for status updates"). |
| **Markdown opt-in** | GPT-5 does NOT output Markdown by default in the API. You must request it. Re-inject the instruction every 3-5 turns in long conversations. |
| **Softening old prompts** | "Be THOROUGH" and "MAXIMIZE context" cause GPT-5 to over-search. Soften to: "If you're not confident after an edit, gather more info." |
| **Meta-prompting** | GPT-5 is excellent at debugging its own prompts. Ask: "Here's a prompt. It does X but should do Y. What minimal edits would fix this?" |
| **Minimal reasoning** | At `none`/`low` reasoning, add explicit CoT: "Before answering, provide a brief bullet-point summary of your reasoning." Also add stronger persistence reminders. |
| **Frontend quality** | Provide full stack defaults, UI/UX rules, and directory structure. Add self-reflection rubric for zero-to-one app generation. |
| **Proactive coding** | "Your edits will be displayed as proposed changes the user can reject. Make changes proactively rather than asking whether to proceed." |

### GPT Anti-Patterns to Avoid
- ❌ Contradictory instructions (consent required + auto-schedule without contacting)
- ❌ "Be THOROUGH" / "MAXIMIZE" — GPT-5 is already thorough, this causes over-searching
- ❌ Forgetting `previous_response_id` — you lose reasoning context between turns
- ❌ Using `xhigh` reasoning as default — it's for rare, max-intelligence tasks only
- ❌ Not specifying Markdown behavior — GPT-5 defaults to plain text in the API
- ❌ Static prompts in long conversations — re-inject formatting rules periodically
- ❌ Premature multi-agent splits — maximize single agent first

---

## GOOGLE (Gemini 3) — The Plan-Execute-Validate-Format Framework

### Architecture

```
SYSTEM INSTRUCTION (processed first, highest priority)
├── 1. Role & Persona
├── 2. Workflow (Plan → Execute → Validate → Format)
├── 3. Constraints (verbosity, tone, grounding, date awareness)
├── 4. Output Format
└── 5. Agentic Behavior (if applicable)

USER PROMPT (data BEFORE instructions)
├── 6. Context / Data (ALL reference material goes here FIRST)
├── 7. Few-Shot Examples (consistent formatting)
├── 8. Task (specific request — placed LAST)
└── 9. Anchor Phrase + Self-Critique ("Based on the above...")

API PARAMETERS
├── temperature: 1.0 (ALWAYS — do not lower)
├── thinking_level: low / medium / high
├── media_resolution: low / medium / high (for multimodal)
└── thought_signatures: pass back for stateful multi-step tool use
```

### System Instruction Template

```xml
<role>
You are [specific role] specialized in [domain].
You are precise, analytical, and persistent.
[One sentence defining what success looks like.]
</role>

<instructions>
[The canonical Gemini 3 workflow:]

1. **Plan**: Analyze the task and parse it into distinct sub-tasks.
   Check if input information is complete. Create a structured outline.
2. **Execute**: Carry out the plan step by step.
   [If tools are available:] Use tools as needed. Reflect before every call.
   Track progress: use [ ] for pending, [x] for complete.
3. **Validate**: Review your output against the user's original task.
   - Did I answer the user's intent, not just their literal words?
   - Is the tone authentic to the requested persona?
   - Have I met all constraints and format requirements?
4. **Format**: Present the final answer in the structure defined below.
</instructions>

<constraints>
- Verbosity: [Low / Medium / High]
  [Low = direct answers only. Medium = answers with brief explanations.
   High = detailed explanations with examples and reasoning.]
- Tone: [Formal / Casual / Technical]
- Current date: Today is [DATE]. Remember it is [YEAR].
- Knowledge cutoff: Your training data goes up to [DATE].
  For anything after that, use search tools if available.

[GROUNDING CLAUSE — include when the model should ONLY use provided context:]
You are a strictly grounded assistant limited to the information provided
in the context below. Rely ONLY on facts directly mentioned there.
Do not access your own knowledge or common sense. If the answer is not
explicitly in the context, state that the information is not available.

[AVOID vague negatives like "do not infer" — instead:]
You are expected to perform calculations and logical deductions based
strictly on the provided text. Avoid using outside knowledge.
</constraints>

<output_format>
[Define the exact structure:]

Structure your response as follows:
1. **Executive Summary**: [2-3 sentence overview]
2. **Detailed Response**: [Main content organized by sub-task]
3. **Confidence**: [high / medium / low with brief justification]

[OR for JSON:]
Return a JSON object with this schema:
{
  "field": "type — description"
}

[OR for structured text:]
Use the completion strategy — start the response format:
```
Category: [fill in]
Severity: [fill in]
Recommendation: [fill in]
```
</output_format>

<!-- AGENTIC ADDITIONS (only if using tools / building an agent) -->

<agentic_behavior>
You are a very strong reasoner and planner. Before taking any action
(tool calls OR responses to the user), proactively reason about:

1. **Logical dependencies**: Analyze the action against policy rules,
   order of operations, prerequisites, and user preferences.
   Resolve conflicts by priority: policy > operations > prerequisites > user prefs.

2. **Risk assessment**: What are the consequences? Will the new state
   cause future issues? For exploratory tasks (search, read), missing
   optional parameters is LOW risk — prefer calling the tool over asking.

3. **Abductive reasoning**: Identify the most logical cause for any problem.
   Look beyond obvious causes. Test hypotheses over multiple steps.
   Don't discard low-probability causes prematurely.

4. **Adaptability**: If observations contradict your plan, pivot immediately.
   Generate new hypotheses based on gathered information.

5. **Information sources**: Use all available sources:
   tools, policies, conversation history, user input.

6. **Precision**: When referring to policies or rules, quote the exact
   applicable text. Do not paraphrase rules loosely.

7. **Completeness**: Don't conclude prematurely. Check all relevant options.
   You may need to ask the user to know if something is applicable.

8. **Persistence**: Do not give up unless all reasoning is exhausted.
   On transient errors: retry (unless max retries reached).
   On other errors: change strategy, don't repeat the failed approach.

9. **Inhibit action**: Only act after all reasoning is complete.
   Once you've taken an action, you cannot take it back.
</agentic_behavior>

<tool_risk_tiers>
Low risk (proceed without asking): search, read, list, grep
Medium risk (proceed + log): update, create, send notification
High risk (always confirm): delete, payment, cancel, external API writes

For low-risk tools, never ask the user — just act.
For high-risk tools, explain what and why, then ask for confirmation.
</tool_risk_tiers>
```

### User Prompt Template

```xml
<context>
[ALL reference data goes here — documents, code, reports, conversation history.
 Place ALL context BEFORE your task/question. Gemini 3 performs best
 when it reads all data before receiving instructions.]

{{paste_all_data_here}}
</context>

<examples>
[2-4 few-shot examples. PERFECTLY consistent formatting across all examples.
 Same delimiters, whitespace, casing, arrows, tags.]

Input: "[example input 1]" → Output: "[example output 1]"
Input: "[edge case input]" → Output: "[edge case output]"
Input: "[example input 3]" → Output: "[example output 3]"
</examples>

<task>
[Your specific request — placed LAST, after all context and examples.]
{{user_request}}
</task>

<final_instruction>
Based on the information above, complete the task following the workflow
in your system instructions (Plan → Execute → Validate → Format).

Before returning your final response, review it against the original
constraints and task requirements.
</final_instruction>
```

### Gemini-Specific Rules

| Rule | Details |
|------|---------|
| **Temperature** | ALWAYS 1.0. Lowering causes looping and degraded performance on math/reasoning. |
| **Data placement** | ALL context/data FIRST in the user prompt. Instructions and task LAST. Opposite of most other models. |
| **Anchor phrases** | After large context blocks, use: "Based on the information above..." to bridge data and task. |
| **Verbosity** | Gemini 3 defaults to terse. You MUST explicitly request detailed responses if you want them. |
| **Date awareness** | Tell Gemini the current date and year. Add: "Remember it is 2026." It doesn't always know. |
| **Knowledge cutoff** | Explicitly state: "Your knowledge cutoff is January 2025." |
| **Negative constraints** | NEVER use broad negatives like "do not infer" or "do not guess." These cause the model to fail at basic logic. Use specific positive instructions instead. |
| **Few-shot examples** | Always include them. Google explicitly states zero-shot is less effective. Formatting consistency is critical — same tags, whitespace, and delimiters across all examples. |
| **Completion strategy** | Start the output format and let the model continue the pattern. This is more effective than describing the format in words. |
| **Thinking level** | `low` for fast tasks, `medium` default, `high` for deep reasoning. Can also add "think silently" to system instructions for lower latency. |
| **Thought signatures** | For multi-step tool use, pass thought signatures back in conversation history to maintain reasoning state. |
| **Media resolution** | `high` for fine text in images, `medium` for PDFs, `low` for video summaries. Controls token cost. |
| **Fallback responses** | If getting "I can't help with that" on legitimate queries, ensure temperature is at 1.0 and adjust safety settings. |
| **Self-critique** | Add before final output: "Review against original constraints. Did I answer intent? Is the tone right? Did I meet format requirements?" |

### Gemini Anti-Patterns to Avoid
- ❌ Setting temperature below 1.0 — causes looping and reasoning degradation
- ❌ Putting instructions before data — Gemini 3 wants data FIRST, instructions LAST
- ❌ "Do not infer" / "Do not guess" — kills basic logic and arithmetic
- ❌ Zero-shot prompts — always include few-shot examples
- ❌ Inconsistent example formatting — even minor whitespace differences cause format drift
- ❌ Assuming Gemini knows the current date — it often doesn't
- ❌ Expecting verbose output without requesting it — Gemini 3 defaults to terse
- ❌ Mixing XML tags and Markdown headers in the same prompt

---

# CROSS-PROVIDER COMPARISON TABLE

Use this to quickly identify which levers to pull for each provider.

| Dimension | **Anthropic (Claude)** | **OpenAI (GPT-5)** | **Google (Gemini 3)** |
|---|---|---|---|
| **Primary structure** | XML tags | XML tags + API params | XML tags or Markdown (pick one) |
| **Format control** | Prefill assistant turn | `verbosity` API param | Completion strategy (start the output) |
| **Reasoning depth** | Extended thinking toggle | `reasoning_effort` param (none→xhigh) | `thinking_level` param (low/med/high) |
| **Default verbosity** | Concise (4.x) | Concise (API) | Terse |
| **Data placement** | Anywhere (XML-delimited) | Anywhere | Data FIRST, instructions LAST |
| **Temperature** | Model-dependent | Model-dependent | ALWAYS 1.0 (never lower) |
| **Markdown** | Responds to prompt style | Opt-in (not default in API) | Responds to explicit request |
| **Key anti-pattern** | "MUST/CRITICAL" overtriggers | Contradictions burn CoT tokens | "Do not infer" kills basic logic |
| **Unique strength** | Parallel tool calling, prefill | Meta-prompting itself, reasoning_effort | Agentic system prompt template, thought signatures |
| **Multi-turn memory** | Compaction + memory tool + git | `previous_response_id` + Sessions | Thought signatures + context caching |
| **Overengineering risk** | High — needs explicit "keep it minimal" | Medium | Low (defaults terse) |
| **Tool triggering** | Dial BACK aggressive language | Define eagerness spectrum explicitly | Define risk tiers + persistence rules |
| **Few-shot examples** | Strongly recommended | Strongly recommended | Required (zero-shot explicitly worse) |
| **Frontend design** | Needs `<frontend_aesthetics>` anti-slop block | Needs stack defaults + UI/UX rules | Needs explicit verbosity + design instructions |
| **Self-reflection** | "Evaluate" (not "think") in thinking tags | Self-reflection rubric (5-7 categories) | Plan → Execute → Validate → Format workflow |
| **Date awareness** | Generally aware | Generally aware | MUST be told current date and year |

---

# QUICK-START: GENERATING A PROMPT

When an agent uses this document to generate a system prompt, follow this flow:

```
1. USER describes their use case
   ↓
2. IDENTIFY the target provider (Claude / GPT / Gemini)
   ↓
3. SELECT the provider-specific template above
   ↓
4. FILL IN each section based on the use case:
   - Role: Who is the agent?
   - Context: What does it need to know?
   - Instructions: What should it do? (positive, action-oriented)
   - Examples: 2-4 representative input/output pairs
   - Output format: Exact structure of the response
   - Reasoning: Does this task need CoT / planning?
   - Guardrails: What should it do when uncertain?
   - Tools: What tools are available? What are the risk tiers?
   ↓
5. APPLY provider-specific rules from the rules table
   ↓
6. CHECK against the anti-patterns list for that provider
   ↓
7. REVIEW for contradictions, redundancy, and implicit assumptions
   ↓
8. OUTPUT the final system prompt ready for deployment
```

---

*Built from official documentation by Anthropic, OpenAI, and Google. Updated April 2026.*