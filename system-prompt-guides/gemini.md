# Google Gemini — System Prompt Guidelines for Agentic Tasks

## System Instructions: Fundamentals

### Terminology
Google uses the term **"system instructions"** (not "system prompts"). They are passed via the `system_instruction` parameter in `GenerateContentConfig`, not as part of the message/content history. System instructions are processed as a separate, privileged layer before the model processes any user prompts.

> Source: [Gemini API — System Instructions](https://ai.google.dev/gemini-api/docs/system-instructions)

### How They Work
- System instructions apply to the entire request and persist across multiple user/model turns in a conversation
- The model does not distinguish between cached system instruction tokens and regular input tokens
- System instructions are **text-only** (no images, audio, or other modalities allowed)
- They count toward token limits

> Source: [Gemini API — System Instructions](https://ai.google.dev/gemini-api/docs/system-instructions)

### What Google Says to Put in System Instructions
1. **Persona/Role definition** — who the model is, depth of expertise
2. **Output format** — Markdown, JSON, YAML, specific templates
3. **Style and tone** — verbosity, formality, reading level
4. **Goals and rules** — constraints, requirements (e.g., "include docstrings in code")
5. **Contextual information** — background, knowledge cutoffs, domain context
6. **Language handling** — "Respond in the same language as the query"

> Source: [Gemini API — System Instructions](https://ai.google.dev/gemini-api/docs/system-instructions)

### Critical Limitation
Google explicitly warns: "System instructions can help guide the model to follow instructions, but they don't fully prevent jailbreaks or leaks." Do NOT put sensitive information (API keys, secrets, proprietary logic you cannot afford to leak) in system instructions.

> Source: [Gemini API — System Instructions](https://ai.google.dev/gemini-api/docs/system-instructions)

---

## Gemini-Specific Prompt Structure and Ordering

This is where Gemini diverges significantly from other providers. Google has documented specific ordering rules.

### The Ordering Rule for Constraints
Negative constraints and formatting constraints should be placed at the **END** of the instruction, not at the beginning. This is counterintuitive if you come from Claude/OpenAI where front-loading constraints is typical.

The recommended structure for Gemini:
1. **Context and source material** (first)
2. **Main task instructions** (middle)
3. **Negative constraints, formatting constraints, quantitative constraints** (last)

Google's rationale: "When dealing with sufficiently complex requests, the model may drop negative constraints or formatting constraints if they appear too early in the prompt."

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### Long Context Ordering
When providing large amounts of context (documents, code, data): supply all the context FIRST, then place your specific instructions or questions at the very END of the prompt.

> Source: [Gemini API — Long Context](https://ai.google.dev/gemini-api/docs/long-context)

### XML Tags and Markdown Headings
Google recommends using either XML-style tags or Markdown headings consistently to structure system instructions:
```xml
<role>Define purpose</role>
<constraints>List limitations</constraints>
<context>Background information</context>
<task>Specific request</task>
```
The key requirement is consistency — pick one format and use it throughout. Do not mix XML and Markdown within a single prompt.

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

---

## Gemini 3-Specific Behaviors and Quirks

### Temperature: DO NOT LOWER IT
This is perhaps the single most important Gemini-specific rule. Google strongly recommends keeping temperature at its default value of `1.0` for Gemini 3 models. Quote: "Changing the temperature (setting it to less than 1.0) may lead to unexpected behavior, such as looping or degraded performance, particularly with complex mathematical or reasoning tasks."

This is a stark contrast to OpenAI and Claude where lowering temperature for deterministic tool calling is standard advice.

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### Default Verbosity Is Low
Gemini 3 models default to concise, direct answers. If you need conversational or detailed responses, you must explicitly request it: "Explain this as a friendly, talkative assistant." This differs from Claude (which tends toward thorough explanations by default) and GPT-4 (which tends toward moderate verbosity).

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### Persona Prioritization
Gemini treats assigned personas very seriously and may prioritize persona adherence over conflicting instructions. Google warns: "Review personas carefully and avoid ambiguous scenarios." This means if your persona says "you are a pirate" but your instructions say "respond formally," the pirate persona may win.

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### Thinking/Reasoning
- Gemini 3 has a `thinkingLevel` parameter: `minimal`, `low`, `medium`, `high`
- Gemini 2.5 uses `thinkingBudget` (token count, 0-32768)
- For complex tasks, simple prompts like "Think very hard" can enhance reasoning without requiring explicit step-by-step chain-of-thought instructions
- If you are already using Gemini's thinking feature, Google says to try prompting without step-by-step instructions first — the model's internal thinking may already handle it
- To reduce latency: set thinking level to `LOW` and add "think silently" to system instructions

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### "Do Not Infer" Anti-Pattern
Google specifically warns against broad negative constraints like "do not infer." Instead, be specific: "You are expected to perform calculations and logical deductions based strictly on the provided text. Do not introduce external information."

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

---

## Agentic System Instructions: The Template

Google provides a specific system instruction template for agentic workflows that they say improved performance on agentic benchmarks by approximately 5%. The template encourages the agent to "act as a strong reasoner and planner" and enforces behaviors across three categories.

### Category 1: Reasoning and Strategy
- **Logical decomposition** — how thoroughly to analyze constraints, prerequisites, and operation sequencing
- **Problem diagnosis** — depth of analysis for root causes; whether to accept obvious answers or explore less probable hypotheses (abductive reasoning)
- **Information exhaustiveness** — trade-off between analyzing every policy/document versus prioritizing speed

### Category 2: Execution and Reliability
- **Adaptability** — whether to strictly follow initial plans or pivot with new data
- **Persistence and recovery** — self-correction attempts, error handling, retry behavior
- **Risk assessment** — distinguishing exploratory read operations from high-risk write operations

### Category 3: Interaction and Output
- **Ambiguity and permission handling** — when to make assumptions versus when to pause and ask the user
- **Verbosity** — whether to explain actions or remain silent during tool execution
- **Precision and completeness** — whether to solve every edge case or optimize for the common path

> Source: [Gemini API — Agentic Behaviors](https://ai.google.dev/gemini-api/docs/agentic)

### The 9 Reasoning Areas in the Template
The template requires the agent to plan across these areas before taking action:
1. Logical dependencies and constraints against policy rules, prerequisites, user preferences
2. Risk assessment — evaluating action consequences and future state impacts
3. Abductive reasoning — identifying root causes through hypothesis exploration
4. Outcome evaluation — adapting plans when observations contradict assumptions
5. Information availability — incorporating tools, policies, history, user input
6. Precision and grounding — verifying claims with exact applicable information
7. Completeness — exhaustively addressing all requirements and resolving conflicts
8. Persistence — retrying transient errors unless explicit limits reached
9. Response inhibition — completing reasoning before taking irreversible actions

The core directive is: "Before taking any action, you must proactively, methodically, and independently plan and reason" through dependencies, risks, and outcome evaluation.

> Source: [Gemini API — Agentic Behaviors](https://ai.google.dev/gemini-api/docs/agentic)

---

## Function Calling: Complete Best Practices

### Function Declaration Writing
- **Names**: Use descriptive names with underscores or camelCase. No spaces, periods, or dashes.
- **Descriptions**: "Be extremely clear and specific in your descriptions. The model relies on these to choose the correct function and provide appropriate arguments." Include examples: "Finds theaters based on location and optionally movie title..."
- **Parameters**: Use strong typing (integer, string, enum). For fixed-value parameters, **always use `enum` arrays** rather than describing valid values in the description text. This significantly improves accuracy.
- **Required fields**: Explicitly mark which parameters are required.

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)

### Tool Count Management
- Aim for 10-20 maximum active tools. More tools increase the risk of incorrect selection.
- For large tool sets, implement dynamic tool selection based on conversation context.
- Generic low-level tools (like `bash`) get used more often but with less accuracy. Specific high-level tools (like `get_weather`) get used less often but more accurately.

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)

### Function Calling Modes

| Mode | When to Use |
|---|---|
| **AUTO** | Default. Model decides whether to call a function or respond with text. |
| **ANY** | Forces the model to always make a function call. |
| **VALIDATED** | Default when combining tools. Stricter schema adherence than AUTO. |
| **NONE** | Temporarily disable function calling without removing tool definitions. |

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)

### CRITICAL: AUTO Mode Degrades in Long Conversations
A documented issue: In AUTO mode, the model deprioritizes or "forgets" tool use as the conversation gets longer. For long-running agentic sessions, explicitly setting the mode (e.g., VALIDATED or ANY) may be necessary to maintain reliable tool calling.

> Source: [Google AI Forum — Function Calling Issues](https://discuss.ai.google.dev/), community reports

### Thought Signatures (Gemini 3 Only — MANDATORY)
When Gemini 3 makes a function call, it includes an encrypted `thought_signature` in the response. This MUST be passed back to the model in the next turn, or you get a 400 validation error. Rules:
- Always return `thought_signature` inside its original `Part`
- Always include the exact `id` from `function_call` in your `function_response`
- Never merge Parts with signatures and Parts without signatures
- Never combine two Parts that both contain signatures
- For parallel calls: only the first call gets a signature
- For sequential/chained calls: each step gets its own signature, and ALL must be accumulated and returned

The SDKs handle this automatically. This is only a concern if you are manually constructing API requests or manipulating conversation history.

Workaround for injected function calls without signatures: use dummy values `"context_engineering_is_the_way_to_go"` or `"skip_thought_signature_validator"` in the signature field.

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling), [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### System Instructions for Function Calling
Google recommends including in your system instructions:
- **Context**: Role definition like "You are a helpful weather assistant"
- **Tool usage instructions**: "Don't guess dates; always use a future date for forecasts"
- **Clarification encouragement**: Tell the model to ask clarifying questions when user input is ambiguous
- **Current date/time/location**: Include these in system instructions when functions use date, time, or location parameters — the model needs this context even if the user doesn't provide it

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)

### Parallel vs Compositional Calling
- **Parallel**: Multiple independent functions called simultaneously. Results mapped back via `id` — order does not matter.
- **Compositional**: Sequential chaining where one function's output feeds into the next. The model determines sequencing internally.

> Source: [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)

---

## Caching and Cost Implications

### Implicit Caching
- Automatically enabled on Gemini 2.5+ models
- No guaranteed cost savings
- Minimum token thresholds: 1024 tokens (Flash models), 4096 tokens (Pro models)
- Place large, stable content at the beginning of prompts to maximize cache hits
- Known issue: system prompts with long common prefixes (>9k tokens) but different suffixes produce inconsistent cache hits

> Source: [Gemini API — Context Caching](https://ai.google.dev/gemini-api/docs/caching)

### Explicit Caching
- Guaranteed cost savings
- System instructions can be included in cached content via `system_instruction` parameter in `CreateCachedContentConfig`
- **CRITICAL limitation**: When using explicit caching, you cannot simultaneously use `system_instruction`, `tools`, or `tool_config` in the `GenerateContent` request. They must all be in the cached content. This is a significant architectural constraint for agentic systems.

> Source: [Gemini API — Context Caching](https://ai.google.dev/gemini-api/docs/caching)

---

## Known Issues and Anti-Patterns

### Instruction Following Degrades with Complexity
Community reports and Google forum discussions confirm that Gemini's instruction following drops to approximately 78% on complex nested constraints in dense system prompts (4,000+ words). Google has acknowledged this as an area for improvement.

Specific failure modes:
- Skipping mandatory workflow steps (jumping straight to code without research/planning phases)
- Producing minimal output despite comprehensiveness requirements
- Losing context in extended conversations
- Ignoring negative constraints, "believing its approach is superior"
- Truncating system prompts, missing closing tags
- Short-term memory failures — repeating mistakes immediately after acknowledging them

> Source: [Google AI Forum Discussions](https://discuss.ai.google.dev/), community reports

### Workaround: Small, Focused Sessions
Community-developed workaround: break complex agentic tasks into small, testable steps in isolated sessions with dedicated, focused instructions rather than one massive system prompt.

> Source: Community best practices / forum reports

### Looping
If temperature is set below 1.0 on Gemini 3, the model may enter infinite loops, especially on complex reasoning tasks. Always keep temperature at 1.0.

> Source: [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)

### The "Lazy" Generation Pattern
Users report Gemini engaging in "token saving behaviors" — truncating outputs, generating incomplete code, missing closing tags. Explicit instructions about completeness may be needed.

> Source: Community reports / Google AI Forum

---

## Live API-Specific System Instruction Structure

For real-time/streaming agentic use cases via the Live API, Google recommends this specific ordering within system instructions:

1. **Agent persona** (name, role, characteristics, language/accent)
2. **Conversational rules** (ordered by expected flow; distinguish one-time elements from loops)
3. **Tool invocation guidance** ("Specify tool calls within a flow in distinct sentences")
4. **Guardrails** (unwanted behaviors with examples)

Additional Live API notes:
- The model performs best with single function calls (not parallel)
- Async function calling is not yet supported in Gemini 3.1 Flash Live
- Tool responses must be handled manually (no automatic tool calling)
- For non-English: "RESPOND IN {OUTPUT_LANGUAGE}. YOU MUST RESPOND UNMISTAKABLY IN {OUTPUT_LANGUAGE}."

> Source: [Gemini API — Live API](https://ai.google.dev/gemini-api/docs/live)

---

## Complete System Prompt Template (Plan-Execute-Validate-Format)

From the internal framework, the recommended end-to-end template for Gemini 3:

### System Instruction
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
- Tone: [Formal / Casual / Technical]
- Current date: Today is [DATE]. Remember it is [YEAR].
- Knowledge cutoff: Your training data goes up to [DATE].

[GROUNDING CLAUSE — when model should ONLY use provided context:]
You are a strictly grounded assistant limited to the information provided
in the context below. Rely ONLY on facts directly mentioned there.
Do not access your own knowledge or common sense. If the answer is not
explicitly in the context, state that the information is not available.
</constraints>

<output_format>
Structure your response as follows:
1. **Executive Summary**: [2-3 sentence overview]
2. **Detailed Response**: [Main content organized by sub-task]
3. **Confidence**: [high / medium / low with brief justification]
</output_format>

<!-- AGENTIC ADDITIONS -->
<agentic_behavior>
You are a very strong reasoner and planner. Before taking any action:

1. **Logical dependencies**: Analyze against policy rules, order of operations,
   prerequisites, user preferences. Resolve conflicts by priority:
   policy > operations > prerequisites > user prefs.
2. **Risk assessment**: Consequences? Low risk → proceed without asking.
3. **Abductive reasoning**: Look beyond obvious causes.
4. **Adaptability**: If observations contradict plan, pivot immediately.
5. **Information sources**: Use all: tools, policies, history, user input.
6. **Precision**: Quote exact policies when referring to them.
7. **Completeness**: Don't conclude prematurely.
8. **Persistence**: Retry transient errors. On other errors, change strategy.
9. **Inhibit action**: Only act after all reasoning is complete.
</agentic_behavior>

<tool_risk_tiers>
Low risk (proceed without asking): search, read, list, grep
Medium risk (proceed + log): update, create, send notification
High risk (always confirm): delete, payment, cancel, external API writes
</tool_risk_tiers>
```

### User Prompt Template
```xml
<context>
[ALL reference data goes here FIRST — documents, code, reports.
 Gemini 3 performs best when it reads all data before receiving instructions.]
{{paste_all_data_here}}
</context>

<examples>
[2-4 few-shot examples. PERFECTLY consistent formatting.]
Input: "[example input 1]" → Output: "[example output 1]"
Input: "[edge case input]" → Output: "[edge case output]"
</examples>

<task>
[Specific request — placed LAST, after all context and examples.]
{{user_request}}
</task>

<final_instruction>
Based on the information above, complete the task following the workflow
in your system instructions (Plan → Execute → Validate → Format).
Before returning your final response, review it against the original
constraints and task requirements.
</final_instruction>
```

> Source: Internal framework (`prompt-framework.md`)

---

## Additional Techniques from Internal Guides

### Anchor Phrases After Large Context Blocks
Use a clear transition phrase to bridge context and your query:
```
<context>{{entire codebase}}</context>

Based on the codebase above, identify all functions that perform
database writes without transaction wrappers.
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Date Awareness Is Mandatory
Gemini often doesn't know the current date. Always include:
```
"Today's date is April 7, 2026. Your knowledge cutoff is
January 2025. For current events, always search.
Remember it is 2026."
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Completion Strategy — Start the Output
Starting the output pattern forces the model to follow it (more effective than describing the format):
```
Order: Two burgers, a drink, and fries.
Output:
{"hamburger": 2, "drink": 1, "fries": 1}

Order: A cheeseburger and a coffee.
Output:
```

> Source: Internal checklist (`prompt-guides.md`)

### Few-Shot Examples Are Required
Google explicitly states zero-shot is less effective. Formatting consistency is critical — even minor whitespace differences cause format drift:
```
Bad:
  Input: "hello" → Output: greeting
  Input: "buy shoes"
  output: purchase_intent    # different casing, missing arrow

Good:
  Input: "hello" → Output: greeting
  Input: "buy shoes" → Output: purchase_intent
  Input: "refund please" → Output: refund_request
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Media Resolution Parameter
Controls multimodal token costs:
- `high` for fine text in images
- `medium` for PDFs
- `low` for video summaries

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Self-Critique Before Final Output
```
"Before returning your response, review it:
 1. Did I answer intent, not just literal words?
 2. Is the tone right?
 3. Did I meet all output format requirements?"
```

> Source: Internal framework (`prompt-framework.md`), Internal checklist (`prompt-guides.md`)

### Fallback Responses Workaround
If getting "I can't help with that" on legitimate queries, ensure temperature is at 1.0 and adjust safety settings:
```python
response = model.generate(
    prompt=legitimate_question,
    temperature=1.0,
    safety_settings={"HARM_CATEGORY_DANGEROUS": "BLOCK_ONLY_HIGH"}
)
```

> Source: Internal checklist (`prompt-guides.md`)

### Google Agent Architecture: Three Core Components
An agent = Model + Tools + Orchestration layer. Google categorizes tools into:
- **Extensions**: Agent calls APIs directly (agent-side)
- **Functions**: Agent generates params, YOU call the API (client-side, for auth/human-in-the-loop)
- **Data Stores**: RAG over your documents

Use Functions when you need auth control or human-in-the-loop. Teach Extensions with examples, not just descriptions.

> Source: `agent-guides.md`, [Introduction to Agents Whitepaper (Google)](https://ppc.land/content/files/2025/01/Newwhitepaper_Agents2.pdf)

### Reasoning Framework: ReAct
Google recommends the ReAct (Reason + Act) pattern:
```
"For each user request, follow this loop:
 1. Thought: reason about what you need to do next
 2. Action: choose a tool and its inputs
 3. Observation: read the tool result
 4. Repeat until you have enough info, then give Final Answer."
```

> Source: `agent-guides.md`, [Introduction to Agents Whitepaper (Google)](https://ppc.land/content/files/2025/01/Newwhitepaper_Agents2.pdf)

### Deterministic Guardrails Outside the Model
Hardcoded rules as a security chokepoint the model cannot bypass:
```python
def guardrail_check(tool_call):
    if tool_call.name == "make_purchase" and tool_call.args["amount"] > 100:
        raise BlockedAction("Purchases over $100 require human approval")
    if tool_call.name == "delete_account":
        raise BlockedAction("Account deletion is never automated")
    return tool_call
```

> Source: `agent-guides.md`, [Building AI Agents with Gemini 3 (Google Blog)](https://developers.googleblog.com/building-ai-agents-with-google-gemini-3-and-open-source-frameworks/)

### Route Models by Task Complexity
```python
ROUTING = {
    "classify_intent": "gemini-3-flash",    # cheap, fast
    "plan_complex_task": "gemini-3-pro",     # deep reasoning
}
```

> Source: `agent-guides.md`

---

## Contradictions Found

### Data Placement vs. Constraint Placement
- **Internet research** says: "Negative constraints and formatting constraints should be placed at the END"
- **Internal checklist** (`prompt-guides.md`) says: "Put critical instructions first — role, constraints, output format at the top"
- **Resolution**: These are NOT contradictory when understood correctly. The rule is about **two different locations**:
  - **System instruction**: Role, core constraints, and output format go at the TOP (they are behavioral anchors)
  - **User prompt**: When mixing data + instructions, put data FIRST and negative/formatting constraints LAST
  - The internal framework template makes this clear: system instruction has role/constraints at top, user prompt has data first then task/constraints last

### Few-Shot Examples: How Mandatory?
- **Internet research** says: "We recommend to always include few-shot examples"
- **Internal guides** are more emphatic: "Prompts without examples are likely less effective" and "zero-shot explicitly worse"
- **Resolution**: No contradiction — internal guides are just stronger in wording. Treat few-shot examples as mandatory for Gemini.

---

## Key Differentiators (What Makes Gemini Unique)

1. **Temperature must stay at 1.0** on Gemini 3 — lowering causes loops and degraded reasoning
2. **Constraints go LAST** — negative/formatting constraints placed early get dropped
3. **Data-first ordering** — supply context first, instructions last (opposite of some providers)
4. **Persona priority** — personas can override conflicting instructions
5. **Default conciseness** — must explicitly request verbose/detailed responses
6. **9-area planning template** — official template improved agentic benchmarks by ~5%
7. **Thought signatures** — mandatory encrypted tokens for function calling on Gemini 3
8. **AUTO mode degrades** — tool use reliability drops in long conversations
9. **Explicit caching constraint** — system instructions, tools, and tool_config cannot be mixed with explicit cache
10. **VALIDATED mode** — stricter schema adherence than AUTO for tool calling

---

## All Sources

- [Gemini API — System Instructions](https://ai.google.dev/gemini-api/docs/system-instructions)
- [Gemini 3 Prompting Guide](https://ai.google.dev/gemini-api/docs/gemini-v3-prompting-guide)
- [Gemini API — Function Calling](https://ai.google.dev/gemini-api/docs/function-calling)
- [Gemini API — Agentic Behaviors](https://ai.google.dev/gemini-api/docs/agentic)
- [Gemini API — Context Caching](https://ai.google.dev/gemini-api/docs/caching)
- [Gemini API — Long Context](https://ai.google.dev/gemini-api/docs/long-context)
- [Gemini API — Live API](https://ai.google.dev/gemini-api/docs/live)
- [Google AI Forum Discussions](https://discuss.ai.google.dev/)
