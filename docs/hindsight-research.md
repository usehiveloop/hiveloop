## What Hindsight is good at

Hindsight is not just “chat history in a vector DB.” Its core model is: **retain → recall → reflect**. You push content into a memory bank with `retain()`, retrieve relevant facts with `recall()`, and use `reflect()` when you want Hindsight itself to synthesize an answer from memory. Hindsight’s docs say it uses multiple retrieval modes in parallel: semantic search, BM25 keyword search, graph traversal, and temporal reasoning, then reranks the result into a ranked list of structured facts. ([hindsight.vectorize.io][1])

For your Slack AI employee, the right mental model is:

> **Hindsight is the employee’s long-term company memory. Your app is the memory operating system.**

Your system decides what gets retained, which bank to write to, how to tag it, when to inject it, and when to delete/update it. Hindsight handles extraction, entity linking, recall, observations, and reflection.

---

# 1. Recommended architecture for your AI employee

Use **one primary Hindsight bank per company/workspace**, not one global bank for everyone.

```txt
company:{company_id}  → one Hindsight memory bank
```

Inside that bank, use tags to scope memory by team, source, user, project, channel, and visibility.

Example tags:

```txt
company:acme
team:engineering
team:product
project:billing-redesign
source:slack
source:github
source:gmail
channel:C12345
thread:1714920000.123
user:U123
visibility:company
visibility:team
visibility:private
memory_type:preference
memory_type:decision
memory_type:feedback
memory_type:policy
memory_type:company-context
```

This works because Hindsight banks are isolated containers, and tags are the recommended way to control visibility and filtering inside a shared bank. Hindsight’s docs explicitly say a memory bank is the unit of isolation, and that a shared bank with tags can work for cross-user analysis. ([hindsight.vectorize.io][2])

For your use case, I would use **three layers**:

```txt
1. Company bank
   Long-term shared business knowledge.

2. Team/project scoped memories inside the company bank
   Engineering standards, product decisions, customer issues, pivot reasons.

3. Optional private per-user bank or private tags
   User-specific preferences that should not be visible to everyone.
```

Do **not** mix different customer companies in one bank unless you are extremely careful with strict tag filtering. Hindsight warns that loose tag matching in multi-tenant banks can leak memories, and recommends strict matching for partitioned data. ([hindsight.vectorize.io][2])

---

# 2. What the AI employee should remember

You should not ask it to remember literally every token as “memory.” Instead, retain rich source content and configure Hindsight to extract durable facts.

For your company AI employee, the memory categories should be:

| Category               | Example                                                                             |
| ---------------------- | ----------------------------------------------------------------------------------- |
| Company identity       | “The company is building an email marketing platform for developers and marketers.” |
| Strategy               | “The company is pivoting from X to Y because enterprise buyers asked for Z.”        |
| Team members           | “Ada leads backend infra. Tunde owns deliverability.”                               |
| Product decisions      | “The team chose TiDB for event analytics because…”                                  |
| Engineering rules      | “Use server-side validation before sending transactional email.”                    |
| PR feedback            | “Reviewers repeatedly ask for smaller PRs and migration plans.”                     |
| Slack preferences      | “The bot should avoid posting non-urgent messages before 10am.”                     |
| Customer/user feedback | “Customers want import from Mailchimp before advanced automations.”                 |
| Recurring issues       | “Billing complaints usually come from failed card retries.”                         |
| Bot behavior feedback  | “Engineering prefers short, actionable bot replies in standup channels.”            |

Hindsight’s best-practice docs say your `retain_mission` should explicitly list what to extract and what to ignore. Vague missions like “extract all information” are called out as bad because they create noisy memories. ([hindsight.vectorize.io][2])

---

# 3. Bank configuration I would use

Configure the company bank before ingestion. Hindsight says bank configuration should happen before first use because missions steer memory behavior. ([hindsight.vectorize.io][2])

### `retain_mission`

Use this to control what facts are extracted from Slack, PRs, emails, docs, tickets, etc.

```txt
You are retaining memory for an AI employee embedded in the company.

Extract durable company knowledge, including:
- company identity, strategy, pivots, goals, constraints, and reasons for decisions
- team members, roles, ownership areas, working preferences, and communication norms
- product decisions, technical decisions, architectural trade-offs, risks, blockers, and incidents
- user/customer feedback, repeated complaints, feature requests, and sentiment
- explicit feedback about the AI employee’s behavior
- policies, recurring workflows, deadlines, commitments, and decisions

Ignore:
- greetings, jokes, filler, reactions without substance, temporary chit-chat
- transient scheduling logistics unless they affect durable working preferences
- one-off emotional reactions unless they are explicit feedback or recurring pattern evidence

Preserve who said it, when it was said, where it came from, and why it matters.
```

### `observations_mission`

Observations are important because Hindsight consolidates repeated evidence into durable learnings. The docs say observations are deduplicated, evidence-grounded, track proof count and freshness trend, and are refined rather than overwritten. ([hindsight.vectorize.io][3])

```txt
Identify durable business patterns, evolving preferences, recurring feedback, strategic shifts,
team communication norms, repeated product/customer issues, and contradictions with earlier knowledge.

Prefer stable patterns over one-off events.
When new evidence contradicts earlier knowledge, preserve the historical transition:
what changed, when it changed, who said it, and why.
```

### `reflect_mission`

This shapes how the AI employee reasons when using `reflect()`.

```txt
You are the company’s AI employee. You remember company context, people, projects,
decisions, feedback, and communication norms.

You should help the team by being concise, context-aware, and useful.
Before answering, consider company strategy, team preferences, active projects,
past decisions, and prior feedback about your behavior.

Do not pretend to know things that are not in memory.
When memory is uncertain or conflicting, say so.
```

### Disposition

For this kind of employee, I would set:

```txt
skepticism: 4
literalism: 4
empathy: 3
```

Reason: it should question stale/conflicting memories, read people’s feedback fairly literally, and still be socially aware. Hindsight dispositions affect `reflect`, not raw `recall`. ([hindsight.vectorize.io][2])

---

# 4. Entity labels to add

Use Hindsight `entity_labels` so memories can be classified consistently. Hindsight supports controlled vocabulary labels and can optionally turn labels into tags for filtering. ([hindsight.vectorize.io][4])

I would define labels like:

```json
{
  "entity_labels": [
    {
      "key": "memory_type",
      "description": "The kind of durable company memory",
      "type": "value",
      "tag": true,
      "optional": false,
      "values": [
        { "value": "company_identity", "description": "Company mission, positioning, business model, identity" },
        { "value": "strategy", "description": "Strategic decision, pivot, market reasoning, business constraint" },
        { "value": "team_member", "description": "Person, role, responsibility, ownership area" },
        { "value": "working_preference", "description": "Communication style, timing, meeting, writing, or collaboration preference" },
        { "value": "bot_feedback", "description": "Explicit feedback about how the AI employee should behave" },
        { "value": "technical_decision", "description": "Architecture, stack, API, infra, security, or product engineering decision" },
        { "value": "product_feedback", "description": "Customer/user feedback, feature request, complaint, or sentiment" },
        { "value": "policy", "description": "Company rule, team convention, process, or standard" },
        { "value": "risk", "description": "Risk, blocker, incident, concern, or unresolved issue" }
      ]
    },
    {
      "key": "source_type",
      "description": "Where this memory came from",
      "type": "value",
      "tag": true,
      "optional": false,
      "values": [
        { "value": "slack", "description": "Slack channel, DM, or thread" },
        { "value": "github_pr", "description": "GitHub pull request, review, comment, issue, or commit" },
        { "value": "email", "description": "Email thread or message" },
        { "value": "document", "description": "Internal doc, handbook, meeting notes, roadmap, spec" },
        { "value": "ticket", "description": "Linear/Jira/support ticket" }
      ]
    },
    {
      "key": "durability",
      "description": "How long this memory is likely to remain useful",
      "type": "value",
      "tag": true,
      "optional": true,
      "values": [
        { "value": "permanent", "description": "Should remain true unless explicitly changed" },
        { "value": "long_lived", "description": "Likely useful for months" },
        { "value": "project_lived", "description": "Useful while a project is active" },
        { "value": "temporary", "description": "Useful briefly; avoid making broad observations from it" }
      ]
    }
  ]
}
```

This lets you recall only operating rules, only bot feedback, only strategy memories, only engineering memories, etc.

---

# 5. Ingestion design by source

## Slack

For every Slack thread where the bot participates or is mentioned:

```txt
document_id = slack:{workspace_id}:{channel_id}:{thread_ts}
tags = [
  company:{company_id},
  source:slack,
  channel:{channel_id},
  thread:{thread_ts},
  team:{team_id},
  visibility:team or visibility:company
]
metadata = {
  source: "slack",
  workspace_id,
  channel_id,
  channel_name,
  thread_ts,
  permalink,
  participants,
  captured_at
}
timestamp = first_message_timestamp or current message timestamp
context = "Slack thread in #engineering about billing redesign"
```

Hindsight recommends retaining a full conversation as one item when possible, clearly preserving who said what and when. It also recommends using a stable `document_id`, because random IDs create duplicate memories. ([hindsight.vectorize.io][5])

For growing Slack threads, use one of these patterns:

```txt
Option A: retain full updated thread with same document_id
Option B: append only new messages with update_mode="append"
```

Hindsight supports `append` mode for growing content like chat transcripts, and says it avoids reprocessing unchanged chunks. ([hindsight.vectorize.io][5])

## GitHub PR reviews

For each PR:

```txt
document_id = github:{org}:{repo}:pr:{number}
tags = [
  company:{company_id},
  source:github,
  source_type:github_pr,
  repo:{repo},
  project:{project_id},
  team:engineering,
  memory_type:technical_decision,
  visibility:team
]
context = "GitHub PR review for repo X, PR #123, including review comments and author replies"
```

Retain:

* PR title/body
* review comments
* requested changes
* merge decision
* important author replies
* linked issue
* final outcome

This lets the bot learn things like: “Backend reviewers prefer migration plans in PR descriptions” or “The team rejects schema changes without rollback notes.”

## Emails

Use email ingestion for durable business context only.

```txt
document_id = email:{thread_id}
tags = [
  company:{company_id},
  source:email,
  user:{sender_id},
  topic:sales or topic:support or topic:partnership,
  visibility:restricted
]
context = "Customer email thread about onboarding blockers and pricing objection"
```

Do not put all emails into company-visible memory. Many emails are private, sensitive, or irrelevant. Use visibility tags and strict filtering.

## Internal docs / handbook / roadmap

Use document IDs that preserve versioning:

```txt
document_id = doc:{doc_id}:v:{version}
```

For living documents, you have two choices:

* Use same `document_id` to replace/update the document.
* Use versioned IDs to preserve history.

Hindsight says re-retaining with the same `document_id` replaces the old document and its memories. That is useful for “current source of truth,” but not for “never forget the historical pivot.” ([hindsight.vectorize.io][6])

For strategy/pivots, I would **always retain an event memory separately**:

```txt
document_id = event:company-pivot:2026-05-11
content = "The company decided to pivot from X to Y because A, B, and C..."
```

That way updating the current roadmap does not erase the historical reason.

---

# 6. “Never forget unless explicit” design

Do not rely on one mechanism. Use four safeguards:

### 1. Keep source systems as ground truth

Hindsight is your memory layer, but Slack/GitHub/email/docs remain your evidence layer. Hindsight documents and chunks help trace source content, but your app should also store source URLs/permalinks in metadata for audit and rehydration. Hindsight’s docs say documents help track sources, update content, delete memories in bulk, and organize facts by source; chunks preserve original text segments for context. ([hindsight.vectorize.io][6])

### 2. Avoid accidental replacement

Never use random `document_id`s for the same thread, and never replace a historical document unless you mean to. Hindsight warns that random document IDs create duplicates, while same document ID performs upsert/delete-then-reprocess. ([hindsight.vectorize.io][2])

Use:

```txt
Slack growing thread → append mode or full-thread upsert
Strategy decision → immutable event document
Current policy doc → replace mode
Historical policy version → versioned document_id
```

### 3. Make deletion explicit and audited

Implement an internal “memory deletion request” workflow:

```txt
User says: "Forget that we are pivoting because of X"
System:
1. classify as deletion/update request
2. ask for confirmation if broad
3. locate relevant memories/documents
4. delete or supersede them
5. retain a non-sensitive audit note: "A memory correction was made on DATE by USER"
```

Do not give the Slack agent broad `clear_memories` or `delete_bank` access. Keep destructive memory tools admin-only.

### 4. Handle corrections as new evidence

If someone says:

> “Actually, we are not pivoting because of pricing. We are pivoting because onboarding is too complex.”

Retain that as a correction with `memory_type:strategy` and `memory_type:correction`. Hindsight observations are designed to evolve when new evidence supports, contradicts, or extends prior knowledge. ([hindsight.vectorize.io][3])

---

# 7. Auto memory injection at the start of every Slack thread

At the start of every Slack interaction, your system should do **pre-response retrieval**.

Do not simply dump all memories into the prompt. Build a memory preloader.

## Step 1: Resolve context

From Slack event:

```ts
const ctx = {
  companyId,
  workspaceId,
  channelId,
  channelName,
  threadTs,
  userId,
  teamId,
  projectId,
  participants,
  messageText,
  timestamp
};
```

## Step 2: Build tag filters

For a team thread:

```txt
company:{company_id}
team:{team_id}
channel:{channel_id}
visibility:company OR visibility:team
```

For a private DM:

```txt
company:{company_id}
user:{user_id}
visibility:private OR visibility:company
```

Use strict tag matching where leakage is unacceptable. Hindsight documents `any_strict` and `all_strict` tag matching modes and recommends strict modes for fully partitioned memory. ([hindsight.vectorize.io][2])

## Step 3: Retrieve mental models first

Create mental models for common startup context:

```txt
company-profile
team-directory
team-communication-preferences
active-projects
engineering-standards
product-strategy
bot-behavior-rules
customer-feedback-themes
```

Mental models are precomputed reflect responses. Hindsight says they are checked first during reflect and are useful for repeated queries, high-traffic agents, user profiles, and slowly changing state. ([hindsight.vectorize.io][7])

## Step 4: Recall relevant facts

Run 2–4 recall queries, not one giant query.

Example:

```txt
1. "Relevant company, project, and team context for this Slack thread"
2. "Known preferences and communication rules for this channel and participants"
3. "Prior decisions, blockers, and unresolved issues related to: {messageText}"
4. "Explicit feedback about how the AI employee should behave in this context"
```

Use:

* `budget: low` for simple thread startup
* `budget: mid` for normal responses
* `budget: high` only for deep analysis

Hindsight recommends `mid` as the default and reserving `high` for deep recall. ([hindsight.vectorize.io][2])

## Step 5: Inject memory as a system block

Example:

```txt
Relevant long-term memory:

Company identity:
- The company is building...

Team norms:
- Engineering prefers short replies in morning standup.
- The bot should avoid non-urgent messages before 10am.

Project context:
- Billing redesign is blocked by migration risk.
- The team chose Prisma for app-level data access but raw SQL for heavy analytics.

Bot behavior:
- In #engineering, only respond when mentioned unless the thread contains a direct question.
```

Then add:

```txt
Use these memories as context, not absolute truth.
If the current thread contradicts memory, prefer current explicit instructions and retain the correction.
```

Hindsight’s Vercel Chat SDK integration exposes a `memoriesAsSystemPrompt()` helper for this kind of prompt injection, and its docs show auto-recall and auto-retain patterns for chat handlers. ([hindsight.vectorize.io][8])

---

# 8. What tools to give the agent

Give the Slack agent **safe operational tools** and keep destructive/admin tools in your backend.

## Agent-facing tools

### Memory tools

Give:

```txt
retain
recall
reflect
getMentalModel
getDocument
```

Hindsight’s AI SDK integration registers these five tools, with the app controlling infrastructure concerns like tags, metadata, budgets, and async behavior. ([hindsight.vectorize.io][9])

Use `retain` when:

* user states a durable preference
* someone gives feedback about the bot
* a decision is made
* a project constraint/blocker is discussed
* customer feedback appears
* a team member’s role/ownership changes

Use `recall` when:

* generating a response that depends on past context
* entering a new Slack thread
* answering “what did we decide?”
* checking behavior preferences

Use `reflect` when:

* synthesizing strategy
* summarizing patterns
* deciding what to recommend
* answering “why are we doing this?”
* producing structured insight from many memories

### Slack tools

Give:

```txt
read_thread
read_channel_context
post_reply
post_ephemeral
react_to_message
get_permalink
```

Avoid giving it broad “read all Slack history” unless your system scopes and filters what it can access.

### GitHub tools

Give:

```txt
read_pr
read_pr_comments
read_issue
read_file
search_repo
```

Maybe give `comment_on_pr`, but only with clear product rules.

### Email tools

For an AI employee, I would **not** give free-form email access by default. Give:

```txt
search_allowed_threads
read_allowed_thread
draft_reply
```

Sending emails should require confirmation.

### Admin-only memory tools

Keep these away from the agent:

```txt
delete_bank
clear_memories
delete_document
update_bank
create_directive
delete_directive
list_banks
```

Hindsight’s MCP docs list both normal tools and powerful bank-management tools; in production, expose only what the agent actually needs. Hindsight supports per-bank MCP endpoints and an `mcp_enabled_tools` allowlist. ([hindsight.vectorize.io][10])

---

# 9. What your system should do automatically

Your backend should be responsible for:

## A. Identity resolution

Map all identities:

```txt
Slack user U123 → person: Ada Okafor
GitHub user ada-dev → person: Ada Okafor
Email ada@company.com → person: Ada Okafor
```

Pass known entities into retain when possible. Hindsight supports providing entities explicitly so they are recognized and linked in the graph. ([hindsight.vectorize.io][5])

## B. Source normalization

Convert Slack, PRs, emails, and docs into structured text/JSON with:

* who said it
* where it happened
* when it happened
* source URL
* participants
* project/team
* final outcome if known

Hindsight recommends rich representations and warns against pre-summarizing because summaries lose entity relationships, temporal markers, and structure. ([hindsight.vectorize.io][2])

## C. Memory routing

Decide:

```txt
Which bank?
Which tags?
Which visibility?
Which document_id?
Replace or append?
Sync or async?
```

Hindsight recommends async retain for end-of-turn/session ingestion where latency matters, and warns not to retain and recall in the same turn because writes may not be available immediately. ([hindsight.vectorize.io][2])

## D. Memory quality control

Add a “memory candidate classifier” before retain:

```txt
Should remember? yes/no
Memory category
Visibility
Durability
Confidence
Reason
```

You can let the LLM classify, but your app should enforce hard rules.

## E. Mental model refresh

Use automatic refresh for high-value models:

```txt
company-profile
team-directory
communication-policy
bot-behavior-rules
active-projects
engineering-standards
customer-feedback-themes
```

Hindsight mental models can auto-refresh after observation consolidation, and webhooks can notify your app when consolidation completes. ([hindsight.vectorize.io][7])

---

# 10. Example memory flow

User says in Slack:

> “Can the bot stop posting long updates before 10am? It’s noisy during morning standup.”

Your system should retain:

```json
{
  "bank_id": "company:acme",
  "content": [
    {
      "role": "user",
      "name": "Ada Okafor",
      "content": "Can the bot stop posting long updates before 10am? It’s noisy during morning standup.",
      "timestamp": "2026-05-11T08:42:00Z"
    }
  ],
  "context": "Slack feedback in #engineering about AI employee behavior during morning standup",
  "timestamp": "2026-05-11T08:42:00Z",
  "document_id": "slack:T123:C456:1715416920.000",
  "tags": [
    "company:acme",
    "source:slack",
    "team:engineering",
    "channel:C456",
    "memory_type:bot_feedback",
    "memory_type:working_preference",
    "visibility:team"
  ],
  "metadata": {
    "slack_permalink": "https://...",
    "user_id": "U123",
    "channel_name": "engineering"
  }
}
```

Then at future thread startup, recall:

```txt
Query: "AI employee behavior preferences for #engineering, morning standup, posting frequency, response length"
Tags: ["company:acme", "team:engineering", "memory_type:bot_feedback"]
Tags match: all_strict or strict equivalent
```

Injected memory:

```txt
Bot behavior rule:
- Engineering feedback says long bot updates before 10am are noisy during morning standup.
- In morning engineering contexts, prefer short replies and avoid proactive non-urgent updates.
```

---

# 11. Example: company pivot memory

When leadership says:

> “We are pivoting from generic email marketing to developer-first transactional + marketing email because teams want one API, SES is too low-level, and Mailchimp is too bloated.”

Retain it as an immutable strategy event:

```json
{
  "document_id": "event:strategy:pivot:2026-05-11",
  "context": "Company strategy decision from leadership Slack thread",
  "timestamp": "2026-05-11T12:00:00Z",
  "tags": [
    "company:acme",
    "memory_type:strategy",
    "memory_type:company_identity",
    "visibility:company",
    "durability:permanent"
  ],
  "content": "Leadership decided to pivot from generic email marketing to developer-first transactional plus marketing email. Reasons: teams want one API, SES is too low-level, and Mailchimp is too bloated."
}
```

Also update the mental model:

```txt
company-profile:
"What is the company, what is it building, who is it for, and what strategic pivots explain its current direction?"
```

---

# 12. Production checklist

1. **One Hindsight bank per company.**
2. **Strict tags for team/user/private visibility.**
3. **Stable document IDs for Slack threads, PRs, emails, and docs.**
4. **Append mode for growing Slack/email threads.**
5. **Immutable event memories for pivots, major decisions, incidents, and policy changes.**
6. **Rich raw conversation format; do not pre-summarize before retain.**
7. **Always set `context`, `timestamp`, `tags`, `metadata`, and `document_id`.**
8. **Use mental models for startup context and repeated queries.**
9. **Auto-recall at the beginning of every Slack thread.**
10. **Retain at the end of turns/sessions, not immediately before recall.**
11. **Keep destructive memory tools admin-only.**
12. **Audit memory changes and corrections.**
13. **Use webhooks/operations to refresh mental models after consolidation.**
14. **Monitor retain/recall/reflect latency, token usage, and failures. Hindsight exposes Prometheus metrics and Grafana dashboards for this.** ([hindsight.vectorize.io][11])

---

# 13. Final recommendation

For your AI employee, I would implement Hindsight like this:

```txt
Company brain:
  bank_id = company:{company_id}

Startup context:
  mental models + recall relevant facts per thread

Long-term learning:
  async retain at end of Slack threads, PR reviews, email threads, docs, tickets

Visibility:
  company/team/user/source/project tags with strict matching

Historical memory:
  immutable event memories for decisions, pivots, feedback, incidents

Behavior adaptation:
  dedicated bot_feedback + working_preference memories

Admin safety:
  deletion/update tools only in backend, never directly exposed to the agent
```

The biggest mistake would be letting the bot randomly call `retain()` on tiny snippets with weak context. The best version is: **your backend curates the memory envelope, Hindsight extracts and consolidates, and the agent recalls/reflects only inside the correct company/team/thread scope.**

[1]: https://hindsight.vectorize.io/ "Overview | Hindsight"
[2]: https://hindsight.vectorize.io/best-practices "Best Practices | Hindsight"
[3]: https://hindsight.vectorize.io/developer/observations "Observations: Knowledge Consolidation | Hindsight"
[4]: https://hindsight.vectorize.io/developer/api/memory-banks "Memory Banks | Hindsight"
[5]: https://hindsight.vectorize.io/developer/api/retain "Ingest Data | Hindsight"
[6]: https://hindsight.vectorize.io/developer/api/documents "Documents | Hindsight"
[7]: https://hindsight.vectorize.io/developer/api/mental-models "Mental Models | Hindsight"
[8]: https://hindsight.vectorize.io/sdks/integrations/chat "Vercel Chat SDK Persistent Memory with Hindsight | Integration | Hindsight"
[9]: https://hindsight.vectorize.io/sdks/integrations/ai-sdk "Vercel AI SDK Persistent Memory with Hindsight | Integration | Hindsight"
[10]: https://hindsight.vectorize.io/developer/mcp-server "MCP Server | Hindsight"
[11]: https://hindsight.vectorize.io/developer/monitoring "Monitoring | Hindsight"
