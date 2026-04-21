# 01 — Agent Design Principles

Before writing a single trigger, decide what the agent is. This sounds obvious, but the most common cause of agents that drift, loop, confuse users, or produce embarrassing output is that nobody wrote down what the agent was supposed to do in one sentence. This doc is about how to form that one sentence, and how to use it to make every subsequent decision easier.

## The one-sentence test

You should be able to complete this sentence in fewer than 25 words:

> "This agent exists to _______, and it stops working when _______."

Examples that pass the test:

- "This agent exists to review new pull requests for style violations, and it stops working when the PR is merged or closed."
- "This agent exists to triage new bug reports by labeling and assigning them, and it stops working when the bug is labeled."
- "This agent exists to draft replies to customer support conversations, and it stops working when a human admin sends a reply."
- "This agent exists to respond to failed CI runs on its own PRs, and it stops working when the PR merges."

Examples that fail the test:

- "This agent handles GitHub stuff." (too broad — what is "handles"? what is "stuff"?)
- "This agent helps developers be more productive." (too abstract — you can't build that)
- "This agent responds to events in Linear and Slack and GitHub and Jira..." (multi-purpose — split it)
- "This agent does everything a project manager does." (impossible scope)

If you can't write the sentence, you're not ready to configure the trigger. Go figure out the sentence first.

## Single responsibility

Every agent should have one primary job. "One primary job" means: a user asked for exactly one thing, and the agent does exactly that thing, across whatever events are relevant to it. Agents that try to do many things at once produce worse output, are harder to debug, and are more likely to loop.

### Signs you've violated single-responsibility

- Your agent's system prompt is longer than 3 paragraphs
- Your agent has triggers for 4+ different event types with unrelated purposes
- Your instructions block has nested conditional language ("if this is a PR review, do X, otherwise if it's a push, do Y, otherwise...")
- Your context actions fetch data for scenarios the current event doesn't care about
- You have a `terminate_on` rule that fires on an event the agent has no business caring about

If you see these signs, you probably want to split the agent into two or three smaller ones, each with a single job. See [07-worked-examples.md](07-worked-examples.md) for how this looks in practice.

### Single responsibility doesn't mean single event

An agent with a single responsibility can still react to multiple events, as long as they're all in service of the same job. A code-review agent reacts to:

- `pull_request.opened` — to do the initial review
- `pull_request.synchronize` — to re-review after new commits
- `pull_request_review_comment.created` — to respond to reviewer comments
- `pull_request.closed` — to close out the conversation

That's four events, one job. The unifying theme is "react to anything that happens on a PR I'm reviewing." This is fine — in fact, it's the design the trigger system was built for.

What's NOT fine is an agent that reacts to `pull_request.opened` AND `issue.created` AND `release.published`, because those are three different jobs happening on three different resource types.

## Scope boundaries

Every agent should have explicit scope boundaries that it refuses to cross. Write these down in the system prompt. Examples:

- "Only respond to issues labeled `bug` or `triage-needed`."
- "Only review PRs where the head branch starts with `feature/`."
- "Only act on conversations assigned to the hiveloop team."
- "Only handle events in the `engineering` org's connections."
- "Never modify production database rows, even if asked."
- "Never approve or merge a pull request; always leave that to a human."

Boundaries serve two purposes. First, they make the agent's behavior predictable — users learn what to expect. Second, they give the agent a clear thing to refuse when an edge case arrives that wasn't anticipated. "This PR is outside my scope; I'm not going to comment on it" is a useful response, and the agent needs permission to say it.

Boundaries should live in the system prompt (permanent) and optionally in trigger conditions (enforced at the dispatcher layer). Conditions are stricter — they prevent the agent from even seeing the event. Prompt-level boundaries are softer — the agent sees the event but decides not to act. Use conditions for hard rules (branch prefixes, label filters), prompt-level for soft judgment calls (tone, format).

## When to create a new agent vs. extend an existing one

### Create a new agent when:

- **The job is fundamentally different.** A code reviewer and a bug triager are different jobs even if they live in the same repo. Don't mash them together.
- **The source is different and you don't need cross-source continuation.** A Linear-initiated agent and a Slack-initiated agent should be separate, even if they do the same downstream work. See [multi-provider patterns](../06-multi-provider-patterns.md).
- **The permissions or access scope is different.** An agent that needs write access to production is a different agent from one that only reads, even if their jobs overlap. Keep the write-capable one on a tight leash with fewer triggers.
- **The tone or output format is different.** A public-facing PR review bot needs different prompting from an internal notes-taker. Different agents, even if both trigger on the same events.
- **The failure modes are different.** An agent that might post customer-visible content should fail safer than one that only posts internal comments.

### Extend an existing agent when:

- **The new event is a natural continuation of the existing job.** Adding `pull_request.synchronize` to a PR reviewer that already handles `pull_request.opened` is a natural extension, not a new agent.
- **The scope expansion is small and well-defined.** A bug triage agent that starts triaging feature requests too is fine, as long as the system prompt covers both clearly.
- **The behavior is identical, just on a different filter.** Adding a condition to match a new label isn't a new agent.

### The default should be "new agent"

When in doubt, create a new agent. Two small, focused agents are almost always easier to reason about, debug, and modify than one large agent that tries to do both jobs. The cost of a new agent row in the database is zero; the cost of untangling a bloated agent is real. Err toward splitting.

## The cost of a new agent

Because "new agent" is the default recommendation, it's worth being explicit about what new agents actually cost:

- **One row in the `agents` table.** Free.
- **A few rows in `agent_triggers`.** Free.
- **A system prompt to write.** Maybe 30 minutes if you're careful about scope boundaries.
- **A sandbox.** Shared agents reuse the pool sandbox; dedicated agents get their own (sleeping when idle — see [05-economics-and-performance.md](05-economics-and-performance.md)).
- **Configuration maintenance.** When the job changes, you update N agents instead of 1. For most cases N is 1 or 2, so this is marginal.

The real cost is operational complexity — having more agents to monitor and reason about. This is non-trivial at scale, but it's still usually cheaper than a tangled super-agent.

## Thinking about state

Agents look stateful from the user's perspective — they remember prior conversations, pick up where they left off, respond to follow-ups. That's the continuation feature working correctly. But under the hood, the agent's state lives in three places, and it helps to know which is which:

1. **The conversation history in Bridge.** Every message the agent has sent or received on this resource lives here. When continuation fires (e.g., a new comment on an open PR), the agent picks up where it left off because the executor routes the new event into the existing Bridge conversation. See [03-lifecycle-and-state-management.md](03-lifecycle-and-state-management.md) for details.
2. **The agent's memory** (if `SharedMemory: true` on the agent). Long-lived facts that persist across conversations, stored in the project's memory system. "I reviewed PR #42 last week and the author didn't like when I nitpicked whitespace" — that kind of fact lives here.
3. **The webhook payload.** Every event carries the current state of the subject resource. The agent doesn't need to remember what the PR's files were on the last event; it can re-read them from the fresh payload or fetch them via a context action.

When designing an agent, decide which of these three your agent relies on:

- **Payload-only agents** are the simplest — every event is handled fresh based only on the current payload. These are robust but can't learn from past interactions.
- **Conversation-continuation agents** carry context forward within a single resource's lifecycle. This is the default for most agents.
- **Memory-using agents** carry facts across resources and across time. Only use memory when the job genuinely benefits from long-term learning — e.g., a code reviewer that remembers team conventions from past reviews.

Most agents should be payload + conversation-continuation. Memory is powerful but adds a complication: memory can be wrong, stale, or contradict the current payload. Agents that consult memory need explicit handling for "the memory says X but the current event says Y" — design for that conflict or it will surprise you.

## What an agent is NOT

Two common confusions worth naming:

### An agent is not a workflow engine

If your "agent" is really just "when X happens, do Y; when Y happens, do Z; when Z happens, do W," that's a workflow, not an agent. Workflows are deterministic sequences of steps with defined inputs and outputs. Agents are LLM-powered reasoners that make judgment calls within a bounded scope. If you find yourself writing if/then/else logic in the instructions block, you probably want a workflow, not an agent.

Agents shine when there's genuine ambiguity that benefits from LLM reasoning: writing natural-language responses, interpreting unstructured text, summarizing, deciding on priorities, drafting code fixes. Workflows shine when there's a clear procedure to execute.

### An agent is not a chatbot

A chatbot reacts to every user utterance and tries to maintain conversational coherence. A trigger-driven agent reacts to specific events that the customer has explicitly wired up. They look similar in the Bridge UI (both produce conversations, both take messages), but the design mindset is different:

- A chatbot's job is to respond to whatever comes in.
- An agent's job is to perform a specific task when specific events happen.

This matters for scope. Chatbots are expected to handle arbitrary topics; agents are expected to stay in their lane. An agent that happily responds to "what's the weather" because the user mentioned it in a PR comment is misbehaving, not being helpful. Its scope is PR review, not weather reports.

## Putting it all together

Before writing your first trigger:

1. Write the one-sentence statement. If you can't, stop and think more.
2. Write down the scope boundaries. Hard rules go in conditions; soft rules go in the system prompt.
3. Decide whether this agent exists or needs to be created. If there's any doubt, create new.
4. Decide which state layer(s) the agent uses: payload-only, continuation, memory.
5. Decide which events are in scope. If the list goes past 5, reconsider scope.

Once you've done this, writing the trigger configuration is the easy part. See [02-trigger-configuration-strategy.md](02-trigger-configuration-strategy.md) for how to translate these decisions into YAML.

## Where to go from here

- The mechanics of writing a good trigger: [02-trigger-configuration-strategy.md](02-trigger-configuration-strategy.md)
- How to think about conversation lifecycle: [03-lifecycle-and-state-management.md](03-lifecycle-and-state-management.md)
- The failure mode to worry about most: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- Complete examples of well-scoped agents: [07-worked-examples.md](07-worked-examples.md)
