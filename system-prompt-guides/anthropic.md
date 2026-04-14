# Anthropic (Claude) — System Prompt Guidelines for Agentic Tasks

## Foundational Principles

### Think of Claude as a New Employee
Anthropic says to think of Claude as "a brilliant but new employee who lacks context on your norms and workflows." Show your prompt to a colleague with minimal context — if they would be confused, Claude will be too.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Be Explicit, Not Vague
Claude responds well to clear, direct instructions. If you want "above and beyond" behavior, you must explicitly request it. Claude will not infer ambitions from vague prompts. Claude is trained for precise instruction following and takes instructions more literally than some competitors.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Explain the "Why"
Providing context or motivation behind instructions helps Claude generalize. Instead of "NEVER use ellipses", say "Your response will be read aloud by a text-to-speech engine, so never use ellipses since the text-to-speech engine will not know how to pronounce them." Claude generalizes from the explanation.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Tell Claude What TO Do, Not What NOT to Do
Instead of "Do not use markdown in your response", try "Your response should be composed of smoothly flowing prose paragraphs." This is a consistent theme across all Anthropic docs.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

---

## XML Tags — Claude's Distinctive Formatting

This is the single biggest differentiator from other providers. Claude is specifically trained to parse XML tags in prompts, and Anthropic recommends them pervasively.

### Key Principles
- Use XML tags to separate instructions, context, examples, and variable inputs: `<instructions>`, `<context>`, `<input>`
- There are no canonical "best" tag names — use descriptive names that make sense for the content they surround
- Use consistent tag names across prompts and refer to them when discussing content
- Nest tags for hierarchical content: `<documents><document index="1"><source>...</source><document_content>...</document_content></document></documents>`
- Wrap examples in `<example>` tags (multiple in `<examples>` tags) so Claude distinguishes them from instructions
- Use XML tags to control output format: "Write the prose sections in `<smoothly_flowing_prose_paragraphs>` tags"
- Combine XML with chain-of-thought: `<thinking>` and `<answer>` tags to separate reasoning from output

> Source: [Use XML tags to structure prompts](https://platform.claude.com/docs/en/docs/build-with-claude/prompt-engineering/use-xml-tags)

### Agentic-Specific XML Patterns Anthropic Uses in Their Own Prompts
- `<default_to_action>` for proactive tool use behavior
- `<do_not_act_before_instructions>` for conservative behavior
- `<use_parallel_tool_calls>` for parallel execution guidance
- `<investigate_before_answering>` for anti-hallucination
- `<avoid_excessive_markdown_and_bullet_points>` for output control
- `<frontend_aesthetics>` for design guidance

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

---

## System Prompt Structure for Agentic Use Cases

From Anthropic's context engineering guide, the recommended structure is:

### Organize into Distinct Sections Using XML Tags or Markdown Headers
1. `<background_information>` — role, identity, context
2. `<instructions>` — behavioral rules and constraints
3. Tool guidance — when and how to use each tool
4. Output descriptions — format expectations

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Find the "Right Altitude"
Avoid two extremes:
- **Overly brittle**: Hardcoded complex if-else logic creates fragility
- **Overly vague**: High-level guidance without concrete signals fails

The sweet spot is "specific enough to guide behavior effectively, yet flexible enough to provide the model with strong heuristics."

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Start Minimal, Then Add Based on Failure Modes
Do not over-engineer prompts upfront. Begin with minimal instructions, observe where Claude fails, then add targeted guidance for those specific failures.

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Document/Data Ordering
Place long documents at the TOP of prompts, queries at the BOTTOM. Anthropic reports up to 30% quality improvement with this ordering.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Prompt Style Influences Output Style
The formatting of your prompt directly influences Claude's response formatting. Remove markdown from your prompt to reduce markdown in output.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

---

## Tool Use — Claude-Specific Behaviors

### Opus 4.5/4.6 Are More Responsive to System Prompts
Prompts designed for older models that pushed tool use aggressively ("CRITICAL: You MUST use this tool") will cause overtriggering on newer models. Dial back to natural language: "Use this tool when..."

> Source: [Advanced tool use on Claude](https://www.anthropic.com/engineering/advanced-tool-use)

### Proactive Action vs. Conservative Action
By default, if you say "can you suggest some changes," Claude may only suggest rather than implement. Two prompt patterns control this:

For proactive agents:
```xml
<default_to_action>
By default, implement changes rather than only suggesting them. If the user's intent is unclear, infer the most useful likely action and proceed, using tools to discover any missing details instead of guessing.
</default_to_action>
```

For conservative agents:
```xml
<do_not_act_before_instructions>
Do not jump into implementation unless clearly instructed. Default to providing information and recommendations rather than taking action.
</do_not_act_before_instructions>
```

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Parallel Tool Calling
Claude excels at parallel tool execution natively. You can boost reliability to near 100% with explicit prompting about when to parallelize vs. sequentialize. The key instruction pattern: "If there are no dependencies between calls, make all independent calls in parallel. Never use placeholders or guess missing parameters."

> Source: [Advanced tool use on Claude](https://www.anthropic.com/engineering/advanced-tool-use)

### Tool Description Design
- Describe tools as you would to a new team member
- Use unambiguous parameter names (`user_id` not `user`)
- Document return formats clearly since Claude parses outputs
- Return human-interpretable fields, not cryptic UUIDs
- Prefer `search_contacts` over `list_contacts` to avoid context waste
- Consolidate related operations into single tools when possible
- Keep tool sets minimal — "If a human engineer can't definitively say which tool should be used in a given situation, an AI agent can't be expected to do better"

> Source: [Writing effective tools for AI agents](https://www.anthropic.com/engineering/writing-tools-for-agents)

### Strict Tool Use
Add `strict: true` to tool definitions to ensure Claude's tool calls always match your schema exactly.

> Source: [Advanced tool use on Claude](https://www.anthropic.com/engineering/advanced-tool-use)

---

## Thinking and Reasoning

### Adaptive Thinking (Claude 4.6)
Use `thinking: {type: "adaptive"}` instead of the older `budget_tokens` approach. Claude dynamically decides when and how much to think based on the effort parameter and query complexity. Internal evaluations show adaptive thinking reliably outperforms manual extended thinking.

For agentic workloads, Anthropic specifically recommends adaptive thinking with `high` effort for multi-step tool use, complex coding, and long-horizon agent loops.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Guide Interleaved Thinking
"After receiving tool results, carefully reflect on their quality and determine optimal next steps before proceeding."

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Prefer General Instructions Over Prescriptive Steps
"A prompt like 'think thoroughly' often produces better reasoning than a hand-written step-by-step plan. Claude's reasoning frequently exceeds what a human would prescribe."

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Self-Checking
"Before you finish, verify your answer against [test criteria]" catches errors reliably, especially for coding and math.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Quirk: "Think" Sensitivity
When extended thinking is disabled, Claude Opus 4.5 is particularly sensitive to the word "think." Use alternatives like "consider," "evaluate," or "reason through."

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Controlling Overthinking
Claude Opus 4.6 may think excessively, inflating tokens. Constrain with: "Choose an approach and commit to it. Avoid revisiting decisions unless you encounter new information that directly contradicts your reasoning."

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

---

## Agentic System Design — Official Patterns

### Start Simple
The most successful implementations use simple, composable patterns, not complex frameworks. "Start by using LLM APIs directly: many patterns can be implemented in a few lines of code."

> Source: [Building effective AI agents](https://www.anthropic.com/research/building-effective-agents)

### Workflows vs. Agents
- Workflows = LLMs orchestrated through predefined code paths (predictable)
- Agents = LLMs dynamically direct their own processes (flexible)
- Only use agents when you need open-ended problem solving with autonomous decisions

> Source: [Building effective AI agents](https://www.anthropic.com/research/building-effective-agents)

### Five Core Workflow Patterns
1. **Prompt chaining** — sequential steps, each processing prior output
2. **Routing** — classify input, direct to specialized handlers
3. **Parallelization** — run multiple calls simultaneously (sectioning or voting)
4. **Orchestrator-workers** — central LLM decomposes tasks, delegates to workers
5. **Evaluator-optimizer** — generate then critique in a loop

> Source: [Building effective AI agents](https://www.anthropic.com/research/building-effective-agents)

### The Practical Implementation Path
Single LLM call with retrieval -> prompt chaining -> routing -> parallelization -> orchestrator-workers -> full agents. Measure at each step before advancing.

> Source: [Building effective AI agents](https://www.anthropic.com/research/building-effective-agents)

---

## Long-Horizon Agent Techniques

### Context Is the Most Important Resource to Manage
LLM performance degrades as context fills. This is the single most critical constraint for agentic systems.

> Source: [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)

### Three Techniques for Context Management

**1. Compaction**
Summarize conversations nearing context limits. Preserve architectural decisions, unresolved bugs, and implementation details. Discard redundant tool outputs. "Tool result clearing is the safest, lightest form of compaction."

**2. Structured Note-Taking**
Agents write notes persisted outside the context window, retrieving them later as memory. Use JSON for structured state (test results, task status) and freeform text for progress notes.

**3. Sub-Agent Architectures**
Main agent coordinates high-level plan; sub-agents handle focused tasks in clean context windows, returning condensed summaries (1000-2000 tokens from tens of thousands of exploration tokens).

> Source: [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)

### Context Awareness Prompt Pattern
```
Your context window will be automatically compacted as it approaches its limit, allowing you to continue working indefinitely. Do not stop tasks early due to token budget concerns. Save your current progress and state to memory before the context window refreshes. Never artificially stop any task early.
```

> Source: [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)

### Multi-Window Workflow Recommendations
- Use the first context window to set up framework (write tests, create scripts)
- Have Claude write tests in structured format (e.g., `tests.json`) before starting work
- Create `init.sh` setup scripts to prevent repeated work across sessions
- Consider starting fresh rather than compacting — Claude is excellent at discovering state from the filesystem
- Use git for state tracking across sessions
- Provide verification tools (Playwright, browser automation) for autonomous correctness checking

> Source: [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)

---

## Known Quirks and Anti-Patterns

### Overeagerness / Overengineering
Claude Opus 4.5/4.6 tend to create extra files, add unnecessary abstractions, and build in unrequested flexibility. Counter with explicit guidance about minimal scope.

> Source: [Best practices for Claude Code](https://code.claude.com/docs/en/best-practices)

### Excessive Subagent Spawning
Claude Opus 4.6 has "a strong predilection for subagents" and may spawn them when a simple grep would suffice. Add explicit guidance about when subagents are warranted.

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Test-Focused Coding
Claude can fixate on making tests pass at the expense of general solutions, or hard-code values matching test inputs. Counter with: "Implement a solution that works correctly for all valid inputs, not just the test cases."

> Source: [Best practices for Claude Code](https://code.claude.com/docs/en/best-practices)

### Hallucination in Coding
Less prone in latest models, but counter with: "Never speculate about code you have not opened. Read the file before answering."

> Source: [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Prefilled Responses Deprecated
Starting with Claude 4.6, prefilled assistant turns are deprecated. Use structured outputs, XML tags, or direct instructions instead.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### LaTeX Default
Claude Opus 4.6 defaults to LaTeX for math. Must explicitly request plain text if you don't want it.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Autonomy Safety
Without guidance, Claude may take irreversible actions (deleting files, force-pushing, posting to external services). Always include reversibility guidelines.

> Source: [Best practices for Claude Code](https://code.claude.com/docs/en/best-practices)

### Over-Specified Instructions
If your instructions are too long, Claude ignores half of them. Keep instructions concise and relevant.

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

---

## Balancing Autonomy and Safety

Anthropic provides this specific prompt pattern for agentic safety:
```
Consider the reversibility and potential impact of your actions. Take local, reversible actions freely (editing files, running tests), but for hard-to-reverse or shared-system actions, ask before proceeding.

Examples warranting confirmation:
- Destructive: deleting files, dropping tables, rm -rf
- Hard to reverse: git push --force, git reset --hard
- Visible to others: pushing code, commenting on PRs, sending messages
```

> Source: [Best practices for Claude Code](https://code.claude.com/docs/en/best-practices)

---

## Complete System Prompt Template

The recommended XML template structure from internal framework docs and Anthropic's patterns:

```xml
<role>
You are a [specific role] with expertise in [domain].
You are [personality traits relevant to the task: precise, thorough, concise, creative].
[One sentence on what success looks like for this role.]
</role>

<context>
[Why this task matters. What the user is trying to achieve. What will happen
with the output. This helps Claude calibrate tone, depth, and format.]
[Any background information, domain knowledge, or constraints.]
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
- If you lack sufficient information to complete the task, say so explicitly
  and list what additional information you need.
- [Domain-specific constraint]
- [Quality bar: "Ensure all code compiles", "Verify all citations exist", etc.]
</constraints>

<!-- AGENTIC ADDITIONS (only if using tools) -->

<tool_behavior>
- Use [tool_name] when [specific condition].
- Do NOT use [tool_name] for [specific anti-pattern].
- When multiple tools could apply, prefer [tool_name] because [reason].

[Parallel tool calling:]
If you need to call multiple tools with no dependencies between them,
call all independent tools in parallel. If a call depends on a previous
result, call it sequentially. Never guess missing parameters.
</tool_behavior>

<action_mode>
[Choose ONE based on use case:]

<!-- For agents that should ACT by default: -->
By default, implement changes rather than only suggesting them.
If the user's intent is unclear, infer the most useful likely action
and proceed, using tools to discover any missing details instead of guessing.

<!-- For agents that should ADVISE by default: -->
Do not jump into implementation unless clearly instructed.
Default to providing information, analysis, and recommendations.
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

> Source: Internal framework document (`prompt-framework.md`), synthesized from [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

---

## Additional Techniques from Internal Guides

### Use XML Format Indicators to Steer Output Formatting
Wrapping expected output in XML tags effectively controls formatting:
```
"Write your analysis in <flowing_prose> tags. Do not use bullet
points or numbered lists inside these tags."
```

> Source: Internal checklist (`prompt-guides.md`), derived from [Prompting best practices](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Prefill Assistant Response to Lock Format (Pre-4.6 Only)
Start the assistant's response to eliminate "how should I begin" ambiguity:
```python
messages = [
    {"role": "user", "content": "Classify this ticket: '{text}'"},
    {"role": "assistant", "content": '{"category": "'}
]
```

**CONTRADICTION NOTE**: This technique is documented in `prompt-framework.md` and `prompt-guides.md` as a recommended Claude trick. However, our internet research found that prefilled assistant turns are **deprecated starting with Claude 4.6**. Use prefill for Claude 4.x models prior to 4.6; for 4.6+, use structured outputs or XML tags instead.

> Source: `prompt-framework.md`, `prompt-guides.md`; deprecation from [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)

### Explicitly Request Summaries After Tool Use
Claude 4.5 may skip verbal summaries after tool calls, jumping directly to the next action. If you want the agent to narrate its work:
```
"After completing a task that involves tool use, provide a quick
summary of the work you've done."
```

> Source: Internal checklist (`prompt-guides.md`)

### Prevent Hard-Coding in Solutions
Claude can focus on passing tests instead of implementing general solutions:
```
"Implement a solution that works for all valid inputs, not just
test cases. Do not hard-code values. If tests are incorrect,
inform me rather than working around them."
```

> Source: Internal checklist (`prompt-guides.md`)

### Fight "AI Slop" in Frontend Design
Without guidance, Claude defaults to generic Inter fonts, purple gradients, and predictable layouts:
```xml
<frontend_aesthetics>
Avoid: Inter, Roboto, Arial. No purple gradients on white.
Instead: Distinctive typography, cohesive color themes with sharp
accents, CSS animations for micro-interactions, atmospheric
backgrounds with layered gradients.
</frontend_aesthetics>
```

> Source: `prompt-framework.md`, `prompt-guides.md`

### Use Git as Primary State Tracking Mechanism
```
"After each logical unit of work, commit with a descriptive message.
When starting a new session, run git log --oneline -20 and read
progress.txt to restore context."
```

> Source: `prompt-framework.md`, `prompt-guides.md`

### Subagent Orchestration Happens Naturally
Claude 4.5 proactively delegates to subagents without explicit instruction. Just make tools available and well-described. Only add constraints if it's overdoing it.

> Source: `prompt-framework.md`

### Use Diverse Canonical Examples Instead of Rule Walls
3 well-chosen examples teach behavior more reliably than 15 rules. Examples define format, tone, length, and reasoning depth simultaneously. Include edge cases in your examples, not as separate rules.

> Source: `anthropic-agent-guide.txt`, [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Compaction Implementation Pattern
```python
# In your agent loop, after every N turns:
for msg in message_history[:-10]:  # keep last 10 intact
    if msg.role == "tool_result":
        msg.content = f"[Result from {msg.tool_name} — already processed]"
```

Tune compaction for recall first, then precision:
```
"When summarizing, ERR ON THE SIDE OF KEEPING TOO MUCH.
Include: decisions made and why, files touched, errors encountered,
constraints discovered, things still to do.
Only exclude: raw file contents already saved to disk,
duplicate tool calls that returned the same result."
```

> Source: `anthropic-agent-guide.txt`, `agent-guides.md`, [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

### Match Context Strategy to Task Type
```python
if task.type == "conversation":       # back-and-forth
    strategy = "compaction"
elif task.type == "multi_step_build":  # coding, migrations
    strategy = "note_taking"
elif task.type == "research":          # parallel exploration
    strategy = "sub_agents"
```

> Source: `anthropic-agent-guide.txt`, `agent-guides.md`

### Hybrid Retrieval — Preload Essentials, Explore the Rest
```xml
<preloaded_context>
{contents of project README}
{contents of .env.example}
{project directory tree, 2 levels deep}
</preloaded_context>

<instructions>
The above gives you project orientation. For everything else,
use read_file() and search_codebase() to find what you need
at the moment you need it. Do not ask for more files upfront.
</instructions>
```

> Source: `anthropic-agent-guide.txt`, `agent-guides.md`

### Research Tasks: Structured Hypothesis Tracking
```
"Search in a structured way. Develop competing hypotheses.
Track confidence levels. Regularly self-critique. Update a
hypothesis tree or research notes file."
```

> Source: Internal checklist (`prompt-guides.md`)

### Vision Tasks: Give Claude a Crop Tool
Testing shows consistent uplift when Claude can "zoom" in on image regions:
```python
@tool
def crop_image(image_path: str, x: int, y: int, w: int, h: int):
    """Crop a region to examine details more closely."""
    return Image.open(image_path).crop((x, y, x+w, y+h))
```

> Source: Internal checklist (`prompt-guides.md`)

---

## Contradictions Found

### Claude 4.x Verbosity
- **Internet research** describes Claude as tending "toward thorough explanations by default"
- **Internal framework** (`prompt-framework.md`) says: "Claude 4.x is naturally concise. If you want detailed output, say: 'Provide a comprehensive, detailed response.'"
- **Resolution**: The conciseness observation applies specifically to Claude 4.x (Opus 4.5/4.6). Earlier Claude models (3.x) were more verbose. For system prompts targeting current models, treat Claude as concise by default and explicitly request verbosity when needed.

### Prefill Assistant Turns
- **Internal guides** (`prompt-framework.md`, `prompt-guides.md`) recommend prefill as a key technique
- **Internet research** says prefilled responses are deprecated starting with Claude 4.6
- **Resolution**: Version-dependent. Use prefill for Claude 4.0-4.5. For Claude 4.6+, use structured outputs or XML format indicators instead.

---

## Key Differentiators (What Makes Claude Unique)

1. **XML tags are first-class**: No other major model has this level of XML tag support baked into training and documentation
2. **Explain the "why" behind instructions**: Claude is specifically designed to generalize from motivational context
3. **System prompt has elevated authority**: Claude treats system prompt instructions with high fidelity. Opus 4.5/4.6 are "more responsive to the system prompt than previous models"
4. **Positive framing over negative**: "Tell Claude what to do, not what not to do" is a core principle
5. **Context engineering over prompt engineering**: Anthropic explicitly frames the discipline as "context engineering" — curating the smallest set of high-signal tokens
6. **Adaptive thinking is unique**: The effort parameter and adaptive thinking system where Claude dynamically decides how much to reason
7. **Native parallel tool calling**: Claude's latest models excel at this without special prompting
8. **Context awareness**: Claude 4.5/4.6 can track their remaining context window during a conversation

> Source: [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices), [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)

---

## All Sources

- [Prompting best practices (Claude 4 models)](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices)
- [Effective context engineering for AI agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Building effective AI agents](https://www.anthropic.com/research/building-effective-agents)
- [Writing effective tools for AI agents](https://www.anthropic.com/engineering/writing-tools-for-agents)
- [Advanced tool use on Claude](https://www.anthropic.com/engineering/advanced-tool-use)
- [Best practices for Claude Code](https://code.claude.com/docs/en/best-practices)
- [Effective harnesses for long-running agents](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents)
- [Prompt engineering overview](https://platform.claude.com/docs/en/docs/build-with-claude/prompt-engineering/overview)
- [Use XML tags to structure prompts](https://platform.claude.com/docs/en/docs/build-with-claude/prompt-engineering/use-xml-tags)
- [System prompts documentation](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/system-prompts)
- [Anthropic Cookbook (GitHub)](https://github.com/anthropics/anthropic-cookbook)
- [Interactive Prompt Engineering Tutorial (GitHub)](https://github.com/anthropics/prompt-eng-interactive-tutorial)
