# 08 — Anti-Patterns

These are things that look reasonable when you're writing your first trigger config, cause real problems in production, and are easy to avoid once you've seen them once. Each anti-pattern below includes what it looks like, why it's a problem, and what to do instead.

## 1. The "handles everything" agent

**What it looks like**:

```yaml
trigger_keys:
  - issues.opened
  - issues.closed
  - issues.edited
  - issues.labeled
  - pull_request.opened
  - pull_request.closed
  - pull_request_review.submitted
  - workflow_run.completed
  - push
  - release.published
```

Ten trigger keys on one agent. The system prompt is three pages long with an if/else tree for "if this is an issue, do X; if a PR, do Y; if CI, do Z..."

**Why it's a problem**: the agent does too many things badly instead of one thing well. Each event type has its own scope, conditions, and reasoning requirements — cramming them into one agent means your system prompt has to cover all of them, your instructions have to branch on the event type, and debugging any one thing means untangling the interactions with everything else.

**What to do instead**: split into 2–5 smaller agents, each with a single responsibility. A PR reviewer, an issue triager, a CI responder. Separate agents can share a system prompt family (same tone, same general guidelines) via copy-paste while having their own specific instructions. See [01-agent-design-principles.md](01-agent-design-principles.md).

## 2. Forgetting to exclude yourself

**What it looks like**:

```yaml
trigger_keys:
  - issue_comment.created

conditions:
  match: all
  rules:
    - path: comment.body
      operator: contains
      value: "@our-bot"

# ... no filter on sender.login
```

The agent responds to @-mentions. The agent's own replies contain "@our-bot" because it's addressing the user. The reply fires another event, the agent responds again, loop.

**Why it's a problem**: the classic infinite loop. If your agent posts to the same surface it listens on, and you don't filter out the agent's own identity, you WILL hit a loop within the first hour of production.

**What to do instead**: always add `sender.login not_equals <bot-identity>` (or `sender.type not_equals Bot` as a backup). See [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md).

## 3. Using `match: any` when you meant `match: all`

**What it looks like**:

```yaml
conditions:
  match: any
  rules:
    - path: sender.login
      operator: not_equals
      value: bot-name
    - path: pull_request.draft
      operator: not_equals
      value: true
```

You wanted "not a bot AND not a draft." You wrote `match: any`, which means "not a bot OR not a draft." So drafts from non-bots still trigger the agent, and bot-authored non-drafts also trigger it. Two bugs in one config.

**Why it's a problem**: `any` is rarely what you want. Multiple conditions usually combine as AND — "this AND that AND the other." OR is appropriate only when you're matching alternate paths ("fire on label X OR label Y"). Most filter logic is AND.

**What to do instead**: default to `match: all`. Use `match: any` only when you can verbalize the rule as "any of these is enough."

## 4. Over-broad `trigger_keys`

**What it looks like**:

```yaml
trigger_keys:
  - pull_request.opened
  - pull_request.closed
  - pull_request.edited
  - pull_request.synchronize
  - pull_request.ready_for_review
  - pull_request.converted_to_draft
  - pull_request.labeled
  - pull_request.unlabeled
  - pull_request.assigned
  - pull_request.unassigned
  - pull_request.milestoned
  - pull_request.demilestoned
  - pull_request.review_requested
  - pull_request.review_request_removed
```

Listening for every `pull_request.*` sub-action "just in case."

**Why it's a problem**: the agent fires on noise. Labeling a PR, assigning a reviewer, milestoning — most of these aren't things the agent needs to react to. Each unnecessary fire wastes compute and tokens, and clutters the agent's conversation history with irrelevant events. The conversation drifts off-topic over time.

**What to do instead**: walk through the user's mental model. Which events would a human reviewer actually care about? Usually: `opened`, `synchronize`, `ready_for_review`, maybe `reopened`. That's 3–4 events, not 14.

## 5. Fetching everything "just in case"

**What it looks like**:

```yaml
context:
  - as: pr
    action: pulls_get
    ref: pull_request
  - as: files
    action: pulls_list_files
    ref: pull_request
  - as: reviews
    action: pulls_list_reviews
    ref: pull_request
  - as: review_comments
    action: pulls_list_review_comments
    ref: pull_request
  - as: comments
    action: issues_list_comments
    ref: issue
  - as: commits
    action: pulls_list_commits
    ref: pull_request
  - as: author
    action: users_get_by_username
    params:
      username: "{{$pr.user.login}}"
  - as: contributors
    action: repos_list_contributors
    ref: repository
  - as: readme
    action: repos_get_content
    ref: repository
    params:
      path: README.md
  - as: contributing
    action: repos_get_content
    ref: repository
    params:
      path: .github/CONTRIBUTING.md
```

Ten context actions. The agent uses maybe 3 of them consistently. The other 7 are fetched on every run and mostly ignored.

**Why it's a problem**: wasted Nango calls (rate limit pressure), wasted tokens in the opening prompt (the agent has to read all this stuff), slower runs (each call is network latency), harder debugging (what's in the prompt?). If you have 10 context actions and the agent is firing 1000 times a day, you're making 10,000 API calls a day you don't need.

**What to do instead**: pre-fetch only what the agent uses in >80% of runs. Everything else moves to runtime tools the agent can call only when needed. See [05-economics-and-performance.md](05-economics-and-performance.md).

## 6. Missing the terminal action in instructions

**What it looks like**:

```yaml
instructions: |
  A PR was opened. Review the code and leave feedback.
```

Three sentences. No guidance on how much feedback, what format, or when to stop.

**Why it's a problem**: the agent doesn't know when it's done. It might leave one comment and stop — good. Or it might leave twenty inline comments — bad. Or it might open a new issue to track follow-ups — probably not what you wanted. Without a terminal action, behavior is unpredictable from run to run.

**What to do instead**: always end instructions with an explicit terminal action. "Post a single review comment and stop." "Push a fix commit and comment; do not re-request review." "Label the issue and exit." The terminal action is the agent's contract with you.

## 7. Putting business logic in instructions

**What it looks like**:

```yaml
instructions: |
  If this is a PR to main and the author is a core committer, do a
  full review. If the author is a new contributor, do a welcoming
  review. If the PR is marked as "wip", do nothing. If the PR
  changes files in the security directory, escalate to the security
  team. If the PR changes database migrations, check for backwards
  compat. If...
```

The instructions block is a nested flowchart.

**Why it's a problem**: LLMs are bad at nested if/else. They'll either miss a branch, take the wrong one, or ignore the structure entirely. Also, most of these branches should be trigger-level conditions that prevent the agent from firing in the first place.

**What to do instead**: push branching logic into conditions. Create separate trigger rows for cases that need different treatment. Keep instructions as prose about the task, not control flow.

```yaml
# Trigger row 1: welcome new contributors
conditions:
  match: all
  rules:
    - path: pull_request.author_association
      operator: equals
      value: FIRST_TIME_CONTRIBUTOR
instructions: |
  A first-time contributor opened a PR. Write a warm welcoming review...

# Trigger row 2: standard review for core contributors
conditions:
  match: all
  rules:
    - path: pull_request.author_association
      operator: one_of
      value: [MEMBER, OWNER, COLLABORATOR]
instructions: |
  A core contributor opened a PR. Do a rigorous review focused on...
```

Two rows, two simple instruction blocks, no nested branching.

## 8. Using conditions to do validation

**What it looks like**:

```yaml
conditions:
  match: all
  rules:
    - path: pull_request.body
      operator: contains
      value: "Fixes #"  # require an issue link in the body
```

Using a condition to enforce that PRs link to an issue.

**Why it's a problem**: conditions are for "should I fire the agent at all?" If you want to enforce a policy, the enforcement belongs in the agent's behavior, not in the trigger's gate. A PR without an issue link should STILL trigger the agent — the agent can then comment asking for the link. Hiding the PR entirely behind a condition gives the user no feedback.

**What to do instead**: let the event through, have the agent check the condition, and take appropriate action. The agent can post "this PR doesn't link to an issue, please add one" and stop.

## 9. Multiple agents with overlapping scopes

**What it looks like**:

Agent `pr-reviewer` fires on `pull_request.opened` and does a full review.

Agent `style-checker` also fires on `pull_request.opened` and does style checking.

Agent `security-scanner` also fires on `pull_request.opened` and does security review.

None of them have conditions excluding the others. All three fire on every PR. Three separate comments land on the PR within seconds of each other.

**Why it's a problem**: redundant work, cluttered PR conversations, conflicting advice when two agents disagree. The user sees three bot comments and doesn't know which to prioritize.

**What to do instead**: either merge into one agent with broader scope (if they're really doing the same job with different lenses) or scope each agent to a different subset of PRs. Maybe `pr-reviewer` handles feature branches, `security-scanner` only fires on PRs that touch `security/` directories, `style-checker` only fires on JS files. Non-overlapping scopes prevent redundancy.

## 10. Ignoring the `draft` flag

**What it looks like**:

```yaml
trigger_keys:
  - pull_request.opened

# no condition on pull_request.draft
```

Every PR triggers the agent — including draft PRs that aren't ready for review.

**Why it's a problem**: drafts are work in progress. Commenting on them while the author is still iterating is noisy and annoying. The author doesn't want a formal review yet; they want to work without interruption. Agents that ignore the draft flag get turned off by frustrated developers.

**What to do instead**: filter out drafts:

```yaml
conditions:
  match: all
  rules:
    - path: pull_request.draft
      operator: not_equals
      value: true
```

Also listen for `pull_request.ready_for_review` so the agent fires when a draft graduates to ready. See the worked example in [07-worked-examples.md](07-worked-examples.md).

## 11. Assuming events arrive in order

**What it looks like**:

```yaml
instructions: |
  Respond to this event only if you haven't seen this PR before.
  If you've already responded, do nothing.
```

The agent is supposed to track state across events and not re-respond.

**Why it's a problem**: the agent doesn't have reliable "first time I've seen this" state. Events can arrive out of order, the conversation history might be truncated, and even if the agent sees its prior messages, deciding "is this a new event or a re-send" is hard.

**What to do instead**: design the agent to be idempotent — it can process the same event multiple times without duplicating effort. The continuation model already handles "second event on the same PR" by putting prior messages in context; the agent should look at what's already been said and decide what, if anything, to add.

If strict at-most-once processing matters, enforce it at the dispatcher layer via asynq's `Unique()` dedup on the delivery ID — not at the agent's reasoning layer.

## 12. Writing triggers for events that don't have catalog refs

**What it looks like**:

```yaml
trigger_keys:
  - conversation.user.replied   # Intercom

context:
  - as: conversation
    action: retrieve_conversation
    params:
      conversation_id: $refs.conversation_id   # doesn't exist!
```

You try to use `$refs.conversation_id` but the Intercom trigger catalog doesn't define `conversation_id` as a ref.

**Why it's a problem**: the ref resolves to empty, the path becomes `/conversations/` (missing the ID), the Nango call fails, the agent runs with broken context. The dispatcher doesn't always catch this — it logs a warning about missing refs but continues.

**What to do instead**: check the trigger catalog first to see what refs are defined. If a ref you need doesn't exist, add it to the trigger catalog's JSON before writing the trigger config. See [../01-catalog-architecture.md](../01-catalog-architecture.md) for how the catalog is structured and [../09-known-limitations.md](../09-known-limitations.md) for which providers have ref gaps.

## 13. Not testing conditions before deploying

**What it looks like**:

You write a new trigger, save it, and wait to see if it works. When it doesn't fire on a real event, you spend an hour debugging the conditions.

**Why it's a problem**: conditions are where most bugs hide. A typo in a path (`pull_request.user.type` vs. `pull_request.user_type`), a wrong operator (`contains` vs. `equals`), a missing condition — any of these can silently break your trigger without producing an error.

**What to do instead**: use the test harness described in [06-observability-and-debugging.md](06-observability-and-debugging.md). Fire a test webhook, watch the dispatch logs, confirm the skip reason matches what you expected. Test both the happy path (conditions pass) and the unhappy path (conditions fail) — make sure the filter actually filters.

## 14. Giving agents write access they don't need

**What it looks like**:

You configure the agent with a full-scope GitHub App that has read/write access to everything. You figure "better to have permissions and not need them." The agent's job is only to leave comments.

**Why it's a problem**: if the agent misbehaves, the blast radius is everything the app can touch. A commenting agent that loops is embarrassing; a merging agent that loops is a full-scale incident. Principle of least privilege: the agent should only have the permissions it needs for its actual job.

**What to do instead**: scope the app's permissions narrowly. A commenting agent needs `pull_requests: write` (for comments) but not `contents: write` (for pushes). Use separate apps for separate permission scopes; attach each agent to the narrowest app it can work with.

## 15. Skipping the system prompt

**What it looks like**:

You focus all your configuration effort on the trigger (conditions, context, instructions) and leave the agent's system prompt as "You are a helpful assistant."

**Why it's a problem**: the system prompt is the agent's identity. Without a strong identity, the agent's behavior is inconsistent across runs — sometimes it's formal, sometimes chatty; sometimes thorough, sometimes brief. The instructions block per trigger can't fully compensate because identity applies to every run.

**What to do instead**: invest time in the system prompt. Describe the agent's role, its tone, its constraints, its refusal rules, its interaction style with humans. A good system prompt is 100–500 words and reads like a job description for the agent. See the examples in [07-worked-examples.md](07-worked-examples.md) for agents with clear identities.

## 16. No termination rules on long-lived resources

**What it looks like**:

```yaml
trigger_keys:
  - pull_request.opened
  - pull_request.synchronize

# no terminate_on
```

The agent reviews PRs but never closes the conversation. Every event on every PR keeps firing forever.

**Why it's a problem**: conversations accumulate. Each continuation event carries the prior conversation in context. Over time, the conversation grows, tokens per run grow, costs grow. Eventually the conversation hits context window limits and the agent starts forgetting early turns.

**What to do instead**: add a terminate rule for the natural "done" state — PR merged or closed. The executor will close the conversation and future events on that PR (if any) will start fresh. See [03-lifecycle-and-state-management.md](03-lifecycle-and-state-management.md).

## 17. Treating the system prompt as runtime configuration

**What it looks like**:

```
System prompt:
"You are a code reviewer. Review PRs in the company/main-repo.
You only care about PRs from core contributors on the main branch.
Ignore draft PRs and bot-authored PRs."
```

The system prompt contains filtering rules that should be conditions.

**Why it's a problem**: putting filters in the system prompt means the agent still runs on every event and then decides not to act. This wastes compute (run + tokens for a no-op decision) and relies on the LLM to enforce the rule, which it sometimes forgets.

**What to do instead**: filter at the trigger condition layer, which is enforced mechanically. The system prompt should contain the agent's identity and role, not runtime filters.

## 18. Silently ignoring optional context failures

**What it looks like**:

```yaml
context:
  - as: guidelines
    action: repos_get_content
    params:
      path: .github/CONTRIBUTING.md
    # no optional: true
```

The CONTRIBUTING.md doesn't exist in every repo. When it's missing, the context action fails and the whole run is skipped with a build error.

**Why it's a problem**: a missing optional file shouldn't break the whole trigger. The run should proceed with an empty variable, not a failure.

**What to do instead**: mark fetches that might fail as `optional: true`. The executor will treat failures as empty output and continue.

## 19. Writing terminate rules for events that aren't terminal

**What it looks like**:

```yaml
terminate_on:
  - trigger_keys: [issue_comment.created]
    instructions: "Close the issue conversation"
```

Termination on any comment. So the very next comment closes the conversation.

**Why it's a problem**: `issue_comment.created` fires constantly. It's a normal mid-lifecycle event, not a terminal one. Terminating on it means every conversation closes after one or two comments.

**What to do instead**: only terminate on events that represent genuine end-of-lifecycle states: `issues.closed`, `pull_request.closed` (with a merged filter), `conversation.admin.replied`, `release.published`. If you can verbalize the terminate rule as "once this happens, the agent's job is done," it's a good terminate event. Otherwise it's probably a continuation event.

## 20. Forgetting that conditions silently filter out missing-field cases

**What it looks like**:

```yaml
conditions:
  match: all
  rules:
    - path: pull_request.head.repo.fork
      operator: equals
      value: false
```

You want to exclude forks. But when the payload doesn't have `pull_request.head.repo.fork` (some events don't), the condition resolves "path missing" to "condition fails," and the run is skipped — even for non-fork cases.

**Why it's a problem**: fail-closed behavior is good for safety, but it can silently exclude events you wanted to include if you're not careful.

**What to do instead**: combine the positive check with an existence check, or negate it:

```yaml
# This excludes only confirmed forks; events without the field get through
- path: pull_request.head.repo.fork
  operator: not_equals
  value: true
```

Or make it explicit:

```yaml
- path: pull_request.head.repo.fork
  operator: not_exists
# (combine with OR-equivalent logic if needed)
```

Read the dispatch skip logs after deploying to confirm your conditions are filtering what you expected, not more.

## The meta-lesson

Most of these anti-patterns share a common thread: **treating the trigger system as if it were a simple "when X, do Y" mechanism, instead of as a data-driven pipeline with specific semantics**.

The specific semantics matter:

- Conditions fail closed
- Context must be read-only
- Continuation happens automatically on resource key matches
- The system prompt is identity, conditions are filters, instructions are tasks
- Events can arrive out of order
- Every surface the agent touches is also a potential trigger source

Once these are internalized, the anti-patterns feel obviously wrong. Before they're internalized, they feel like reasonable shortcuts. This doc exists to speed up the internalization.

## Where to go from here

- The positive version of these lessons: [02-trigger-configuration-strategy.md](02-trigger-configuration-strategy.md)
- The safety patterns anti-patterns violate: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- How to catch these in practice: [06-observability-and-debugging.md](06-observability-and-debugging.md)
