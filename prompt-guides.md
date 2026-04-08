# AI Engineering Checklists

**Prompt Engineering + Context Engineering + Agent Architecture**
**Anthropic (Claude) · OpenAI (GPT) · Google (Gemini)**
*Compiled April 2026*

---

# PART 1 — CONTEXT ENGINEERING & AGENT CHECKLISTS

---

## 1. Anthropic: Context Engineering for AI Agents

*Source: [Effective Context Engineering for AI Agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)*

**1. Minimize context, maximize signal.**
Why: As context grows, model recall degrades (context rot). Every token competes for a finite attention budget — n² pairwise relationships means bloat kills precision.
```
# Instead of dumping an entire 500-line API response into context,
# parse it first and only pass the 3-4 fields the agent actually needs.
# In your tool implementation:
return { id, status, error }
# NOT:
return entireAPIResponse
```

**2. Write system prompts at the "right altitude" — specific enough to guide, flexible enough to generalize.**
Why: Too rigid (hardcoded if-else logic) creates brittleness. Too vague assumes shared context the model doesn't have. You want strong heuristics, not scripts.
```
Bad (too rigid):
"If the user says 'deploy', run deploy.sh. If the user says 'test', run test.sh."

Bad (too vague):
"Help the user with their code."

Good:
"You are a deployment assistant. When the user wants to ship code,
verify tests pass before deploying. If tests fail, diagnose the failure
and suggest fixes before retrying. Never deploy with failing tests."
```

**3. Structure prompts with clear sections.**
Why: Delineation helps the model parse intent per section instead of treating your prompt as one undifferentiated blob.
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

**4. Start with a minimal prompt on the best model, then iterate based on failure modes.**
Why: You avoid over-engineering upfront. Real failures tell you exactly what instructions to add.
```
Start with:
"You are a customer support agent. Answer questions using the
knowledge base tool."

Then after testing you notice it hallucinates when the KB has no answer:
Add: "If the knowledge base returns no relevant results, say you don't
know and escalate to a human. Never guess."
```

**5. Build non-overlapping tools with descriptive parameters.**
Why: If a human engineer can't definitively say which tool should be used in a given situation, the agent can't either.
```python
# Bad: ambiguous overlap
search_data(query)
find_info(query)

# Good: single job, clear input, zero ambiguity
search_customers(email: str)
search_orders(order_id: str)
```

**6. Make tools return token-efficient responses.**
Why: Bloated tool outputs eat your attention budget on data the agent may not even need.
```python
def search_customer(email):
    raw = api.get_customer(email)
    return {"name": raw.name, "plan": raw.plan, "status": raw.status}
    # NOT: return raw  (40 fields including internal metadata)
```

**7. Use diverse canonical examples instead of exhaustive rule walls.**
Why: Examples are the "pictures" worth a thousand words for an LLM. A curated set teaches behavior more reliably than a wall of rules.
```xml
<examples>
<example>
User: "This is broken, fix it now!"
Assistant: "I understand the urgency. Let me look into this right away.
Can you share the error message you're seeing?"
</example>
<example>
User: "Hey, quick question about billing"
Assistant: "Sure! What's your billing question?"
</example>
<example>
User: "I want to cancel."
Assistant: "I'm sorry to hear that. Before I process the cancellation,
can I ask what's prompting the change?"
</example>
</examples>
```

**8. Use just-in-time context retrieval.**
Why: Rather than pre-loading everything, agents can maintain lightweight identifiers and dynamically load data at runtime.
```
You have access to these tools:
- read_file(path): reads a file's contents
- list_directory(path): lists files in a directory
- search_codebase(query): greps across the repo

Do NOT ask for all files upfront. Navigate the codebase as needed.
```

**9. Let agents progressively discover context through exploration.**
Why: Folder hierarchies, naming conventions, timestamps — metadata provides free signals.
```
When starting a task:
1. First run list_directory(".") to understand project structure.
2. Read README.md or config files to understand conventions.
3. Then navigate to the relevant files based on what you learned.
Do not assume file locations. Explore first.
```

**10. Implement compaction.**
Why: Compaction distills context in a high-fidelity manner, enabling the agent to continue with minimal performance degradation.
```
If message_history token count > 80% of context window:
    Call the model with: "Summarize this conversation so far.
    Preserve: all architectural decisions, unresolved bugs,
    file paths modified, current task status.
    Discard: redundant tool outputs, exploratory dead ends."
    Replace message_history with summary + last 5 messages.
```

**11. Tune compaction for recall first, then precision.**
Why: Aggressive compaction loses subtle details whose importance only shows up later.
```
"When summarizing, ERR ON THE SIDE OF KEEPING TOO MUCH.
Include: decisions made and why, files touched, errors encountered,
constraints discovered, things still to do.
Only exclude: raw file contents already saved to disk,
duplicate tool calls that returned the same result."
```

**12. Clear old tool call results from history.**
Why: Once a tool has been called deep in the message history, the agent doesn't need the raw result again.
```python
for msg in message_history[:-10]:  # keep last 10 intact
    if msg.role == "tool_result":
        msg.content = f"[Result from {msg.tool_name} — already processed]"
```

**13. Implement structured note-taking (external persistent memory).**
Why: Notes persisted outside the context window get pulled back in when needed.
```
You have a scratchpad tool: write_notes(content) and read_notes().
After completing each subtask, write a progress note:
- What you just did
- What you learned
- What's left to do
Before starting work, always read_notes() to restore context.
```

**14. Use sub-agent architectures for complex tasks.**
Why: Each sub-agent explores extensively but returns only a condensed summary.
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

**15. Match your context strategy to the task type.**
Why: Compaction suits back-and-forth conversations. Note-taking suits iterative work. Multi-agent handles parallel exploration.
```python
if task.type == "conversation":
    strategy = "compaction"
elif task.type == "multi_step_build":
    strategy = "note_taking"
elif task.type == "research":
    strategy = "sub_agents"
```

**16. Use a hybrid retrieval strategy.**
Why: Pre-loaded context gives speed, autonomous exploration gives freshness.
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

**17. Default to the simplest thing that works.**
Why: As model capabilities improve, over-engineering today becomes technical debt tomorrow.
```
Before adding complexity, ask: does the model already handle this?

Start with:
"You are a coding assistant. Help the user with their codebase."

Only add guardrails, tools, and structure when you observe
specific failures. Every line in your prompt should trace back
to a real failure you saw in testing — if it doesn't, delete it.
```

---

## 2. OpenAI: Building Agents + Context Management

*Sources: [A Practical Guide to Building Agents](https://cdn.openai.com/business-guides-and-resources/a-practical-guide-to-building-agents.pdf) · [Context Engineering - Session Memory](https://cookbook.openai.com/examples/agents_sdk/session_memory)*

**1. Prototype with the best model, then downgrade where possible.**
Why: Establishes a performance ceiling so you know exactly where smaller models break.
```python
triage_agent = Agent(model="gpt-4o-mini", ...)  # simple routing
refund_agent = Agent(model="gpt-5", ...)         # complex judgment
```

**2. Set up evals before anything else.**
Why: Without a baseline, you can't tell if your changes help or hurt.
```python
evals = [
    {"input": "I want a refund for order #123", "expected_tool": "lookup_order"},
    {"input": "What's your return policy?", "expected_tool": "search_kb"},
]
```

**3. Give each tool a single job with clear name, description, and typed parameters.**
Why: If improving tool clarity doesn't fix wrong tool selection, then split into multiple agents.
```python
@function_tool
def lookup_order(order_id: str) -> dict:
    """Retrieve order status, items, and shipping info by order ID."""
    return db.orders.find(order_id)
# NOT: get_data(query: str) — ambiguous, no typed params
```

**4. Categorize tools: Data (read), Action (write), Orchestration (agent-as-tool).**
Why: Different risk profiles need different guardrails.
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

**5. Convert existing docs/SOPs into agent instructions.**
Why: Existing operating procedures map directly to LLM-friendly routines.
```
"You are an expert in writing instructions for an LLM agent.
Convert the following help center document into a clear set of
numbered instructions for an agent to follow.
Ensure there is no ambiguity.

Document: {help_center_doc}"
```

**6. Define clear actions in every instruction step.**
Why: Every step should correspond to a specific action or output.
```
Good:
"1. Call lookup_order(order_id) to get order details.
 2. If order is within 30-day window AND status is 'delivered',
    call issue_refund(order_id, amount).
 3. If outside window, tell the user: 'This order is past our
    30-day return policy.'"
```

**7. Anticipate edge cases with conditional branches.**
Why: Real-world interactions create decision points.
```
"If the user doesn't provide an order ID:
   → Ask: 'Could you share your order number? It starts with ORD-.'
If the user provides an email instead:
   → Call lookup_orders_by_email(email) and present the list.
If multiple orders match:
   → List them and ask which one they need help with."
```

**8. Use prompt templates with policy variables.**
Why: A single flexible base prompt adapts to various contexts.
```python
TEMPLATE = """
You are a call center agent for {{company_name}}.
The customer {{user_first_name}} has been a member for {{user_tenure}}.
Their complaint history: {{user_complaint_categories}}.
Refund policy: {{refund_policy}}
Greet them, thank them for their loyalty, and resolve their issue.
"""
```

**9. Maximize single-agent capabilities before going multi-agent.**
Why: More agents introduce additional complexity and overhead.
```python
support_agent = Agent(
    name="Support",
    tools=[search_kb, lookup_order, issue_refund, escalate_to_human],
    instructions="..."
)
# Only split when instructions have too many if-else branches
# or tools overlap and the agent picks wrong ones
```

**10. Pick the right multi-agent pattern: Manager vs. Decentralized.**
Why: Manager keeps one agent in control (good for synthesis). Decentralized lets specialists take over (good for routing).
```python
# Manager pattern
manager = Agent(
    tools=[
        billing_agent.as_tool(tool_name="billing", ...),
        technical_agent.as_tool(tool_name="technical", ...),
    ]
)

# Decentralized pattern
triage_agent = Agent(
    handoffs=[billing_agent, technical_agent, sales_agent]
)
```

**11. Implement context trimming — keep last N turns, drop the rest.**
Why: If too much is carried forward, the model risks distraction, inefficiency, or failure.
```python
session = TrimmingSession("session_123", max_turns=8)
result = await Runner.run(agent, user_message, session=session)
```

**12. Implement context summarization for long-horizon tasks.**
Why: Summaries act as "clean rooms" that correct or omit prior mistakes.
```python
session = SummarizingSession(
    keep_last_n_turns=3,
    context_limit=6,
    summarizer=LLMSummarizer(client, model="gpt-4o")
)
```

**13. Design summarization prompts for the specific use case.**
Why: A well-crafted prompt should be tailored like a support agent handing off a case.
```
SUMMARY_PROMPT = """
Compress earlier conversation into a snapshot with these sections:
• Product & Environment (device, OS, versions)
• Reported Issue (single sentence, latest state)
• Steps Tried & Results (chronological bullets)
• Identifiers (ticket #, serial, account — only if provided)
• Current Status & Blockers
• Next Recommended Step

Rules: ≤200 words. No invented facts. Quote error codes exactly.
Mark uncertain facts as UNVERIFIED.
"""
```

**14. Add hallucination control to your summarization prompt.**
Why: Even minor hallucinations in a summary propagate forward.
```
"Before writing the summary (do this silently):
 - Contradiction check: compare user claims with tool logs.
 - If any fact is uncertain, mark it UNVERIFIED.
 - Do not invent new facts. Quote error strings/codes exactly."
```

**15. Layer multiple guardrails.**
Why: A single guardrail is unlikely to provide sufficient protection.
```python
agent = Agent(
    input_guardrails=[
        Guardrail(guardrail_function=jailbreak_detector),
        Guardrail(guardrail_function=pii_filter),
        Guardrail(guardrail_function=relevance_classifier),
    ],
)
```

**16. Assign risk ratings to tools and gate high-risk ones.**
Why: Read-only vs. write access, reversibility, financial impact all matter.
```python
TOOL_RISK = {
    "search_kb": "low",
    "update_address": "medium",
    "issue_refund": "high",
}
if TOOL_RISK[tool_name] == "high":
    confirmation = await get_human_approval(tool_name, args)
```

**17. Build human escalation with explicit triggers.**
Why: If the agent exceeds retry limits or faces irreversible actions, it should transfer control.
```
"If you fail to understand the customer's intent after 3 attempts,
 say: 'Let me connect you with a specialist.'
 Then call escalate_to_human(reason='intent_unclear').

 If the action involves canceling an account or refunding >$500,
 always call escalate_to_human before proceeding."
```

**18. Run evals on your context management.**
Why: How do you know the model isn't losing or confusing context?
```python
for convo in test_conversations:
    full_result = await run_agent(convo, trimming=False)
    trimmed_result = await run_agent(convo, trimming=True)
    score = compare_entity_recall(full_result, trimmed_result)
    log_eval(convo.id, score)
```

---

## 3. Google: Agent Architecture + Gemini 3 Features

*Sources: [Introduction to Agents Whitepaper](https://ppc.land/content/files/2025/01/Newwhitepaper_Agents2.pdf) · [Gemini 3 Features Blog](https://developers.googleblog.com/building-ai-agents-with-google-gemini-3-and-open-source-frameworks/)*

**1. Build on three core components: Model + Tools + Orchestration layer.**
Why: An agent is an application that achieves a goal by observing the world and acting upon it using tools.
```python
agent = Agent(
    model="gemini-3-pro",
    tools=[search_kb, create_ticket],
    instructions="...",
)
```

**2. Choose a reasoning framework: ReAct, CoT, or ToT.**
Why: These provide a framework for the orchestration layer to take in information, reason, and generate decisions.
```
"For each user request, follow this loop:
 1. Thought: reason about what you need to do next
 2. Action: choose a tool and its inputs
 3. Observation: read the tool result
 4. Repeat until you have enough info, then give Final Answer."
```

**3. Use thinking_level to control reasoning depth per request.**
Why: Set high for deep planning, low for fast throughput tasks.
```python
triage_response = model.generate(prompt=msg, thinking_level="low")
refund_response = model.generate(prompt=ctx, thinking_level="high")
```

**4. Pass Thought Signatures back in conversation history.**
Why: The model retains its exact train of thought for reliable multi-step execution.
```python
response = model.generate(prompt=step_1, tools=tools)
thought_sig = response.thought_signature
response_2 = model.generate(
    prompt=step_2, tools=tools,
    previous_thought_signature=thought_sig
)
```

**5. Use media_resolution to control multimodal token costs.**
Why: High for fine text in images, medium for PDFs, low for video summaries.
```python
response = model.generate(content=[pdf, "Extract clauses"], media_resolution="high")
response = model.generate(content=[video, "Summarize"], media_resolution="low")
```

**6. Categorize tools: Extensions (agent-side), Functions (client-side), Data Stores (RAG).**
Why: Extensions call APIs directly. Functions let you control execution. Data Stores provide RAG.
```python
# Extension — agent calls the API directly
flight_ext = Extension(name="google_flights", api_endpoint="...", examples=[...])

# Function — agent generates params, YOU call the API
@function_tool
def issue_refund(order_id: str, amount: float): ...

# Data Store — RAG over your docs
data_store = DataStore(source="gs://my-bucket/kb/", embedding_model="text-embedding-004")
```

**7. Use Functions (client-side) when you need auth control or human-in-the-loop.**
Why: Security restrictions or timing constraints prevent the agent from calling APIs directly.
```python
result = model.generate(prompt="Transfer $5000", tools=[transfer_func])
if result.function_call.args["amount"] > 1000:
    approval = await get_human_approval(result.function_call)
    if not approval:
        return "Transfer requires manager approval."
execute_transfer(result.function_call.args)
```

**8. Teach Extensions with examples, not just descriptions.**
Why: The agent uses examples at runtime to select the most appropriate Extension.
```python
flight_ext = Extension(
    name="google_flights",
    description="Search for flights between cities",
    examples=[
        {"user": "Flights Austin to Zurich next Friday",
         "params": {"origin": "AUS", "dest": "ZRH", "date": "2026-04-10"}},
    ]
)
```

**9. Route models by task complexity.**
Why: Frontier for complex reasoning, Flash for cheap tasks.
```python
ROUTING = {
    "classify_intent": "gemini-3-flash",
    "plan_complex_task": "gemini-3-pro",
}
```

**10. System prompt = agent's constitution.**
Why: Persona, constraints, output schema, and tool guidance all belong here.
```xml
<persona>You are a senior financial analyst agent.</persona>
<constraints>Never recommend specific stocks. Provide analysis only.</constraints>
<output_schema>Return JSON: {"analysis": str, "confidence": float, "sources": list}</output_schema>
<tool_guidance>
- Use market_data_tool for real-time prices.
- Use sec_filings_tool for quarterly reports.
</tool_guidance>
```

**11. Implement memory at two levels: short-term and long-term.**
Why: Short-term is the scratchpad for the current session. Long-term persists across sessions.
```python
session_state = {"current_task": "diagnose printer", "steps_tried": [...]}
await memory_store.save(user_id="user_123", key="prefs", value={...})
user_prefs = await memory_store.get(user_id="user_123")
```

**12. Implement deterministic guardrails OUTSIDE the model's reasoning.**
Why: Hardcoded rules as a security chokepoint that the model cannot bypass.
```python
def guardrail_check(tool_call):
    if tool_call.name == "make_purchase" and tool_call.args["amount"] > 100:
        raise BlockedAction("Purchases over $100 require human approval")
    if tool_call.name == "delete_account":
        raise BlockedAction("Account deletion is never automated")
    return tool_call
```

**13. Use RAG (Data Stores) to ground the agent in your domain data.**
Why: Data Stores provide access to dynamic, up-to-date information beyond training data.
```python
data_store = DataStore.create(
    sources=["gs://company-docs/policies/", "gs://company-docs/products/"],
    embedding_model="text-embedding-004", chunk_size=512
)
agent = Agent(model="gemini-3-pro", tools=[data_store],
    instructions="Answer using company documentation. If not found, say so.")
```

**14. Use fine-tuning for tool selection at scale.**
Why: Real-world scenarios often require knowledge beyond training data.
```python
training_examples = [
    {"input": "Check my order status",
     "output": {"tool": "lookup_order", "args": {"query_type": "status"}}},
    {"input": "I want to return this item",
     "output": {"tool": "initiate_return", "args": {"reason": "unspecified"}}},
]
tuned_model = gemini.fine_tune(base_model="gemini-3-flash", training_data=training_examples)
```

**15. Start simple, iterate.**
Why: No two agents are created alike. Experimentation and refinement are key.
```
Week 1: Single agent + 3 tools + ReAct. Test on 50 real queries.
Week 2: Add RAG data store. Measure answer accuracy delta.
Week 3: Add guardrails based on failure modes found in weeks 1-2.
Week 4: Split into multi-agent only if single agent hits tool confusion.
Week 5: Fine-tune Flash model on your tool selection data.
Week 6: Production deploy with human-in-the-loop on high-risk actions.
```

---

# PART 2 — PROMPT ENGINEERING CHECKLISTS

---

## 4. Anthropic: Prompt Engineering for Claude 4.x

*Source: [Prompting Best Practices](https://docs.anthropic.com/en/docs/build-with-claude/prompt-engineering/claude-4-best-practices)*

**1. Be explicit — say exactly what you want.**
Why: Claude 4.x models respond well to clear, explicit instructions. "Above and beyond" behavior needs to be explicitly requested.
```
Bad:  "Create an analytics dashboard."
Good: "Create an analytics dashboard. Include filters for date range
       and region, a KPI summary row, line charts for trends, and
       export-to-CSV. Add hover tooltips, loading skeletons,
       and responsive layout."
```

**2. Explain WHY, not just WHAT — give context behind instructions.**
Why: Providing context or motivation helps Claude better understand your goals.
```
Bad:  "Use bullet points sparingly."
Good: "Use bullet points sparingly. This output will be pasted into
       a client-facing email, and heavy formatting looks unprofessional
       in email clients."
```

**3. Audit your examples — they steer behavior more than rules.**
Why: Claude 4.x pays close attention to details and examples. Ensure examples align with desired behaviors.
```xml
<!-- Include examples of varying lengths matching real scenarios -->
<examples>
<example>
<query>What is a VPN?</query>
<response>A VPN encrypts your internet traffic and routes it through
a remote server, hiding your IP address.</response>
</example>
<example>
<query>Explain how TLS 1.3 differs from TLS 1.2</query>
<response>[4-paragraph detailed technical comparison]</response>
</example>
</examples>
```

**4. Tell Claude what TO DO, not what NOT to do.**
Why: Positive instructions are more effective than negative constraints.
```
Bad:  "Don't be formal. Don't use jargon. Don't write long paragraphs."
Good: "Write in a casual, conversational tone. Use plain language.
       Keep paragraphs to 2-3 sentences."
```

**5. Use XML tags to structure your prompt.**
Why: XML tags create unambiguous boundaries between sections.
```xml
<role>You are a senior code reviewer.</role>
<context>The user is submitting a PR for a payments microservice.</context>
<instructions>
Review for security vulnerabilities, race conditions, and PCI compliance.
Flag each finding with severity (critical/high/medium/low).
</instructions>
<output_format>
Return findings as: {file, line, severity, issue, suggestion}
</output_format>
```

**6. Use XML format indicators to steer output formatting.**
Why: Wrapping expected output in XML tags effectively controls formatting.
```
"Write your analysis in <flowing_prose> tags. Do not use bullet
points or numbered lists inside these tags."
```

**7. Match your prompt style to your desired output style.**
Why: The formatting in your prompt influences Claude's response style.
```
Bad (prompt full of bullets → gets bullets back):
  "- Analyze the data
   - Find trends
   - Summarize findings"

Good (prompt in prose → gets prose back):
  "Analyze the data, identify the three most significant trends,
   and write a two-paragraph summary of your findings."
```

**8. Prefill Claude's response to lock in format.**
Why: Starting the response eliminates "how should I begin" ambiguity.
```python
messages = [
    {"role": "user", "content": "Classify this ticket: '{text}'"},
    {"role": "assistant", "content": '{"category": "'}
]
```

**9. Use chain-of-thought for complex reasoning.**
Why: Intermediate reasoning steps reduce errors on complex tasks.
```
"Before giving your final answer, work through the problem
step by step inside <thinking> tags. Then provide your final
answer in <answer> tags."
```

**10. Use extended thinking for complex multi-step reasoning.**
Why: Claude 4.x offers thinking capabilities especially helpful for reflection after tool use.
```
"After receiving tool results, carefully reflect on their quality
and determine optimal next steps before proceeding."
```

**11. Avoid the word "think" when extended thinking is disabled.**
Why: Claude Opus 4.5 is particularly sensitive to the word "think" when thinking is off.
```
Bad:  "Think about whether this approach is correct."
Good: "Evaluate whether this approach is correct."
```

**12. Default to action, not suggestion — be explicit about which.**
Why: "Can you suggest changes" may produce suggestions instead of implementations.
```xml
<!-- Action by default: -->
<default_to_action>
By default, implement changes rather than only suggesting them.
If the user's intent is unclear, infer the most useful action
and proceed, using tools to discover missing details.
</default_to_action>

<!-- Or suggestion only: -->
<do_not_act_before_instructions>
Do not jump into implementation unless clearly instructed.
Default to providing information and recommendations.
</do_not_act_before_instructions>
```

**13. Dial back aggressive tool-triggering language for Claude 4.x.**
Why: Claude Opus 4.5 is more responsive to the system prompt. Old "CRITICAL: MUST" phrasing now overtriggers.
```
Bad:  "CRITICAL: You MUST ALWAYS call search_kb before answering."
Good: "Use search_kb when the user asks a factual question that
       might not be in your training data."
```

**14. Explicitly request summaries after tool use.**
Why: Claude 4.5 may skip verbal summaries after tool calls, jumping to the next action.
```
"After completing a task that involves tool use, provide a quick
summary of the work you've done."
```

**15. Enable parallel tool calling with explicit instructions.**
Why: Claude 4.x excels at parallel tool execution with explicit prompting.
```xml
<use_parallel_tool_calls>
If you intend to call multiple tools and there are no dependencies,
make all independent calls in parallel. When reading 3 files,
run 3 tool calls simultaneously. If some calls depend on previous
results, call those sequentially. Never guess missing parameters.
</use_parallel_tool_calls>
```

**16. Tell Claude its context will be compacted — prevent premature wrap-up.**
Why: Claude may try to wrap up work as it approaches the context limit.
```
"Your context window will be automatically compacted as it
approaches its limit. Do not stop tasks early due to token budget
concerns. Save progress to memory before context refreshes."
```

**17. For multi-window workflows: write tests first, create setup scripts, use git.**
Why: Use the first context window for scaffolding, then iterate in future windows.
```
"Before starting implementation:
 1. Write all test cases in tests.json
 2. Create init.sh to start servers and run linters
 3. Commit this scaffolding to git
Then begin implementation. After each feature, run tests and commit."
```

**18. For research tasks, use structured hypothesis tracking.**
Why: Claude can find and synthesize information iteratively when given structure.
```
"Search in a structured way. Develop competing hypotheses.
Track confidence levels. Regularly self-critique. Update a
hypothesis tree or research notes file."
```

**19. Force Claude to read code before proposing changes.**
Why: Claude can propose solutions without looking at code or make assumptions about unread code.
```xml
<investigate_before_answering>
ALWAYS read relevant files before proposing edits.
Do not speculate about code you have not opened.
If the user references a file, you MUST open and inspect it first.
</investigate_before_answering>
```

**20. Prevent overengineering — keep solutions minimal.**
Why: Claude tends to overengineer by creating extra files and unnecessary abstractions.
```
"Avoid over-engineering. Only make changes directly requested.
Don't add features beyond what was asked.
Don't add error handling for impossible scenarios.
Don't create abstractions for one-time operations.
The right complexity is the minimum for the current task."
```

**21. Fight "AI slop" in frontend design.**
Why: Without guidance, models default to generic Inter fonts, purple gradients, and predictable layouts.
```xml
<frontend_aesthetics>
Avoid: Inter, Roboto, Arial. No purple gradients on white.
Instead: Distinctive typography, cohesive color themes with sharp
accents, CSS animations for micro-interactions, atmospheric
backgrounds with layered gradients.
</frontend_aesthetics>
```

**22. Prevent hard-coding — solve generally, not just for tests.**
Why: Claude can focus on passing tests instead of implementing general solutions.
```
"Implement a solution that works for all valid inputs, not just
test cases. Do not hard-code values. If tests are incorrect,
inform me rather than working around them."
```

**23. Give Claude a crop tool for vision tasks.**
Why: Testing shows consistent uplift when Claude can "zoom" in on image regions.
```python
@tool
def crop_image(image_path: str, x: int, y: int, w: int, h: int):
    """Crop a region to examine details more closely."""
    return Image.open(image_path).crop((x, y, x+w, y+h))
```

**24. Use git as a state tracking mechanism across sessions.**
Why: Git provides a log and checkpoints. Claude 4.5 excels at using git for state tracking.
```
"After each logical unit of work, commit with a descriptive message.
When starting a new session, run git log --oneline -20 and read
progress.txt to restore context."
```

**25. Let subagent orchestration happen naturally.**
Why: Claude 4.5 proactively delegates to subagents without explicit instruction.
```
# Just make subagent tools available and well-described.
# Only add constraints if it's overdoing it:
"Only delegate to subagents when the task clearly benefits
from a separate agent with a new context window."
```

---

## 5. OpenAI: Prompt Engineering for GPT-5 / GPT-5.4

*Sources: [GPT-5 Prompting Guide](https://cookbook.openai.com/examples/gpt-5/gpt-5_prompting_guide) · [GPT-5.4 Prompt Guidance](https://developers.openai.com/api/docs/guides/prompt-guidance)*

**1. Use the Responses API with previous_response_id.**
Why: Score increases of ~4 points by persisting reasoning traces across turns. Eliminates reconstructing plans from scratch.
```python
response = client.responses.create(model="gpt-5", input="Search for X", tools=tools)
response2 = client.responses.create(
    model="gpt-5", input="Summarize findings",
    previous_response_id=response.id  # reasoning persists
)
```

**2. Match reasoning_effort to task complexity.**
Why: Start with none for field extraction and triage. Use medium for synthesis. Use high for complex decisions.
```python
client.responses.create(model="gpt-5", reasoning={"effort": "none"}, ...)   # classify
client.responses.create(model="gpt-5", reasoning={"effort": "high"}, ...)   # complex refund
```

**3. Control agentic eagerness with explicit scope and stop conditions.**
Why: GPT-5 is thorough by default. Define how you want it to explore.
```
<!-- Less eager: -->
<context_gathering>
Goal: Get enough context fast. Stop as soon as you can act.
Early stop: You can name exact content to change.
Maximum 2 tool calls. Bias toward answering quickly.
</context_gathering>

<!-- More eager: -->
<persistence>
Keep going until the query is completely resolved.
Never stop when uncertain — deduce and continue.
Don't ask the human to confirm assumptions.
</persistence>
```

**4. Add tool preambles — narrate what you're doing before each tool call.**
Why: GPT-5 is trained to provide upfront plans and progress updates.
```
<tool_preambles>
Always begin by rephrasing the user's goal before calling tools.
Outline a structured plan. Narrate each step succinctly.
Finish by summarizing completed work.
</tool_preambles>
```

**5. Eliminate contradictions — GPT-5 burns reasoning tokens reconciling them.**
Why: Contradictory instructions cause GPT-5 to waste CoT tokens searching for reconciliation.
```
Bad: "Never schedule without consent."
   + "For urgent cases, auto-assign without contacting the patient."

Good: "Never schedule without consent.
       For urgent cases, auto-assign AFTER informing the patient.
       If consent status is unknown, tentatively hold and request."
```

**6. Use the verbosity API parameter globally, override with natural language for specific contexts.**
Why: GPT-5's verbosity parameter controls final answer length. Override for specific tools.
```python
client.responses.create(model="gpt-5", verbosity="low", ...)
```
```
"Write code for clarity first. Prefer readable solutions
with clear names. Use high verbosity for code tools."
```

**7. For minimal reasoning, add explicit planning prompts.**
Why: Minimal reasoning has fewer internal tokens for planning — prompted planning compensates.
```
"Decompose the query into all required sub-requests.
Confirm each is completed before stopping.
Plan extensively before making function calls."
```

**8. For minimal reasoning, add brief chain-of-thought at the start.**
Why: A bullet-point reasoning summary improves performance on intelligence-heavy tasks.
```
"Before your final answer, provide a brief bullet-point
summary of your reasoning at the top."
```

**9. Audit prompts for ambiguity — use the prompt optimizer tool.**
Why: Multiple early users found ambiguities and contradictions that drastically hurt performance.
```
# Use OpenAI's prompt optimizer:
# https://platform.openai.com/chat/edit?optimize=true

# Or use GPT-5 as its own meta-prompter (see #10)
```

**10. Use GPT-5 as a meta-prompter for itself.**
Why: Early testers deployed prompt revisions to production generated by asking GPT-5 to fix its own prompts.
```
"Here's a prompt: {{prompt}}
Desired behavior: {{desired}}
Actual behavior: {{actual}}
What minimal edits would fix this?"
```

**11. For frontend apps, specify stack, design system, and aesthetics.**
Why: GPT-5 performs best with explicit framework choices and visual hierarchy rules.
```
<frontend_stack_defaults>
Framework: Next.js (TypeScript)
Styling: TailwindCSS
UI: shadcn/ui, Lucide icons
State: Zustand
</frontend_stack_defaults>

<ui_ux_best_practices>
- 4-5 font sizes max
- 1 neutral base + 2 accent colors
- Multiples of 4 for padding/margins
- Skeleton placeholders for loading
</ui_ux_best_practices>
```

**12. For zero-to-one apps, prompt GPT-5 to iterate against its own rubric.**
Why: Self-constructed rubrics improve output quality through self-reflection.
```
<self_reflection>
Think deeply about what makes a world-class web app.
Create a 5-7 category rubric (don't show it).
Use it to internally iterate. If not hitting top marks, start again.
</self_reflection>
```

**13. For existing codebases, summarize engineering principles and directory structure.**
Why: Enhances GPT-5's natural codebase navigation with explicit guidance.
```
<code_editing_rules>
<guiding_principles>
- Modular and reusable components. No duplication.
- Consistent design system — color tokens, typography, spacing.
</guiding_principles>
<directory_structure>
/src/app/api/ → API endpoints
/src/components/ → UI building blocks
/src/hooks/ → Reusable hooks
/src/lib/ → Utilities
</directory_structure>
</code_editing_rules>
```

**14. Be proactive by default in coding — implement, don't suggest.**
Why: Cursor found that framing edits as "proposed changes the user can reject" improved autonomy.
```
"Your code edits will be displayed as proposed changes.
The user can always reject. Make changes proactively
rather than asking whether to proceed."
```

**15. Soften overly aggressive context-gathering prompts from older models.**
Why: "Be THOROUGH" worked for GPT-4 but causes GPT-5 to over-search on simple tasks.
```
Bad:  <maximize_context_understanding>
      Be THOROUGH. Make sure you have the FULL picture.
Good: <context_understanding>
      If you're not confident after a partial edit, gather more info.
      Bias towards finding answers yourself.
```

**16. GPT-5 doesn't output Markdown by default in the API.**
Why: Preserves compatibility with apps that don't render Markdown.
```
"Use Markdown only where semantically correct (inline code,
code fences, lists, tables). Use backticks for file/function names."
```

**17. Re-inject Markdown instructions every 3-5 messages in long conversations.**
Why: Markdown adherence degrades over long conversations.
```python
if turn_count % 4 == 0:
    messages.append({"role": "system",
        "content": "Reminder: format code with backticks, use ### headers."})
```

**18. Define safe vs. unsafe actions with different thresholds per tool.**
Why: Checkout tools need low uncertainty thresholds. Search tools need high thresholds.
```
<tool_safety>
Low risk (proceed freely): search, grep, read_file
Medium risk (log): update_record, send_notification
High risk (confirm first): delete_file, process_payment
</tool_safety>
```

**19. For GPT-5.4: choose reasoning effort by task shape, not intuition.**
Why: The highest-leverage prompt change is matching reasoning effort to task type.
```python
EFFORT_MAP = {
    "field_extraction": "none",
    "support_triage": "none",
    "multi_doc_review": "medium",
    "strategy_writing": "medium",
    "complex_refund": "high",
}
```

**20. Start with the smallest prompt that passes your evals.**
Why: Add blocks only when they fix a measured failure mode.
```
Week 1: "You are a support agent. Use tools to help the user."
Week 2: Evals show over-clarifying → Add persistence block
Week 3: Evals show bad JSON → Add output contract block
Week 4: Evals show over-searching → Soften context-gathering
Each addition traces to a specific failure.
```

---

## 6. Google: Prompt Engineering for Gemini 3

*Sources: [Prompt Design Strategies](https://ai.google.dev/gemini-api/docs/prompting-strategies) · [Gemini 3 Prompting Guide (Vertex AI)](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/start/gemini-3-prompting-guide)*

**1. Be precise and direct — Gemini 3 punishes fluff.**
Why: Gemini 3 responds best to prompts that are direct, well-structured, and clearly define the task.
```
Bad:  "Could you perhaps help me think about ways to maybe
       summarize this document?"
Good: "Summarize this document in 3 bullet points, max 20 words each."
```

**2. Keep temperature at 1.0 for Gemini 3.**
Why: Setting below 1.0 may cause looping or degraded performance on math and reasoning.
```python
# Don't: model.generate(temperature=0.2, ...)
# Do:    model.generate(temperature=1.0, ...)
```

**3. Control output verbosity explicitly — Gemini 3 defaults to terse.**
Why: By default, Gemini 3 provides direct, efficient answers. Request verbosity if needed.
```
# For verbose: "Explain this as a friendly, talkative assistant."
# For terse: just ask normally — it's the default.
```

**4. Use consistent delimiters — pick XML OR Markdown, not both.**
Why: Choose one format and use it consistently within a single prompt.
```xml
<!-- XML style: -->
<role>You are a helpful assistant.</role>
<constraints>1. Be objective. 2. Cite sources.</constraints>
<context>{{data}}</context>
<task>{{request}}</task>
```
```markdown
# Markdown style:
# Identity
You are a senior solution architect.
# Constraints
- Python 3.11+ only
# Output format
Return a single code block.
```

**5. Put critical instructions first — role, constraints, output format at the top.**
Why: Place essential behavioral constraints and role definitions in the System Instruction or at the very beginning.
```
System instruction:
<role>Legal compliance reviewer</role>
<constraints>Never provide legal advice. Only flag issues.</constraints>
<output_format>JSON: {clause, risk_level, explanation}</output_format>

User prompt:
<context>{{contract}}</context>
<task>Review for compliance risks.</task>
```

**6. For long context: data first, instructions last.**
Why: Supply all context first. Place instructions at the very end.
```
<context>
{{10,000 words of financial report}}
</context>

Based on the information above, identify the three highest-risk
investments and explain why.
```

**7. Use an anchor phrase after large context blocks.**
Why: A clear transition phrase bridges the context and your query.
```
<context>{{entire codebase}}</context>

Based on the codebase above, identify all functions that perform
database writes without transaction wrappers.
```

**8. Always include few-shot examples.**
Why: Prompts without examples are likely less effective. Examples can even replace instructions.
```
Classify sentiment:

Review: "Battery lasts forever!" → Sentiment: positive
Review: "Broke after two days." → Sentiment: negative
Review: "It's okay." → Sentiment: neutral
Review: "{{user_review}}" → Sentiment:
```

**9. Keep few-shot formatting perfectly consistent.**
Why: Inconsistent whitespace, tags, and casing cause undesired output formats.
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

**10. Use the completion strategy — start the response and let the model continue.**
Why: Starting the output pattern forces the model to follow it.
```
Order: Two burgers, a drink, and fries.
Output:
{"hamburger": 2, "drink": 1, "fries": 1}

Order: A cheeseburger and a coffee.
Output:
```

**11. Don't use vague negative constraints like "do not infer."**
Why: Broad negatives cause the model to over-index and fail at basic logic.
```
Bad:  "Do not infer. Do not guess."
Good: "Perform calculations based strictly on the provided text.
       Avoid using outside knowledge."
```

**12. Add a grounding clause when the model should only use provided context.**
Why: Prevents the model from using its own knowledge, improving factual accuracy.
```
"You are a strictly grounded assistant limited to the provided
context. Rely ONLY on directly mentioned facts. If the answer
is not explicitly in the context, state it's not available."
```

**13. Tell Gemini 3 the current year.**
Why: It doesn't always know it's 2026.
```
"Today's date is April 7, 2026. Your knowledge cutoff is
January 2025. For current events, always search.
Remember it is 2026."
```

**14. Prompt for explicit planning before action.**
Why: Gemini 3's thinking capabilities improve with structured planning prompts.
```
"Before providing the final answer:
 1. Parse the goal into distinct sub-tasks.
 2. Check if the input information is complete.
 3. Create a structured outline.
 Then execute."
```

**15. Add self-critique before final output.**
Why: Self-critique catches errors in tone, completeness, and constraint adherence.
```
"Before returning your response, review it:
 1. Did I answer intent, not just literal words?
 2. Is the tone right?
 3. Did I meet all output format requirements?"
```

**16. Break complex prompts into components.**
Why: One prompt per instruction. Chain prompts for sequential steps.
```python
facts = model.generate("Extract dates, names, amounts from: {{doc}}")
classification = model.generate(f"Classify by risk level: {facts}")
report = model.generate(f"Write risk report: {classification}")
```

**17. For agentic workflows: define reasoning, execution, and interaction behavior.**
Why: Complex agents require configuring the trade-off between cost and accuracy.
```xml
<reasoning>
Analyze constraints before acting. Use abductive reasoning.
Prioritize hypotheses by likelihood but don't discard long-shots.
</reasoning>
<execution>
Retry transient errors up to 3x. On other errors, change strategy.
Low-risk: proceed without asking. High-risk: confirm first.
</execution>
<interaction>
Only ask user for clarification when info is genuinely unavailable.
Default to reasonable assumptions and document them.
</interaction>
```

**18. Use Google's agentic system prompt template.**
Why: Evaluated by researchers to improve performance on agentic benchmarks.
```
"You are a very strong reasoner and planner. Before any action:

1. Analyze logical dependencies. Resolve by priority:
   policy rules > order of operations > prerequisites > user prefs.
2. Risk assessment: consequences of action? Low risk → proceed.
3. Abductive reasoning: look beyond obvious causes.
4. Adaptability: if observations contradict plan, pivot.
5. Use all sources: tools, policies, history, user.
6. Precision: quote exact policies when referring to them.
7. Completeness: don't conclude prematurely.
8. Persistence: don't give up. Retry transient errors.
   On other errors, change strategy.
9. Only act after all reasoning is complete."
```

**19. Specify constraints clearly.**
Why: Explicit constraints on length, format, and scope guide the model.
```
"Summarize in one sentence. Max 30 words.
Only facts from the source text. No opinions."
```

**20. If getting fallback responses, increase temperature (to default 1.0).**
Why: Fallback responses are triggered by safety filters. Higher temperature can help.
```python
response = model.generate(
    prompt=legitimate_question,
    temperature=1.0,
    safety_settings={"HARM_CATEGORY_DANGEROUS": "BLOCK_ONLY_HIGH"}
)
```

**21. When iterating: rephrase, switch to analogous tasks, or reorder content.**
Why: Different words yield different results. Analogous tasks can bypass instruction-following issues.
```
# If "Classify this book" gives verbose answers:
"Multiple choice: Which category describes The Odyssey?
 A) thriller  B) sci-fi  C) mythology  D) biography
Answer:"
```

**22. Use the Plan → Execute → Validate → Format template.**
Why: Google's recommended end-to-end template combining all best practices.
```xml
<!-- System instruction: -->
<role>Gemini 3, specialized for {{domain}}. Precise and persistent.</role>
<instructions>
1. Plan: Analyze task, create step-by-step plan.
2. Execute: Carry out the plan.
3. Validate: Review output against user's task.
4. Format: Present in requested structure.
</instructions>
<constraints>Verbosity: {{level}}. Tone: {{tone}}.</constraints>
<output_format>
1. Executive Summary
2. Detailed Response
</output_format>

<!-- User prompt: -->
<context>{{data}}</context>
<task>{{request}}</task>
<final_instruction>Think step-by-step before answering.</final_instruction>
```

---

*Built from official documentation by Anthropic, OpenAI, and Google. All checklist items trace directly to the source articles cited at the top of each section.*