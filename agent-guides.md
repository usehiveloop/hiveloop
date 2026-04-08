# The Complete Context Engineering & Agent Building Checklist

## A Unified Reference from Anthropic, OpenAI, and Google's Official Guides

*Compiled for book research. Sources: Anthropic's "Effective Context Engineering for AI Agents" (Sep 2025), OpenAI's "A Practical Guide to Building Agents" (2025) + OpenAI Cookbook "Context Engineering - Short-Term Memory Management with Sessions" (Sep 2025), Google's "Introduction to Agents" Whitepaper (Nov 2025) + "Building AI Agents with Gemini 3" (Nov 2025).*

---

# Part 1: Anthropic — Context Engineering for AI Agents

Source: https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents

---

## 1. Minimize context, maximize signal

**Why:** As context grows, model recall degrades (context rot). Every token competes for a finite attention budget — n² pairwise relationships means bloat kills precision.

**Practical Example:**
Instead of dumping an entire 500-line API response into context, parse it first and only pass the 3-4 fields the agent actually needs. In your tool implementation: `return { id, status, error }` not `return entireAPIResponse`.

---

## 2. Write system prompts at the "right altitude"

**Why:** Too rigid (hardcoded if-else logic) creates brittleness. Too vague assumes shared context the model doesn't have. You want strong heuristics, not scripts.

**Practical Example:**

```
Bad (too rigid):
"If the user says 'deploy', run deploy.sh. If the user says 'test', run test.sh. If the user says 'build'..."

Bad (too vague):
"Help the user with their code."

Good:
"You are a deployment assistant. When the user wants to ship code, verify tests pass before deploying. If tests fail, diagnose the failure and suggest fixes before retrying. Never deploy with failing tests."
```

---

## 3. Structure prompts with clear sections (XML tags, markdown headers)

**Why:** Delineation helps the model parse intent per section instead of treating your prompt as one undifferentiated blob.

**Practical Example:**

```xml
<role>You are a senior code reviewer.</role>
<rules>
- Flag security vulnerabilities as blocking.
- Flag style issues as non-blocking.
</rules>
<output_format>
Return a JSON array of findings: {file, line, severity, message}
</output_format>
```

---

## 4. Start with a minimal prompt on the best model, then iterate based on failure modes

**Why:** You avoid over-engineering upfront. Real failures tell you exactly what instructions to add — nothing else does.

**Practical Example:**

```
Start with:
"You are a customer support agent. Answer questions using the knowledge base tool."

Then after testing you notice it hallucinates when the KB has no answer, so you add:
"If the knowledge base returns no relevant results, say you don't know and escalate to a human. Never guess."
```

---

## 5. Build self-contained, non-overlapping tools with descriptive parameters

**Why:** If a human engineer can't definitively say which tool should be used in a given situation, the agent can't either. Ambiguous tool sets cause wrong tool selection and wasted turns.

**Practical Example:**

```
Bad:
search_data(query) and find_info(query) — what's the difference?

Good:
search_customers(email: string) and search_orders(order_id: string)
— each tool has one job, clear input, zero ambiguity.
```

---

## 6. Make tools return token-efficient responses

**Why:** Bloated tool outputs eat your attention budget on data the agent may not even need, pushing out the signal that matters.

**Practical Example:**

```python
# Bad tool response: returns the full customer object with 40 fields including internal metadata.

# Good: your tool wrapper strips it down:
def search_customer(email):
    raw = api.get_customer(email)
    return {"name": raw.name, "plan": raw.plan, "status": raw.status}
```

---

## 7. Use diverse canonical examples instead of rule walls

**Why:** Examples are the "pictures" worth a thousand words for an LLM. A curated set of examples teaches behavior more reliably than a wall of rules.

**Practical Example:**

```xml
<examples>
User: "This is broken, fix it now!"
Assistant: "I understand the urgency. Let me look into this right away. Can you share the error message you're seeing?"

User: "Hey, quick question about billing"
Assistant: "Sure! What's your billing question?"

User: "I want to cancel."
Assistant: "I'm sorry to hear that. Before I process the cancellation, can I ask what's prompting the change?"
</examples>
```

---

## 8. Use just-in-time context retrieval (store references, load data on demand)

**Why:** Rather than pre-loading everything, agents can maintain lightweight identifiers and dynamically load data at runtime. This keeps the context window clean and avoids stale or irrelevant data.

**Practical Example:**

```
You have access to these tools:
- read_file(path): reads a file's contents
- list_directory(path): lists files in a directory
- search_codebase(query): greps across the repo

Do NOT ask for all files upfront. Navigate the codebase as needed.
```

---

## 9. Let agents progressively discover context through exploration

**Why:** Folder hierarchies, naming conventions, timestamps — metadata provides free signals. Each interaction yields context that informs the next decision, building understanding layer by layer.

**Practical Example:**

```
When starting a task:
1. First run list_directory(".") to understand project structure.
2. Read README.md or config files to understand conventions.
3. Then navigate to the relevant files based on what you learned.
Do not assume file locations. Explore first.
```

---

## 10. Implement compaction (summarize and restart context when it gets long)

**Why:** Compaction distills context in a high-fidelity manner, enabling the agent to continue with minimal performance degradation. Without it, long tasks drown in accumulated noise.

**Practical Example:**

```
If message_history token count > 80% of context window:
    Call the model with: "Summarize this conversation so far.
    Preserve: all architectural decisions, unresolved bugs,
    file paths modified, current task status.
    Discard: redundant tool outputs, exploratory dead ends."
    Replace message_history with summary + last 5 messages.
```

---

## 11. Tune compaction for recall first, then precision

**Why:** Aggressive compaction loses subtle details whose importance only shows up later. Better to keep too much initially, then trim.

**Practical Example:**

```
"When summarizing, ERR ON THE SIDE OF KEEPING TOO MUCH.
Include: decisions made and why, files touched, errors encountered,
constraints discovered, things still to do.
Only exclude: raw file contents already saved to disk,
duplicate tool calls that returned the same result."
```

---

## 12. Clear old tool call results from message history

**Why:** Once a tool has been called deep in the message history, the agent doesn't need the raw result again. It's the safest, lightest form of compaction.

**Practical Example:**

```python
# In your agent loop, after every N turns:
for msg in message_history[:-10]:  # keep last 10 intact
    if msg.role == "tool_result":
        msg.content = f"[Result from {msg.tool_name} — already processed]"
```

---

## 13. Implement structured note-taking (external persistent memory)

**Why:** Notes persisted outside the context window get pulled back in when needed. This lets agents track progress, dependencies, and milestones across tasks that outlive any single context window.

**Practical Example:**

```
You have a scratchpad tool: write_notes(content) and read_notes().
After completing each subtask, write a progress note:
- What you just did
- What you learned
- What's left to do
Before starting work, always read_notes() to restore context.
```

---

## 14. Use sub-agent architectures for complex tasks

**Why:** Each sub-agent explores extensively but returns only a condensed summary (often 1,000–2,000 tokens). The lead agent keeps a clean context focused on synthesis, not raw exploration data.

**Practical Example:**

```python
# Lead agent prompt
"You are a research coordinator. Break the user's question into
sub-questions. For each, dispatch a research_agent(question) call.
Each sub-agent will return a 200-word summary. Synthesize all
summaries into a final answer. Never do deep research yourself."

# Sub-agent prompt
"Answer this specific question thoroughly. Return ONLY a concise
summary of your findings in under 200 words."
```

---

## 15. Match your context strategy to the task type

**Why:** Compaction suits back-and-forth conversational tasks, note-taking suits iterative work with milestones, and multi-agent handles parallel exploration. Wrong strategy = wasted tokens or lost coherence.

**Practical Example:**

```python
if task.type == "conversation":       # back-and-forth
    strategy = "compaction"
elif task.type == "multi_step_build":  # coding, migrations
    strategy = "note_taking"
elif task.type == "research":          # parallel exploration
    strategy = "sub_agents"
```

---

## 16. Consider a hybrid retrieval strategy (some data upfront + autonomous exploration)

**Why:** Pre-loaded context gives speed, autonomous exploration gives freshness and adaptability. Claude Code does this — CLAUDE.md files load upfront while grep/glob handle just-in-time retrieval.

**Practical Example:**

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

---

## 17. Default to the simplest thing that works

**Why:** As model capabilities improve, smarter models require less prescriptive engineering. Over-engineering today becomes technical debt tomorrow when the model can handle it natively.

**Practical Example:**

```
Before adding complexity, ask: does the model already handle this
without extra instructions? Start with:

"You are a coding assistant. Help the user with their codebase."

Only add guardrails, tools, and structure when you observe specific
failures. Every line in your prompt should trace back to a real
failure you saw in testing — if it doesn't, delete it.
```

---
---

# Part 2: OpenAI — Practical Agent Building & Context Management

Sources:
- https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf
- https://cookbook.openai.com/examples/agents_sdk/session_memory

---

## 1. Prototype with the best model, then downgrade where possible

**Why:** Establishes a performance ceiling so you know exactly where smaller models break.

**Practical Example:**

```python
# Start all tasks on gpt-5
# After evals pass, swap classification to gpt-4o-mini
triage_agent = Agent(model="gpt-4o-mini", ...)  # simple routing
refund_agent = Agent(model="gpt-5", ...)         # complex judgment
```

---

## 2. Set up evals before anything else

**Why:** Without a baseline, you can't tell if your changes help or hurt.

**Practical Example:**

```python
# Define eval cases as input/expected-output pairs
evals = [
    {"input": "I want a refund for order #123", "expected_tool": "lookup_order"},
    {"input": "What's your return policy?", "expected_tool": "search_kb"},
]
# Run agent on each, score tool selection accuracy
```

---

## 3. Give each tool a single job with a clear name, description, and typed parameters

**Why:** If improving tool clarity by providing descriptive names, clear parameters, and detailed descriptions doesn't improve performance, then split into multiple agents.

**Practical Example:**

```python
@function_tool
def lookup_order(order_id: str) -> dict:
    """Retrieve order status, items, and shipping info by order ID."""
    return db.orders.find(order_id)

# NOT: get_data(query: str) — ambiguous, no typed params
```

---

## 4. Categorize tools into Data, Action, and Orchestration types

**Why:** Forces you to separate read-only context gathering from write operations and agent delegation — different risk profiles need different guardrails.

**Practical Example:**

```python
# Data tool (read-only, low risk)
@function_tool
def search_kb(query: str): ...

# Action tool (write, medium risk — needs confirmation)
@function_tool
def issue_refund(order_id: str, amount: float): ...

# Orchestration tool (agent-as-tool)
research_tool = research_agent.as_tool(
    tool_name="deep_research",
    tool_description="Performs multi-source research on a topic"
)
```

---

## 5. Convert existing docs/SOPs into agent instructions instead of writing from scratch

**Why:** Using existing operating procedures, support scripts, or policy documents helps create LLM-friendly routines. In customer service, routines can roughly map to individual articles in your knowledge base.

**Practical Example:**

```
prompt = f"""
You are an expert in writing instructions for an LLM agent.
Convert the following help center document into a clear set of
numbered instructions for an agent to follow.
Ensure there is no ambiguity.

Document: {help_center_doc}
"""
# Use o3/gpt-5 to auto-generate your agent instructions
```

---

## 6. Define clear actions in every instruction step — no vague guidance

**Why:** Every step in your routine should correspond to a specific action or output. Being explicit about the action leaves less room for errors in interpretation.

**Practical Example:**

```
Bad:  "Handle the refund appropriately."

Good: "1. Call lookup_order(order_id) to get order details.
       2. If order is within 30-day window AND status is 'delivered',
          call issue_refund(order_id, amount).
       3. If outside window, tell the user: 'Unfortunately, this order
          is past our 30-day return policy.'"
```

---

## 7. Anticipate edge cases with conditional branches in your instructions

**Why:** Real-world interactions create decision points like how to proceed when a user provides incomplete information. A robust routine anticipates common variations with conditional steps.

**Practical Example:**

```
"If the user doesn't provide an order ID:
   → Ask: 'Could you share your order number? It starts with ORD-.'
If the user provides an email instead:
   → Call lookup_orders_by_email(email) and present the list.
If multiple orders match:
   → List them and ask which one they need help with."
```

---

## 8. Use prompt templates with policy variables instead of maintaining separate prompts per use case

**Why:** A single flexible base prompt that accepts policy variables adapts easily to various contexts, significantly simplifying maintenance and evaluation.

**Practical Example:**

```python
TEMPLATE = """
You are a call center agent for {{company_name}}.
The customer {{user_first_name}} has been a member for {{user_tenure}}.
Their complaint history: {{user_complaint_categories}}.
Refund policy: {{refund_policy}}
Greet them, thank them for their loyalty, and resolve their issue.
"""
# Swap variables per customer, per region, per product line
```

---

## 9. Maximize single-agent capabilities before going multi-agent

**Why:** More agents can provide intuitive separation of concepts but introduce additional complexity and overhead. Often a single agent with tools is sufficient.

**Practical Example:**

```python
# Start here — one agent, multiple tools
support_agent = Agent(
    name="Support",
    tools=[search_kb, lookup_order, issue_refund, escalate_to_human],
    instructions="..."
)

# Only split when: instructions have too many if-else branches,
# or tools overlap and the agent picks wrong ones consistently
```

---

## 10. When you do go multi-agent, pick the right pattern: Manager vs. Decentralized handoffs

**Why:** Manager keeps one agent in control of the user experience (good for synthesis). Decentralized handoffs let specialized agents fully take over (good for triage/routing).

**Practical Example — Manager:**

```python
manager = Agent(
    tools=[
        billing_agent.as_tool(tool_name="billing", ...),
        technical_agent.as_tool(tool_name="technical", ...),
    ]
)
# Manager calls sub-agents, synthesizes their answers, talks to user
```

**Practical Example — Decentralized:**

```python
triage_agent = Agent(
    handoffs=[billing_agent, technical_agent, sales_agent]
)
# Triage identifies intent, hands off entirely.
# User now talks directly to the specialist.
```

---

## 11. Implement context trimming — keep last N turns, drop the rest

**Why:** If too much is carried forward, the model risks distraction, inefficiency, or outright failure. Trimming keeps the agent anchored to the latest user goal.

**Practical Example:**

```python
session = TrimmingSession("session_123", max_turns=8)
result = await Runner.run(agent, user_message, session=session)
# Automatically drops everything before the 8th-most-recent user turn
```

**How it works:**
- A "turn" = one user message + everything after it (assistant replies, tool calls) until the next user message.
- On write: `add_items(...)` appends then trims.
- On read: `get_items(...)` returns trimmed view.
- Walk backward, find the Nth user message, keep everything from that index to the end, drop everything before it.

---

## 12. Implement context summarization for long-horizon tasks where old context still matters

**Why:** Summaries act as "clean rooms" that correct or omit prior mistakes. Trimming alone causes amnesia — the agent forgets promises, constraints, and IDs.

**Practical Example:**

```python
session = SummarizingSession(
    keep_last_n_turns=3,   # keep 3 turns verbatim
    context_limit=6,       # summarize when >6 turns
    summarizer=LLMSummarizer(client, model="gpt-4o")
)
# Old turns become a synthetic summary pair at the top of history:
# user: "Summarize the conversation we had so far."
# assistant: "{generated summary}"
```

---

## 13. Design your summarization prompt for the specific use case — not generic compression

**Why:** A well-crafted summarization prompt should be tailored to the specific use case. Think of it like being a support agent handing off a case to the next agent — what details would they need?

**Practical Example:**

```
SUMMARY_PROMPT = """
Compress earlier conversation into a snapshot with these sections:
• Product & Environment (device, OS, versions)
• Reported Issue (single sentence, latest state)
• Steps Tried & Results (chronological bullets)
• Identifiers (ticket #, serial, account — only if provided)
• Timeline Milestones (key events with timestamps or relative order)
• Tool Performance Insights (what tool calls worked/failed and why)
• Current Status & Blockers
• Next Recommended Step

Rules: ≤200 words. No invented facts. Quote error codes exactly.
Mark uncertain facts as UNVERIFIED.
"""
```

---

## 14. Add hallucination control to your summarization prompt

**Why:** Even minor hallucinations in a summary can propagate forward, contaminating future context with inaccuracies (context poisoning).

**Practical Example:**

```
"Before writing the summary (do this silently):
 - Contradiction check: compare user claims with tool logs. Note conflicts.
 - Temporal ordering: sort key events by time; most recent update wins.
 - If any fact is uncertain or not explicitly stated, mark it UNVERIFIED.
 - Do not invent new facts. Quote error strings/codes exactly."
```

---

## 15. Layer multiple guardrails — no single one is enough

**Why:** A single guardrail is unlikely to provide sufficient protection. Using multiple specialized guardrails together creates more resilient agents.

**Practical Example:**

```python
agent = Agent(
    input_guardrails=[
        Guardrail(guardrail_function=jailbreak_detector),
        Guardrail(guardrail_function=pii_filter),
        Guardrail(guardrail_function=relevance_classifier),
    ],
    tools=[...],
)
# Also add: regex blocklist, input length limits, output validation
```

**Types of guardrails to consider:**
- Relevance classifier (flags off-topic queries)
- Safety classifier (detects jailbreaks/prompt injections)
- PII filter (prevents exposure of personally identifiable information)
- Moderation (flags hate speech, harassment, violence)
- Tool safeguards (risk-rate each tool: low/medium/high)
- Rules-based protections (blocklists, input length limits, regex filters)
- Output validation (ensures responses align with brand values)

---

## 16. Assign risk ratings to tools and gate high-risk ones with human approval

**Why:** Assess the risk of each tool — read-only vs. write access, reversibility, financial impact. Use ratings to trigger automated pauses or escalation to a human.

**Practical Example:**

```python
TOOL_RISK = {
    "search_kb": "low",        # read-only
    "update_address": "medium", # write, reversible
    "issue_refund": "high",     # financial, irreversible
}

# In your agent loop:
if TOOL_RISK[tool_name] == "high":
    confirmation = await get_human_approval(tool_name, args)
    if not confirmation:
        return "Action requires manager approval."
```

---

## 17. Build human escalation into the agent loop with explicit triggers

**Why:** If the agent exceeds retry limits or faces high-stakes irreversible actions, it should gracefully transfer control rather than keep looping.

**Practical Example:**

```python
MAX_RETRIES = 3

# In your agent instructions:
"If you fail to understand the customer's intent after 3 attempts,
 say: 'Let me connect you with a specialist who can help.'
 Then call escalate_to_human(reason='intent_unclear', transcript=...).

 If the action involves canceling an account or refunding >$500,
 always call escalate_to_human before proceeding."
```

---

## 18. Run evals on your context management — not just your prompts

**Why:** The key question is: how do we know the model isn't losing context or confusing context? Replay long conversations and measure next-turn accuracy with and without trimming.

**Practical Example:**

```python
# Transcript replay eval
for convo in test_conversations:
    # Run with full history
    full_result = await run_agent(convo, trimming=False)
    # Run with trimming
    trimmed_result = await run_agent(convo, trimming=True)
    # Compare: did trimming drop any critical entity/ID?
    score = compare_entity_recall(full_result, trimmed_result)
    log_eval(convo.id, score)
```

**Eval strategies to consider:**
- Baseline & Deltas: Compare before/after experiments
- LLM-as-Judge: Use a model to evaluate summarization quality
- Transcript Replay: Re-run long conversations, measure next-turn accuracy
- Error Regression Tracking: Watch for dropped constraints or repeated tool calls
- Token Pressure Checks: Flag cases where token limits force dropping critical context

---

## 19. Choose trimming vs. summarization based on task characteristics

**Why:** Each has distinct tradeoffs. Picking wrong wastes tokens or loses coherence.

**Comparison Table:**

| Dimension | Trimming (last-N turns) | Summarizing (compress older turns) |
|---|---|---|
| Latency / Cost | Lowest (no extra calls) | Higher at summary refresh points |
| Long-range recall | Weak (hard cut-off) | Strong (compact carry-forward) |
| Risk type | Context loss | Context distortion/poisoning |
| Observability | Simple logs | Must log summary prompts/outputs |
| Eval stability | High | Needs robust summary evals |
| Best for | Tool-heavy ops, short workflows | Analyst/concierge, long threads |

**Decision rule:**
- Tasks are independent with non-overlapping context → **Trimming**
- Tasks need context collected across the flow (planning, coaching, RAG-heavy analysis) → **Summarization**

---
---

# Part 3: Google — Agent Architecture & Gemini-Specific Strategies

Sources:
- Google "Introduction to Agents" Whitepaper (Nov 2025)
- https://developers.googleblog.com/building-ai-agents-with-google-gemini-3-and-open-source-frameworks/
- https://vanducng.dev/2026/01/10/Google-Introduction-to-Agents-Whitepaper-Summary/

---

## 1. Build on three core components: Model + Tools + Orchestration layer

**Why:** In its most fundamental form, an agent can be defined as an application that attempts to achieve a goal by observing the world and acting upon it using the tools at its disposal. The combination of these components can be described as a cognitive architecture.

**Practical Example:**

```python
from google.adk import Agent

agent = Agent(
    model="gemini-3-pro",              # Model
    tools=[search_kb, create_ticket],   # Tools
    instructions="...",                  # Orchestration guidance
)
```

---

## 2. Choose a reasoning framework for your orchestration layer: ReAct, CoT, or ToT

**Why:** Various reasoning techniques such as ReAct, Chain-of-Thought, and Tree-of-Thoughts provide a framework for the orchestration layer to take in information, perform internal reasoning, and generate informed decisions.

**Framework Selection Guide:**
- **ReAct** — Best for tool-using agents. Interleaves reasoning with action. Use when the agent needs to decide which tools to call and in what order.
- **Chain-of-Thought (CoT)** — Best for complex reasoning without tools. Breaks problems into intermediate steps.
- **Tree-of-Thoughts (ToT)** — Best for exploration or strategic lookahead tasks. Explores multiple reasoning paths simultaneously.

**Practical Example — ReAct in your system prompt:**

```
"For each user request, follow this loop:
 1. Thought: reason about what you need to do next
 2. Action: choose a tool and its inputs
 3. Observation: read the tool result
 4. Repeat until you have enough info, then give Final Answer.

Never guess when a tool can give you the real answer."
```

---

## 3. Use `thinking_level` to control reasoning depth per request — don't over-think cheap tasks

**Why (Gemini-specific):** Adjust the logic depth on a per-request basis. Set it to high for deep planning, bug finding, and complex instruction following. Set it to low for high-throughput tasks to achieve latency comparable to Gemini 2.5 Flash with superior output quality.

**Practical Example:**

```python
# Triage step — fast, cheap
triage_response = model.generate(
    prompt=user_message,
    thinking_level="low"   # just classify intent
)

# Complex refund decision — deep reasoning
refund_response = model.generate(
    prompt=refund_context,
    thinking_level="high"  # weigh policy, edge cases
)
```

---

## 4. Pass Thought Signatures back in conversation history for stateful multi-step tool use

**Why (Gemini-specific):** The model generates encrypted "Thought Signatures" representing its internal reasoning before calling a tool. By passing these signatures back, your agent retains its exact train of thought, ensuring reliable multi-step execution without losing context.

**Practical Example:**

```python
response = model.generate(prompt=step_1, tools=tools)

# Extract thought signature from response
thought_sig = response.thought_signature

# Pass it back on the next turn
response_2 = model.generate(
    prompt=step_2,
    tools=tools,
    previous_thought_signature=thought_sig  # maintains reasoning chain
)
```

**Note:** Kimi K2.5 has a similar requirement — you must keep `reasoning_content` from assistant messages in context during multi-step tool calling, otherwise an error is thrown.

---

## 5. Use `media_resolution` to control multimodal token costs

**Why (Gemini-specific):** Balance token usage and detail with media_resolution. Use high for analyzing fine text in images, medium for optimal PDF document parsing, and low to minimize latency for video and general image descriptions.

**Practical Example:**

```python
# Parsing a contract PDF — need every detail
response = model.generate(
    content=[pdf_document, "Extract all clauses"],
    media_resolution="high"
)

# Quick video summary — save tokens
response = model.generate(
    content=[video_file, "What happens in this video?"],
    media_resolution="low"
)
```

---

## 6. Categorize your tools: Extensions (agent-side), Functions (client-side), Data Stores (RAG)

**Why:** Extensions provide a bridge between agents and external APIs enabling execution of API calls. Functions provide more nuanced control through division of labor, allowing agents to generate parameters which can be executed client-side. Data Stores provide access to structured or unstructured data.

**Practical Example:**

```python
# Extension — agent calls the API directly
flight_extension = Extension(
    name="google_flights",
    api_endpoint="https://flights.googleapis.com/...",
    examples=[{"query": "flights from Lagos to Dubai", "params": {...}}]
)

# Function — agent generates params, YOU call the API
@function_tool
def issue_refund(order_id: str, amount: float):
    """Agent suggests params, client executes the actual payment API call."""
    return {"order_id": order_id, "amount": amount}

# Data Store — RAG over your docs
data_store = DataStore(
    source="gs://my-bucket/knowledge-base/",
    embedding_model="text-embedding-004"
)
```

**Comparison Table:**

| | Extensions | Function Calling | Data Stores |
|---|---|---|---|
| Execution | Agent-side | Client-side | Agent-side |
| Best for | Agent controls API interactions, multi-hop planning | Security/auth restrictions, human-in-the-loop, batch ops | RAG with structured/unstructured data |
| Control | Agent has full control | Developer has full control | Agent queries vector DB |

---

## 7. Use Functions (client-side execution) when you need security, auth control, or human-in-the-loop

**Why:** Security or authentication restrictions prevent the agent from calling an API directly. Timing constraints or order-of-operations constraints prevent the agent from making API calls in real-time — batch operations, human-in-the-loop review, etc.

**When to use Functions over Extensions:**
- API calls need to be made at another layer of the application stack (middleware, frontend)
- Security or authentication restrictions prevent the agent from calling an API directly
- Timing or order-of-operations constraints (batch operations, human-in-the-loop review)
- Additional data transformation logic needed on API responses
- Developer wants to iterate without deploying API infrastructure (stubbing)

**Practical Example:**

```python
# Agent generates the params
result = model.generate(
    prompt="Transfer $5000 to account ending 4521",
    tools=[transfer_funds_function]
)
# result.function_call = {"name": "transfer_funds", "args": {"amount": 5000, ...}}

# YOUR code decides whether to execute
if result.function_call.args["amount"] > 1000:
    approval = await get_human_approval(result.function_call)
    if not approval:
        return "Transfer requires manager approval."
# Only then call the actual banking API
execute_transfer(result.function_call.args)
```

---

## 8. Teach Extensions with examples, not just descriptions

**Why:** The agent uses the model and examples at runtime to decide which Extension would be suitable for solving the user's query. This highlights a key strength of Extensions — their built-in example types that allow the agent to dynamically select the most appropriate Extension.

**Practical Example:**

```python
flight_extension = Extension(
    name="google_flights",
    description="Search for flights between cities",
    examples=[
        {"user": "Find me flights from Austin to Zurich next Friday",
         "params": {"origin": "AUS", "dest": "ZRH", "date": "2026-04-10"}},
        {"user": "Cheapest flight to Tokyo in July",
         "params": {"dest": "TYO", "date_range": "2026-07-01/2026-07-31",
                    "sort": "price"}},
    ]
)
```

---

## 9. Route models by task complexity — frontier for hard tasks, Flash for cheap ones

**Why:** Use frontier model (Gemini 3 Pro) for complex reasoning, route simpler tasks to cost-effective model (Gemini Flash). Not every task requires the smartest model.

**Practical Example:**

```python
ROUTING = {
    "classify_intent": "gemini-3-flash",   # fast, cheap
    "extract_entities": "gemini-3-flash",
    "plan_complex_task": "gemini-3-pro",   # deep reasoning
    "generate_report": "gemini-3-pro",
}

model = get_model(ROUTING[current_task])
```

---

## 10. System prompt = agent's constitution: persona + constraints + output schema + tool guidance

**Why:** The system prompt serves as the agent's constitution — persona, constraints, output schema, tone of voice, tool guidance.

**Practical Example:**

```xml
<persona>You are a senior financial analyst agent.</persona>
<constraints>
- Never recommend specific stocks. Provide analysis only.
- Always cite data sources.
</constraints>
<output_schema>
Return JSON: {"analysis": str, "confidence": float, "sources": list}
</output_schema>
<tool_guidance>
- Use market_data_tool for real-time prices.
- Use sec_filings_tool for quarterly reports.
- Use calculator_tool for financial ratios.
</tool_guidance>
```

---

## 11. Implement memory at two levels: short-term (scratchpad) and long-term (persistent store)

**Why:** Short-term memory is an active scratchpad for the current conversation and (Action, Observation) pairs. Long-term memory persists across sessions and enables continuity.

**Practical Example:**

```python
# Short-term: conversation state within the session
session_state = {
    "current_task": "diagnose printer issue",
    "steps_tried": ["restarted printer", "checked cable"],
    "user_model": "HP LaserJet Pro M404n"
}

# Long-term: persisted to external store
await memory_store.save(
    user_id="user_123",
    key="device_preferences",
    value={"default_printer": "HP LaserJet", "os": "Windows 11"}
)

# Inject into prompt on next session
user_prefs = await memory_store.get(user_id="user_123")
```

---

## 12. Implement deterministic guardrails OUTSIDE the model's reasoning — hardcoded rules as a chokepoint

**Why:** Layer 1 — Deterministic Guardrails: Hardcoded rules as a security chokepoint outside the model's reasoning (e.g., block purchases over $100). The model cannot reason its way past these.

**Practical Example:**

```python
# This runs BEFORE the model's output reaches execution
def guardrail_check(tool_call):
    if tool_call.name == "make_purchase" and tool_call.args["amount"] > 100:
        raise BlockedAction("Purchases over $100 require human approval")
    if tool_call.name == "delete_account":
        raise BlockedAction("Account deletion is never automated")
    if contains_pii(tool_call.args):
        raise BlockedAction("PII detected in tool arguments")
    return tool_call  # safe to proceed
```

---

## 13. Use RAG (Data Stores) to ground the agent in your domain data — don't rely on training data alone

**Why:** Data Stores address the limitation of static training data by providing access to more dynamic and up-to-date information, ensuring responses remain grounded in factuality and relevance.

**Practical Example:**

```python
# Index your docs
data_store = DataStore.create(
    sources=["gs://company-docs/policies/", "gs://company-docs/products/"],
    embedding_model="text-embedding-004",
    chunk_size=512
)

# Agent uses it automatically via RAG
agent = Agent(
    model="gemini-3-pro",
    tools=[data_store],  # agent queries vector DB on demand
    instructions="Answer using company documentation. If not found, say so."
)
```

**RAG lifecycle:**
1. User query → embedding model generates query embeddings
2. Query embeddings matched against vector database (e.g., using ScaNN)
3. Matched content retrieved in text format → sent to agent
4. Agent receives user query + retrieved content → formulates response
5. Final response sent to user

---

## 14. Enhance model performance with targeted learning: in-context, retrieval-based, or fine-tuning

**Why:** Real-world scenarios often require knowledge beyond training data. Fine-tuning involves training a model on a larger dataset of specific examples prior to inference, helping it understand when and how to apply certain tools.

**Three approaches:**

| Approach | Speed | Cost | Best for |
|---|---|---|---|
| In-context learning | Fast | Low | Quick prototyping, few-shot examples |
| Retrieval-based in-context | Medium | Medium | Dynamic tool/example selection at runtime |
| Fine-tuning | Slow | High | Scale, specific tool selection patterns |

**Practical Example — Fine-tuning:**

```python
# Create training data: (user_query, correct_tool, correct_params)
training_examples = [
    {"input": "Check my order status",
     "output": {"tool": "lookup_order", "args": {"query_type": "status"}}},
    {"input": "I want to return this item",
     "output": {"tool": "initiate_return", "args": {"reason": "unspecified"}}},
    # ... hundreds of examples covering edge cases
]

# Fine-tune the model for your agent's tool landscape
tuned_model = gemini.fine_tune(
    base_model="gemini-3-flash",
    training_data=training_examples,
    task="function_calling"
)
```

---

## 15. Start simple, iterate — no two agents are the same

**Why:** Building complex agent architectures demands an iterative approach. Experimentation and refinement are key to finding solutions for specific business cases. No two agents are created alike due to the generative nature of the foundational models.

**Practical Example:**

```
Week 1: Single agent + 3 tools + ReAct. Test on 50 real queries.
Week 2: Add RAG data store. Measure answer accuracy delta.
Week 3: Add guardrails based on failure modes found in weeks 1-2.
Week 4: Split into multi-agent only if single agent hits tool confusion.
Week 5: Fine-tune Flash model on your tool selection data.
Week 6: Production deploy with human-in-the-loop on high-risk actions.
```

---
---

# Part 4: Cross-Cutting Insights & Chinese Model Notes

---

## Kimi K2 / K2.5 Specific Notes

These are model-specific considerations when building agents with Moonshot's Kimi models:

1. **Keep `reasoning_content` in context during multi-step tool calls.** During multi-step tool calling with thinking mode enabled, you must keep the `reasoning_content` from the assistant message in the current turn's tool call within the context, otherwise an error will be thrown. This is analogous to Gemini's Thought Signatures.

2. **Don't over-instruct tool usage.** Tool usage is often automatic in Kimi K2. You don't need to explicitly tell the model when to use a tool in the prompt — doing so can actually interfere with its autonomous decision-making. Instead, define the goals and constraints.

3. **Set maximum tool-call counts per task.** A swarm can multiply tool calls across many sub-agents, so limits protect your time and your wallet. Kimi K2.5 Agent Swarm supports up to 1,500 tool calls in one session — without limits, costs can grow quickly.

4. **Design clear parallel subtasks for Agent Swarm.** The orchestrator breaks the main task into smaller tasks, then assigns them to sub-agents working in parallel. Your prompt should describe clear parallel subtasks if you want more parallel work — Kimi may choose fewer agents than the maximum.

5. **Include visual standards in your system prompt.** Kimi K2.5 can run Python for data visualization. Define hex color codes, font sizes, and layout standards in the prompt for consistent, boardroom-ready outputs.

6. **Mandate specific citation formats to mitigate hallucinations.** Instead of allowing generic "Source: Internet" tags, require the model to specify the publishing institution and the exact article title.

---

## Universal Principles (Apply to ALL Models)

These principles appear across all three guides and apply regardless of which model you're using:

| Principle | Anthropic | OpenAI | Google |
|---|---|---|---|
| Minimize context, maximize signal | ✅ | ✅ | ✅ |
| Structure prompts with clear sections | ✅ | ✅ | ✅ |
| Build non-overlapping, descriptive tools | ✅ | ✅ | ✅ |
| Use few-shot examples over rule walls | ✅ | ✅ | ✅ |
| Start simple, iterate on failures | ✅ | ✅ | ✅ |
| Maximize single agent before multi-agent | ✅ | ✅ | ✅ |
| Implement compaction/summarization for long tasks | ✅ | ✅ | ✅ |
| Layer multiple guardrails | — | ✅ | ✅ |
| Human-in-the-loop for high-risk actions | ✅ | ✅ | ✅ |
| Route models by task complexity | — | ✅ | ✅ |
| Evals before and after every change | — | ✅ | ✅ |
| Just-in-time context retrieval | ✅ | — | ✅ |
| Structured note-taking / persistent memory | ✅ | ✅ | ✅ |

---

## The Master Decision Tree

```
Starting a new agent project?
│
├─ Step 1: Define the task
│   └─ Can a single LLM call solve it? → Don't build an agent. Use a prompt.
│
├─ Step 2: Choose your model
│   ├─ Start with the best model available (GPT-5, Claude Opus, Gemini 3 Pro)
│   └─ Downgrade to cheaper models after evals pass
│
├─ Step 3: Write minimal system prompt
│   ├─ Persona + constraints + output format + tool guidance
│   └─ Test. Add instructions ONLY for observed failures.
│
├─ Step 4: Add tools
│   ├─ Each tool: one job, clear name, typed params, descriptive docstring
│   ├─ Categorize: Data (read) vs Action (write) vs Orchestration (agent-as-tool)
│   └─ Risk-rate each tool: low / medium / high
│
├─ Step 5: Add few-shot examples
│   └─ 3-5 diverse, canonical examples > 15 rules
│
├─ Step 6: Set up evals
│   ├─ Tool selection accuracy
│   ├─ End-to-end task completion
│   └─ Context retention across turns
│
├─ Step 7: Handle long conversations
│   ├─ Independent tasks → Trimming (keep last N turns)
│   ├─ Dependent tasks → Summarization (compress + carry forward)
│   └─ Both → Hybrid (trim tool results, summarize decisions)
│
├─ Step 8: Add guardrails
│   ├─ Layer 1: Deterministic (regex, blocklists, amount limits)
│   ├─ Layer 2: LLM-based (relevance, safety, PII classifiers)
│   └─ Layer 3: Human escalation (failure thresholds, high-risk actions)
│
├─ Step 9: Consider multi-agent only if needed
│   ├─ Too many if-else branches in prompt → Split agents
│   ├─ Tools overlap and confuse the model → Split agents
│   ├─ Need synthesis across sub-results → Manager pattern
│   └─ Need full handoff to specialist → Decentralized pattern
│
└─ Step 10: Ship, monitor, iterate
    ├─ Log every tool call, every turn, every failure
    ├─ Replay transcripts to catch regressions
    └─ Remove prompt lines that don't trace to a real failure
```

---

*Total checklist items: 50 (17 Anthropic + 19 OpenAI + 15 Google + Kimi notes)*

*Last updated: April 2026*