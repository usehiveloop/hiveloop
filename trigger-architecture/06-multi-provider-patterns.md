# 06 — Multi-Provider Patterns

Agents routinely need to respond to events from more than one provider. A coding agent might pick up tasks from Linear OR Slack, and also need to respond to GitHub PR reviews on PRs it opens. A customer-support agent might get pinged from Intercom AND Slack. A security agent might watch GitHub pushes AND CI results. This document covers how multi-provider agents are supported today, what the intentional workaround is for cross-provider correlation, and the design discussion that led to the current shape.

## What's supported today

**Multiple triggers per agent, each bound to one connection.** The schema already allows this: an agent's `integrations` field lists every connection it has access to, and the `agent_triggers` table can hold any number of rows per agent. Each row binds to exactly one connection (and therefore one provider) with its own trigger keys, conditions, context actions, and instructions.

This means a single agent can react to events from Slack, Linear, and GitHub by having three (or more) AgentTrigger rows. Each row fires independently when its event arrives. The agent's system prompt is what makes behavior coherent across sources — one brain, multiple eyes.

## The engineering team's coding agent, concretely

The use case that drove this discussion: an autonomous coding agent that picks up tasks and opens PRs to complete them. Here's how you'd wire it.

### Agent `zira-linear-worker`

One agent row. Four AgentTrigger rows, all pointing at this agent:

**Trigger row 1 — pick up work from Linear**
```yaml
connection_id: <linear-connection-uuid>
trigger_keys:
  - issue.assigned
  - issue.created
conditions:
  match: any
  rules:
    - path: data.assignee.email
      operator: equals
      value: zira-bot@company.com
    - path: data.labels.*.name
      operator: one_of
      value: [zira, automated]
context:
  - as: issue
    action: get_issue
    params:
      id: $refs.issue_id
  - as: comments
    action: list_comments
    params:
      issue_id: $refs.issue_id
instructions: |
  You've been assigned Linear issue $refs.issue_identifier.
  {{$issue.title}}
  {{$issue.description}}
  Comments so far: {{$comments}}
  Read the issue, plan the work, and begin implementation per your system
  prompt. Post a status comment on the Linear issue acknowledging the
  assignment before you start.
```

**Trigger row 2 — respond to reviews on this agent's PRs**
```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - pull_request_review.submitted
  - pull_request_review_comment.created
  - issue_comment.created
conditions:
  match: all
  rules:
    - path: pull_request.head.ref
      operator: matches
      value: "^zira/linear/"
    - path: sender.login
      operator: not_equals
      value: zira-bot[bot]
context:
  - as: pr
    action: pulls_get
    ref: pull_request
  - as: files
    action: pulls_list_files
    ref: pull_request
  - as: review_comments
    action: pulls_list_review_comments
    ref: pull_request
instructions: |
  A reviewer left feedback on PR #$refs.pull_number ($refs.repository).
  Read the review, decide how to address it, push follow-up commits to
  the PR branch, and reply in the review thread.
```

**Trigger row 3 — fix broken CI**
```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - workflow_run.completed
  - check_run.completed
conditions:
  match: all
  rules:
    - path: workflow_run.conclusion
      operator: one_of
      value: [failure, cancelled, timed_out]
    - path: workflow_run.head_branch
      operator: matches
      value: "^zira/linear/"
context:
  - as: run
    action: actions_get_workflow_run
    ref: workflow
instructions: |
  CI failed on $refs.workflow_name for a branch you own. Inspect the
  failure, identify the root cause, push a fix, and post a comment on
  the associated PR explaining what went wrong.
```

**Trigger row 4 — close the PR conversation on merge**
```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - pull_request.opened  # placeholder so the terminate column has a row
conditions:
  match: all
  rules:
    - path: pull_request.head.ref
      operator: matches
      value: "^zira/linear/"
terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: true
    context:
      - as: final
        action: pulls_get
        ref: pull_request
    instructions: |
      PR #$refs.pull_number was merged. Post a short summary comment
      thanking the reviewer and noting anything worth remembering.
```

### Agent `zira-slack-worker`

**The same four trigger rows, but swap Trigger row 1 to listen on Slack mentions instead of Linear issue assignments.** Rows 2, 3, and 4 are identical except the branch filter uses `^zira/slack/` instead of `^zira/linear/`.

Both agents share the same system prompt, same sandbox template, same tools, same model. The only differences are the entry trigger and the branch prefix. This is "two agents with shared config" — the pattern we settled on.

### Why branch prefixes instead of bot identity

The GitHub triggers need to filter to "only PRs this agent created." Two clean approaches:

1. **Branch prefix** (recommended): each agent pushes to a namespaced prefix. `zira/linear/fix-login-bug` vs `zira/slack/add-oauth-flow`. No shared bot identity needed — the filter is just a regex on `pull_request.head.ref`.
2. **PR label**: each agent tags its PRs with `agent:linear-worker` or `agent:slack-worker`. Filter by `pull_request.labels.*.name one_of [agent:linear-worker]`. Requires the agent to manage labels, which is extra work.

Branch prefix wins because it's zero-maintenance — the agent already has to choose a branch name, and adding a prefix is free. Labels are fine too if there's a specific reason to prefer them.

### What works cleanly

- **Within-source continuation works**. A Linear issue opened → assigned → commented → updated all continue the same conversation via `linear:issue:$refs.issue_id` (once the Linear resource gets a template added to its catalog entry). A PR opened by the agent → reviewed → more commits → closed all continue the same `github:owner/repo#pr-42` conversation, closing gracefully via the terminate rule.
- **Cross-source isolation is correct**. The Linear agent and the Slack agent don't interfere. A Slack mention triggers only the Slack agent. A Linear assignment triggers only the Linear agent. PR events filter by branch prefix so each agent only sees its own PRs.
- **Per-agent conversation threading**. Each agent has its own conversations scoped to `(agent_id, connection_id, resource_key)` — two agents on the same PR each have their own thread with the reviewers. No crossed wires.
- **Validation scales**. Each AgentTrigger row is independently validated by the handler. The Linear trigger can only reference Linear actions (the validator checks `provider = linear`), the GitHub trigger can only reference GitHub actions. Typos and cross-provider mistakes are caught at save time.
- **System prompt unification**. The agent's system prompt tells it how to orient on any source — "if you see a Linear assignment, plan the work; if you see a Slack mention, read the thread for context; if you see a PR review, address feedback." One prompt, uniform behavior.

## What you give up

One thing: **the Linear conversation and the PR conversation are separate threads for the same agent.**

Walk through the sequence:

1. `issue.assigned` on Linear → trigger row 1 fires → new conversation keyed by `linear:issue:LIN-123`
2. Agent plans, implements, creates PR `octocat/repo#pr-42` with "Fixes LIN-123" in the description
3. Reviewer leaves a review → trigger row 2 fires → `ResourceKey = octocat/repo#pr-42`
4. Executor looks up existing conversation for `(agent_id, github-conn-id, octocat/repo#pr-42)` → none found → creates a new conversation

The Linear conversation and the PR conversation never merge. The agent's PR-review conversation starts fresh, without the thread of reasoning that started from the Linear issue.

**Why this is fine in practice**:

- The PR body contains `Fixes LIN-123`. When the agent orients on the PR review, it reads the PR description and learns the context. That's exactly how a human developer works — finish a task, move on, come back later and re-read the PR to remember what it was about.
- If the agent needs more Linear context mid-review, it uses its Linear tools to fetch the issue. That's one extra Nango call's worth of context at the start of a review thread. Not free, but cheap, and it's something the agent does naturally anyway.
- The agent's memory (via `SharedMemory: true` or the memory feature on `Agent`) persists task-level knowledge across conversations when that matters.

**What you lose**: a single uninterrupted conversational thread spanning "pickup → work → review → merge." You get multiple shorter threads with overlapping but not identical context.

**What you gain**: zero new complexity in the dispatcher, zero new tables, zero new tools, zero new validation, and a clear mental model users can reason about — "each conversation starts when a specific webhook arrives and continues as long as events land on the same provider resource."

## The design discussion we tabled

The cross-provider continuation problem is real but not urgent. We explored three approaches and chose "two agents" after weighing the tradeoffs.

### Approach 1: Link table + agent-initiated linking

Add a `conversation_resource_links` many-to-many table. Give the agent a `zira_link_resource(connection_id, resource_type, resource_id)` tool. When the agent creates a PR while working on a Linear task, it calls the tool, which inserts a row linking the Linear conversation to the GitHub PR's resource key. Future PR events look up the link table as a secondary lookup path.

**Why we rejected it**: you said it clearly — *"we should default to doing the heavy lifting ourselves, so agents can focus only on their tasks."* The moment we give agents a `zira_link_resource` tool, we're effectively saying "part of your job is now plumbing." That's the wrong direction. Agents should get a task and focus on it, not spend tool budget on correlation housekeeping.

### Approach 2: Multi-provider trigger rows

Restructure `AgentTrigger` so one row can have multiple `(connection, trigger_keys, conditions, context, instructions)` sub-configs. A single trigger row would cover all providers an agent listens on, producing one unified YAML config instead of four separate rows.

**Why we rejected it**: significant schema churn (nested JSONB structures, per-subconfig validation, dispatcher changes to walk the nested shape), and it doesn't actually solve cross-provider continuation — you'd still need the resource-key-linking problem addressed somehow. It's ergonomics-only, and the UI can achieve the same grouping with a `bundle_id` column that costs ~5 minutes. See [08-design-decisions-deferred.md](08-design-decisions-deferred.md).

### Approach 3: Two agents with shared config

The one we chose. Customers create one agent per entry point, with the same system prompt, same sandbox template, same tools, and different triggers. The "duplication" is really just two rows in the agents table — everything meaningful is shared.

**Why this is better than it sounds**:
- Zero new code
- Works with the current schema and dispatcher as-is
- Clean mental model: one agent = one source of work
- Each agent has its own conversation history scoped to its source
- When a Linear agent creates a PR, the PR's follow-up events (reviews, CI) fire triggers on the same agent, keeping continuity within the Linear-initiated work phase
- Same for the Slack agent
- If the customer later says "we really need the two conversations to merge," we can revisit with more real data about the actual pain

The two-agents pattern is the boring answer. Boring is the right answer when the alternative is a new subsystem.

## Validation for multi-provider setups

Nothing new was added for this — the existing per-row validation handles it correctly. Each AgentTrigger row's `validateTriggerRequest` call:

1. Resolves the provider from the connection
2. Validates `trigger_keys` against the catalog for THAT provider
3. Validates context actions exist in THAT provider's catalog
4. Validates they're read-only
5. Validates conditions are well-formed
6. Validates terminate rules recursively

Two agents with four rows each goes through eight independent validation runs. A typo in the Linear trigger's conditions only fails the Linear row — the GitHub rows save independently. This per-row atomicity is a quiet win: users can fix one misconfigured trigger without losing the others.

## Where Intercom would fit

The other example we discussed was a customer-support agent on Intercom. The pattern is the same:

```yaml
# Agent zira-support-worker — single trigger row
connection_id: <intercom-connection-uuid>
trigger_keys:
  - conversation.user.replied
conditions:
  match: all
  rules:
    - path: data.item.state
      operator: equals
      value: open
context:
  - as: conversation
    action: retrieve_conversation
    params:
      conversation_id: $refs.conversation_id  # assumes refs are added to the Intercom trigger catalog
instructions: |
  A customer replied in an open Intercom conversation. Draft a response
  as a note on the conversation — do not send it directly.
```

The catch: Intercom's triggers in the catalog currently have no `refs` defined, and the `conversation` resource has no `ref_bindings` or `resource_key_template`. Adding those is a catalog-only change — no code changes — but it's a prerequisite for the Intercom path to work cleanly. See [09-known-limitations.md](09-known-limitations.md).

## Where to go from here

- What we explicitly deferred and why: [08-design-decisions-deferred.md](08-design-decisions-deferred.md)
- Known gaps in the catalog for providers other than GitHub: [09-known-limitations.md](09-known-limitations.md)
- The resource-key mechanism that makes within-provider continuation work: [03-lifecycle-and-continuation.md](03-lifecycle-and-continuation.md)
