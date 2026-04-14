# OpenAI (GPT-4o / GPT-4.1) — System Prompt Guidelines for Agentic Tasks

## The Fundamental Shift: GPT-4.1 vs GPT-4o

GPT-4.1 follows instructions **more literally** than GPT-4o and its predecessors. GPT-4o tended to liberally infer intent from vague prompts. GPT-4.1 does not — it takes your words at face value.

This means:
- Prompts that worked for GPT-4o may underperform or produce unexpected results on GPT-4.1
- You must treat prompt engineering as structured technical documentation, not casual natural language
- Random/unwanted code edits dropped from 9% (GPT-4o) to 2% (GPT-4.1) thanks to literal adherence
- GPT-4.1 is "highly steerable" — a single firm sentence is usually enough to correct behavior
- When conflicting instructions exist, GPT-4.1 follows the instruction closer to the end of the prompt

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide), [Introducing GPT-4.1 (OpenAI Blog)](https://openai.com/index/gpt-4-1/)

---

## The Three Essential Agentic Reminders

OpenAI's single most impactful recommendation for agentic use. Including these three reminders increased their internal SWE-bench Verified score by ~20%.

### a) Persistence
Prevents the model from prematurely stopping and handing control back to the user.

Exact recommended language:
> "You are an agent - please keep going until the user's query is completely resolved, before ending your turn and yielding back to the user. Only terminate your turn when you are sure that the problem is solved."

### b) Tool-Calling
Prevents hallucination and encourages actual tool use over guessing.

Exact recommended language:
> "If you are not sure about file content or codebase structure pertaining to the user's request, use your tools to read files and gather the relevant information: do NOT guess or make up an answer."

### c) Planning (Optional but +4% on SWE-bench)
Forces explicit reasoning between tool calls instead of silent chaining.

Exact recommended language:
> "You MUST plan extensively before each function call, and reflect extensively on the outcomes of the previous function calls. DO NOT do this entire process by making function calls only, as this can impair your ability to solve the problem and think insightfully."

### Combined Example (from OpenAI's cookbook):
```
You are a helpful agent. Keep going until the user's query is completely resolved,
before ending your turn and yielding back to the user. Only terminate your turn when
you are sure that the problem is solved. If you are not sure about file content or
codebase structure pertaining to the user's request, use your tools to read files and
gather the relevant information: do NOT guess or make up an answer. You MUST plan
extensively before each function call, and reflect extensively on the outcomes of the
previous function calls. DO NOT do this entire process by making function calls only.
```

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)

---

## The Message Role Hierarchy (OpenAI-Specific)

OpenAI has a unique instruction hierarchy baked into model training. This is a significant differentiator from other providers.

### Authority Levels (highest to lowest)
1. **Platform** (OpenAI's internal system messages) — cannot be overridden
2. **Developer** (your application instructions) — overrides user
3. **User** (end-user inputs) — lowest authority
4. **Guideline** (implicit model spec sections)

### Message Roles in the API
- `system` role: Now reserved by OpenAI for its own platform-level instructions. On reasoning models, `system` messages auto-convert to `developer` messages.
- `developer` role: The current standard for application developers. This is what you should use for your system prompts.
- `user` role: End-user messages. Lower priority than developer.
- `assistant` / `tool` roles: No authority by default.

### Key Implication
Never inject untrusted user input into developer messages. Since developer messages have the highest developer-accessible authority, putting untrusted data there gives attackers maximum control. Instead, pass untrusted inputs through `user` messages.

> Source: [The Instruction Hierarchy (OpenAI Research)](https://openai.com/index/the-instruction-hierarchy/), [OpenAI Model Spec (2025/04/11)](https://model-spec.openai.com/2025-04-11.html), [System vs Developer Role Discussion (OpenAI Community)](https://community.openai.com/t/system-vs-developer-role-in-4o-model/1119179)

---

## Three Ways to Provide Instructions (Responses API)

OpenAI's newer Responses API introduces a third mechanism beyond system/developer messages:

### a) `instructions` Parameter
- Per-request behavioral guidance (tone, goals, examples)
- Takes priority over prompt content in `input`
- Does NOT persist across turns when using `previous_response_id`
- Must be re-sent with every request

### b) Developer Role Messages
- Persist across conversation chains
- Can include multiple developer messages (treated cumulatively)
- Later/more specific messages override earlier ones at the same level

### c) System Role Messages (Legacy)
- Auto-converted to developer messages for reasoning models
- Still supported in Chat Completions API

### In Practice
Community testing found that when both `instructions` and developer role messages are present in the same request, the developer role message takes precedence (contrary to some documentation suggesting instructions take priority). OpenAI recommends choosing one approach rather than mixing both.

> Source: [Instructions vs Developer Role Discussion (OpenAI Community)](https://community.openai.com/t/response-endpoint-instructions-parameter-vs-developer-role/1312916), [System and Developer Roles in Responses API (OpenAI Community)](https://community.openai.com/t/system-and-developer-roles-in-messages-and-instructions-in-responses-create/1370516)

---

## Recommended Prompt Structure

OpenAI provides a specific template for system prompts:

```
# Role and Objective
[Who the model is and what it does]

# Instructions
## Sub-categories for detailed instructions
[Specific rules, constraints, behaviors]

# Reasoning Steps
[How the model should think through problems]

# Output Format
[Exact specification of response format]

# Examples
[Input/output pairs demonstrating desired behavior]

# Context
[Supporting data, documents, reference material]

# Final instructions to think step by step
[Closing reminder to plan and reason]
```

### Key Structural Notes
- Place instructions at both the **beginning AND end** of provided context (sandwich pattern — better than one location alone)
- Use **Markdown** (headers, lists, code blocks) as the default formatting for prompts
- Use **XML** for large document collections and long context — it "performed well in long context testing" and supports metadata attributes and nesting
- **Avoid JSON** for document-heavy contexts — it "performed particularly poorly" in testing
- The recommended XML format for documents: `<doc id='1' title='The Fox'>content here</doc>`

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide), [Prompt Engineering Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/prompt-engineering)

---

## Function Calling / Tool Use Best Practices

GPT-4.1 has undergone more training on tool use than previous models. Using the API's native `tools` field versus manually injecting schemas into prompts yielded a 2% increase in SWE-bench pass rate.

### Tool Definition Best Practices
- **Always use the `tools` parameter** — never manually inject tool descriptions into system prompts
- Write clear, detailed function names and parameter descriptions
- Apply the "intern test": could someone use the function with only the provided documentation?
- Use enums and object structure to make invalid states unrepresentable
- Don't require the model to fill arguments you already know
- Combine functions that are always called sequentially
- Keep **fewer than 20 functions** at the start of a turn for higher accuracy
- For complicated tools, create an `# Examples` section in the system prompt rather than embedding examples in the tool description field

> Source: [Function Calling Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/function-calling), [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)

### Strict Mode
- Set `strict: true` on all function definitions (recommended always)
- Requires `additionalProperties: false` on every object in parameters
- All fields must be listed under `required`
- Optional fields should use `null` as a type option (e.g., `"type": ["string", "null"]`)

> Source: [Structured Outputs Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/structured-outputs)

### Tool Choice Options
- `"auto"` (default): Model decides whether to call tools
- `"required"`: Model must invoke at least one tool
- `"none"`: Prevents all tool calling (useful for planning steps)
- Named function: Forces exactly one specific function call
- `allowed_tools`: Restrict to a subset while keeping full list for prompt caching

> Source: [Function Calling Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/function-calling)

### Parallel Tool Calls
- Enabled by default; model can call multiple functions in one turn
- Known issue: Rare incorrect parallel calls observed, especially with `gpt-4.1-nano-2025-04-14`
- Set `parallel_tool_calls: false` if you encounter problems
- Strict mode is disabled when fine-tuned models make multiple function calls in one turn

> Source: [Function Calling Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/function-calling)

### Namespace Organization (Newer Feature)
```json
{
  "type": "namespace",
  "name": "crm",
  "description": "CRM tools for customer lookup and order management.",
  "tools": [
    { "type": "function", "name": "get_customer_profile", ... }
  ]
}
```

> Source: [Function Calling Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/function-calling)

---

## Prompt Injection Defense (OpenAI-Specific)

OpenAI's approach to prompt injection is architecturally unique:

- The Instruction Hierarchy is trained into the model itself — system > developer > user > tool
- Quoted text, tool outputs, images, and `untrusted_text` blocks have no authority by default unless explicitly delegated by higher-level instructions
- Use `untrusted_text` blocks for any user-provided or third-party content
- Use YAML, JSON, or XML formatting to delineate untrusted data when `untrusted_text` blocks are unavailable
- Never pass untrusted input in developer messages — always use user messages for untrusted content
- Design agents so that the impact of manipulation is constrained even if it succeeds

> Source: [The Instruction Hierarchy (OpenAI Research)](https://openai.com/index/the-instruction-hierarchy/), [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)

---

## Prompt Caching Optimization

OpenAI's automatic prompt caching is relevant to how you structure system prompts:

- Caching works on the longest shared prefix (minimum 1024 tokens, then every 128 tokens)
- Static content must come first, dynamic content at the end
- Cached input tokens are 50% cheaper across all models
- Up to 80% latency reduction with cache hits
- No code changes required — it's automatic
- Structure system prompts with stable instructions at the top and variable context/user data at the bottom

> Source: [Prompt Caching (OpenAI API Docs)](https://platform.openai.com/docs/guides/prompt-caching)

---

## Known Quirks and Anti-Patterns

### Things to Avoid
- **Vague instructions**: "Be helpful" or "Don't include irrelevant info" without defining what "relevant" means
- **Over-specifying tool requirements**: Saying "always use tools" causes hallucinated tool inputs when the model lacks enough information. Add: "if you don't have enough information, ask the user"
- **Sample phrases without variation guidance**: The model will reuse example phrases verbatim. Add: "vary your language"
- **ALL CAPS and incentive language**: Skip unless truly necessary — GPT-4.1 responds well to calm, specific instructions
- **JSON for document delimiters**: Performs poorly compared to Markdown and XML
- **Mixing instructions and developer role messages**: Pick one approach
- **Relying on implicit intent**: GPT-4.1 won't infer what you mean; be explicit

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)

### Known Behaviors
- Model may resist producing very long, repetitive outputs (e.g., analyzing hundreds of items one-by-one). Instruct strongly if needed.
- Later instructions in the prompt override earlier ones when they conflict
- GPT-4o returns Markdown formatting by default (headers, backticks) unlike GPT-4
- Log probability distributions differ between `developer` and `system` roles even for identical content in GPT-4o

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide), [System vs Developer Role Discussion (OpenAI Community)](https://community.openai.com/t/system-vs-developer-role-in-4o-model/1119179)

---

## GPT-5 / GPT-5.4 Specific Additions

The internal framework and checklist documents reference GPT-5/5.4 with additional behaviors beyond GPT-4.1. These apply to the newer model family.

### `reasoning_effort` Parameter
The single highest-leverage parameter for GPT-5. Controls how deeply the model reasons:
- `none` — field extraction, triage, classification
- `low` — simple lookups
- `medium` — synthesis, multi-doc review (default)
- `high` — complex decisions, refund logic
- `xhigh` — rare, max-intelligence tasks only

```python
client.responses.create(model="gpt-5", reasoning={"effort": "none"}, ...)   # classify
client.responses.create(model="gpt-5", reasoning={"effort": "high"}, ...)   # complex refund
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### `previous_response_id` for Reasoning Persistence
Always use the Responses API and pass this back to persist reasoning across turns. ~4 point eval improvement for free:
```python
response = client.responses.create(model="gpt-5", input="Search for X", tools=tools)
response2 = client.responses.create(
    model="gpt-5", input="Summarize findings",
    previous_response_id=response.id  # reasoning persists
)
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### `verbosity` API Parameter
Set globally via API. Override in prompt for specific contexts:
```python
client.responses.create(model="gpt-5", verbosity="low", ...)
```
```
"Write code for clarity first. Use high verbosity for code tools."
```

> Source: Internal framework (`prompt-framework.md`)

### Contradiction Sensitivity
GPT-5 burns reasoning tokens trying to reconcile contradictions. Audit your prompt — if two instructions conflict, it will waste CoT tokens searching for reconciliation:
```
Bad: "Never schedule without consent."
   + "For urgent cases, auto-assign without contacting the patient."

Good: "Never schedule without consent.
       For urgent cases, auto-assign AFTER informing the patient.
       If consent status is unknown, tentatively hold and request."
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Softening Aggressive Prompts from Older Models
"Be THOROUGH" and "MAXIMIZE context" cause GPT-5 to over-search on simple tasks:
```
Bad:  <maximize_context_understanding>
      Be THOROUGH. Make sure you have the FULL picture.
Good: <context_understanding>
      If you're not confident after a partial edit, gather more info.
      Bias towards finding answers yourself.
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Markdown Is NOT Default in API
GPT-5 does NOT output Markdown by default in the API. You must explicitly request it. Re-inject the instruction every 3-5 turns in long conversations:
```python
if turn_count % 4 == 0:
    messages.append({"role": "system",
        "content": "Reminder: format code with backticks, use ### headers."})
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Meta-Prompting
GPT-5 is excellent at debugging its own prompts:
```
"Here's a prompt: {{prompt}}
Desired behavior: {{desired}}
Actual behavior: {{actual}}
What minimal edits would fix this?"
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Self-Reflection Rubric (Zero-to-One Apps)
For quality-critical tasks like app generation:
```xml
<self_reflection>
Think deeply about what makes a world-class web app.
Create a 5-7 category rubric (don't show it).
Use it to internally iterate. If not hitting top marks, start again.
</self_reflection>
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Agentic Eagerness Calibration
Control how aggressively the model searches for information:

For less eagerness (fast, focused):
```xml
<context_gathering>
Goal: Get enough context fast. Stop as soon as you can act.
Early stop: You can name exact content to change.
Maximum 2 tool calls. Bias toward answering quickly.
</context_gathering>
```

For more eagerness (thorough, autonomous):
```xml
<persistence>
Keep going until the query is completely resolved.
Never stop when uncertain — deduce and continue.
Don't ask the human to confirm assumptions.
</persistence>
```

> Source: Internal framework (`prompt-framework.md`)

### Tool Preambles — Narrate Before Acting
```xml
<tool_preambles>
Always begin by rephrasing the user's goal before calling tools.
Outline a structured plan. Narrate each step succinctly.
Finish by summarizing completed work.
</tool_preambles>
```

> Source: Internal framework (`prompt-framework.md`)

### Tool Safety Tiers
```xml
<tool_safety>
Low risk (proceed freely): search, grep, read_file
Medium risk (proceed + log): update_record, send_notification
High risk (confirm first): delete_file, process_payment
</tool_safety>
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### For Minimal Reasoning, Add Explicit Planning
At `none`/`low` reasoning effort, add explicit CoT since there are fewer internal tokens for planning:
```
"Before your final answer, provide a brief bullet-point
summary of your reasoning at the top."
```

> Source: Internal checklist (`prompt-guides.md`)

---

## Complete System Prompt Template (GPT-5)

```xml
<agent_identity>
You are [role] — a [type of agent].
Your responsibilities:
- [Primary responsibility]
- [Secondary responsibility]
You operate as an autonomous agent that [completes tasks / advises / implements].
</agent_identity>

<tool_preambles>
Always begin by rephrasing the user's goal before calling tools.
Outline a structured plan. Narrate each step succinctly.
Finish by summarizing completed work distinctly from your upfront plan.
</tool_preambles>

<context_gathering>
[Controls eagerness — choose one:]
<!-- Less eager: -->
Goal: Get enough context fast. Parallelize discovery. Stop when you can act.
Maximum [N] tool calls for context gathering.

<!-- More eager: -->
Keep going until the query is completely resolved.
Only terminate when you are sure the problem is solved.
</context_gathering>

<tool_safety>
Low risk (proceed freely): search, grep, read_file
Medium risk (proceed + log): update_record, send_notification
High risk (confirm first): delete_file, process_payment
</tool_safety>

<domain_rules>
[Task-specific rules, code standards, business logic.]
</domain_rules>

<output_contract>
[Controls format, length, and structure of the final answer.]
</output_contract>

<persistence_and_planning>
Decompose the query into all required sub-requests.
Confirm each is completed before stopping.
Plan extensively before making function calls.
Reflect extensively on outcomes of each call.
</persistence_and_planning>
```

> Source: Internal framework (`prompt-framework.md`)

---

## Additional Agent Building Patterns (from OpenAI's Practical Guide)

### Categorize Tools: Data, Action, Orchestration
Different risk profiles need different guardrails:
```python
# Data tool (read-only, low risk)
@function_tool
def search_kb(query: str): ...

# Action tool (write, medium risk)
@function_tool
def issue_refund(order_id: str, amount: float): ...

# Orchestration tool (agent-as-tool)
research_tool = research_agent.as_tool(
    tool_name="deep_research",
    tool_description="Performs multi-source research on a topic"
)
```

> Source: `agent-guides.md`, [A Practical Guide to Building Agents (OpenAI)](https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf)

### Convert Existing Docs/SOPs into Agent Instructions
```
"You are an expert in writing instructions for an LLM agent.
Convert the following help center document into a clear set of
numbered instructions for an agent to follow.
Ensure there is no ambiguity.
Document: {help_center_doc}"
```

> Source: `agent-guides.md`, [A Practical Guide to Building Agents (OpenAI)](https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf)

### Maximize Single-Agent Before Going Multi-Agent
More agents introduce complexity and overhead. Only split when instructions have too many if-else branches or tools overlap and the agent picks wrong ones consistently. Two multi-agent patterns:
- **Manager**: One agent calls sub-agents, synthesizes answers, talks to user
- **Decentralized**: Triage identifies intent, hands off entirely to specialist

> Source: `agent-guides.md`, [A Practical Guide to Building Agents (OpenAI)](https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf)

### Context Summarization with Hallucination Control
```
"Before writing the summary (do this silently):
 - Contradiction check: compare user claims with tool logs. Note conflicts.
 - Temporal ordering: sort key events by time; most recent update wins.
 - If any fact is uncertain, mark it UNVERIFIED.
 - Do not invent new facts. Quote error strings/codes exactly."
```

> Source: `agent-guides.md`, [Context Engineering - Session Memory (OpenAI Cookbook)](https://cookbook.openai.com/examples/agents_sdk/session_memory)

### Layer Multiple Guardrails
A single guardrail is insufficient. Types to consider:
- Relevance classifier (flags off-topic queries)
- Safety classifier (detects jailbreaks/prompt injections)
- PII filter (prevents exposure of PII)
- Moderation (flags hate speech, harassment, violence)
- Tool safeguards (risk-rate each tool)
- Rules-based protections (blocklists, input length limits, regex filters)
- Output validation (ensures responses align with brand values)

> Source: `agent-guides.md`, [A Practical Guide to Building Agents (OpenAI)](https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf)

---

## Contradictions Found

### GPT Model Versions
- **Internet research** focused on GPT-4o and GPT-4.1 (the models available as of the research date)
- **Internal framework** (`prompt-framework.md`) and **internal checklist** (`prompt-guides.md`) reference GPT-5 and GPT-5.4 with additional parameters (`reasoning_effort`, `verbosity`, `previous_response_id`)
- **Resolution**: Both are relevant. GPT-4.1's literal instruction following and 3 agentic reminders apply broadly. GPT-5's additional API parameters (`reasoning_effort`, `previous_response_id`, `verbosity`) are additive. Use the GPT-4.1 prompting principles as the foundation, add GPT-5 parameters when using those models.

### Markdown Default Behavior
- **Internet research** (GPT-4o): "GPT-4o returns Markdown formatting by default"
- **Internal guides** (GPT-5): "GPT-5 does NOT output Markdown by default in the API"
- **Resolution**: This is a real behavioral change between model generations. GPT-4o defaults to Markdown; GPT-5 does not. Must explicitly request Markdown formatting for GPT-5.

---

## Key Differentiators (What Makes OpenAI Unique)

1. **Instruction Hierarchy trained into the model**: Unlike most providers, OpenAI has explicit authority levels (platform > developer > user) trained via synthetic data
2. **Developer role vs System role**: OpenAI is migrating from `system` to `developer` as the standard role
3. **Responses API with `instructions` parameter**: A separate mechanism from message roles, providing per-request behavioral guidance
4. **Native tool field preference**: OpenAI explicitly recommends against putting tool schemas in system prompts (measured 2% accuracy improvement)
5. **Strict mode for function calling**: Schema-enforced function calling that guarantees output conformance
6. **Literal instruction following**: GPT-4.1 is trained for literal interpretation, requiring fundamentally different prompting than models that infer intent
7. **Automatic prompt caching**: Prefix-based, no configuration needed, but requires careful prompt structure
8. **Namespace organization for tools**: Domain-based grouping of related functions

> Source: [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide), [The Instruction Hierarchy (OpenAI Research)](https://openai.com/index/the-instruction-hierarchy/)

---

## All Sources

- [GPT-4.1 Prompting Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)
- [Prompt Migration Guide (OpenAI Cookbook)](https://developers.openai.com/cookbook/examples/prompt_migration_guide)
- [Prompt Engineering Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/prompt-engineering)
- [Text Generation Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/text)
- [Function Calling Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/function-calling)
- [Structured Outputs Guide (OpenAI API Docs)](https://developers.openai.com/api/docs/guides/structured-outputs)
- [OpenAI Model Spec (2025/04/11)](https://model-spec.openai.com/2025-04-11.html)
- [The Instruction Hierarchy (OpenAI Research)](https://openai.com/index/the-instruction-hierarchy/)
- [Prompt Caching (OpenAI API Docs)](https://platform.openai.com/docs/guides/prompt-caching)
- [System vs Developer Role Discussion (OpenAI Community)](https://community.openai.com/t/system-vs-developer-role-in-4o-model/1119179)
- [Instructions vs Developer Role Discussion (OpenAI Community)](https://community.openai.com/t/response-endpoint-instructions-parameter-vs-developer-role/1312916)
- [System and Developer Roles in Responses API (OpenAI Community)](https://community.openai.com/t/system-and-developer-roles-in-messages-and-instructions-in-responses-create/1370516)
- [Introducing GPT-4.1 (OpenAI Blog)](https://openai.com/index/gpt-4-1/)
- [GPT-4.1 Agent Mode System Prompt Example (GitHub Gist)](https://gist.github.com/burkeholland/a2ae7a2bca478f4fbf1768beb9b4c1f8)
