# 02 — Trigger Configuration Strategy

Once you know what your agent is for (see [01](01-agent-design-principles.md)), the question becomes: how do you translate that into concrete trigger configuration? This doc covers the four moving parts of a trigger config — events, conditions, context, and instructions — and how to think about each one strategically rather than just technically.

The technical reference is in the [top-level architecture docs](../README.md). This doc is about the decisions you make when filling in each field, and the tradeoffs behind those decisions.

## The four layers

Every trigger config has four layers, roughly in order of "broad → narrow":

1. **Events** (`trigger_keys`) — which webhooks cause this trigger to be evaluated at all
2. **Conditions** — which of those webhooks actually produce a run
3. **Context** (`context_actions`) — what data the agent gets before it runs
4. **Instructions** — what the agent is told to do with that data

Think of these as a funnel. Events cast a wide net; conditions prune to the ones you actually care about; context gives the agent what it needs to reason; instructions tell it what to do. Each layer should be as specific as possible without being brittle.

## Layer 1: Events

**Pick events that match your scope statement, not events that might be tangentially related.** The temptation is to listen to every event that could possibly matter. Resist it.

### The two common failure modes

**Too broad**: listening for `issues.*` (all sub-actions) when you only care about `issues.opened`. Every labeled, edited, assigned, milestoned event fires your agent unnecessarily, wastes compute, and forces your conditions to do more work. Fix: list the exact sub-actions you need.

**Too narrow**: listening only for `pull_request.opened` when you also want to react to `pull_request.synchronize` (new commits on an open PR) and `pull_request.ready_for_review` (draft promoted). You'll miss events the user intuitively expects you to handle. Fix: think through the user's mental model and include the events that feel natural.

### How to pick the right set

Walk through a typical lifecycle of the resource. For a PR review agent:

1. PR opened → agent reviews ✓ → need `pull_request.opened`
2. Author pushes more commits → agent re-reviews ✓ → need `pull_request.synchronize`
3. Author marks draft as ready → agent reviews ✓ → need `pull_request.ready_for_review`
4. Reviewer asks a question → agent responds ✓ → need `issue_comment.created` + `pull_request_review_comment.created`
5. PR merged → agent wraps up ✓ → need `pull_request.closed` (as a terminate rule, not a normal trigger)

Five events plus one terminate event. Any event not in this list is outside scope — don't add it "just in case."

### Multi-event trigger rows

A single `AgentTrigger` row can list multiple event keys in `trigger_keys`. This is the right choice when the events all share the same conditions, context, and instructions. Example:

```yaml
trigger_keys:
  - pull_request.opened
  - pull_request.synchronize
  - pull_request.ready_for_review
```

All three events get the same treatment — evaluated against the same `pull_request.draft not_equals true` condition, producing the same context fetch, running the same instructions. One row handles all three.

Split into multiple rows when the treatments differ. If you want different instructions for "PR opened" vs "new commits on an existing PR," those belong in separate rows (or in the agent's system prompt reading the trigger key at runtime — see below).

### When to use the trigger key inside instructions

A single row that handles multiple events can branch on the event key inside the instructions:

```yaml
instructions: |
  Event: $refs.trigger_key
  (but trigger_key isn't actually a ref...)
```

This pattern doesn't work today — there's no `$refs.trigger_key` automatically. If you need per-event branching, either split into multiple rows or let the agent read the `action` field from the payload. Usually splitting is cleaner.

## Layer 2: Conditions

Conditions are where you turn "events that might be relevant" into "events that are actually relevant." Good conditions are specific, fail-closed, and exclude the agent itself.

### Condition-writing principles

**1. Exclude yourself.** Every condition block should filter out the agent's own activity on the surface it's watching. If your agent posts comments on PRs, its triggers should have a condition like `sender.login not_equals zira-bot[bot]`. Without this, the agent will reply to its own comments, and you have an infinite loop. See [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md) for the details.

**2. Exclude bots you don't care about.** Dependabot, Renovate, Mergify, and similar bots generate a lot of noise. A code review agent that triages Dependabot PRs is probably not what the customer wants. Filter them out with `sender.login not_one_of [dependabot[bot], renovate[bot], mergify[bot]]`.

**3. Use the narrowest filter that covers the scope.** If you only care about PRs on feature branches, use `pull_request.head.ref matches "^feature/"`, not "PRs from these 20 authors" or "PRs with this label." Branch prefix is the most specific signal available.

**4. Match the scope statement.** Every condition should trace back to a phrase in your one-sentence scope statement. If a condition doesn't trace back, ask why it's there.

**5. Fail-closed, not fail-open.** `match: all` is the default for a reason. If any condition fails, the agent doesn't run. This is safer than `match: any`, which runs if ANY condition passes and is easier to accidentally configure into a loop. Use `any` only when you genuinely mean OR.

### Operator choice cheatsheet

| What you want | Operator | Example |
|---|---|---|
| Exact match | `equals` | `pull_request.state equals open` |
| Exact non-match | `not_equals` | `pull_request.draft not_equals true` |
| One of a set | `one_of` | `labels.*.name one_of [bug, urgent]` |
| None of a set (bots!) | `not_one_of` | `sender.login not_one_of [dependabot[bot]]` |
| Substring check | `contains` | `comment.body contains "@zira"` |
| Substring absence | `not_contains` | `comment.body not_contains "[skip-zira]"` |
| Regex match | `matches` | `pull_request.head.ref matches "^feature/"` |
| Field is present | `exists` | `issue.pull_request exists` |
| Field is absent | `not_exists` | `issue.pull_request not_exists` |

### Common patterns

**"Only on PRs, not issues" (for issue_comment):**
```yaml
- path: issue.pull_request
  operator: exists
```

**"Only on issues, not PRs" (same event):**
```yaml
- path: issue.pull_request
  operator: not_exists
```

**"Exclude myself and known bots":**
```yaml
- path: sender.type
  operator: not_equals
  value: Bot
- path: sender.login
  operator: not_equals
  value: zira-bot[bot]
```

**"Only feature branches":**
```yaml
- path: pull_request.head.ref
  operator: matches
  value: "^feature/"
```

**"Only when merged, not just closed":**
```yaml
- path: pull_request.merged
  operator: equals
  value: true
```

**"Only on failure":**
```yaml
- path: workflow_run.conclusion
  operator: one_of
  value: [failure, cancelled, timed_out]
```

### The `exists` / `not_exists` trap

`exists` and `not_exists` check whether the path resolves to a non-nil value. They're strict about presence, not about truthiness. A field set to `false`, `0`, or `""` still counts as "exists" because the field is in the payload. If you want "field is truthy," use `equals true` or similar.

### Conditions are not validation

Conditions filter which events trigger a run. They don't validate that the resulting context will be useful. An agent can pass all conditions and still be unable to do useful work because the context fetch returns nothing. Validation of context is the agent's job at runtime, not the condition layer's job.

## Layer 3: Context actions

Context actions fetch data before the agent runs. They're pre-flight reads against the provider's API (via Nango) that populate the agent's initial prompt with everything it needs to reason. Done right, they save turns and tokens. Done wrong, they waste API budget and slow down every run.

### The economic question

Every context action is a round-trip to the provider. Cost isn't free. Before adding an action, ask: **will the agent definitely need this for every run, or only sometimes?**

- **"Definitely"** → pre-fetch via context action
- **"Sometimes"** → let the agent fetch it via tools when needed
- **"Never more than once per conversation"** → pre-fetch on the first event, don't re-fetch on continuation

The first and third cases are what context actions are for. The middle case — conditional fetches — should go through runtime tools, because tools can be decided on per-turn and skipped when not needed.

### Don't duplicate what's in the payload

GitHub webhook payloads are chunky. They already contain the full issue object, the full PR object, the repository info, the sender info, and more. If you can get what you need from the payload directly, DON'T add a context action for it. Use `{{$payload.issue.title}}` in the instructions instead — or, more idiomatically, define the ref you need on the trigger and use `$refs.x`.

Context actions exist to fetch things that AREN'T in the payload: related items (other issues, previous PRs), aggregated data (list of files, list of reviewers), related content (CONTRIBUTING.md, prior comments).

### Common context action patterns

**Fetch the full resource**: when the payload has a truncated version and you need the full thing.
```yaml
- as: pr
  action: pulls_get
  ref: pull_request
```

Even though the payload already contains a pull request object, `pulls_get` returns additional fields (file counts, mergeable state, statuses) that aren't in the webhook body.

**List related items**: when the agent needs to know about siblings.
```yaml
- as: files
  action: pulls_list_files
  ref: pull_request

- as: reviews
  action: pulls_list_reviews
  ref: pull_request
```

**Search for related content**: when the agent needs context from elsewhere in the repo.
```yaml
- as: similar
  action: search_issues_and_pull_requests
  params:
    q: "repo:$refs.repository is:issue {{$refs.issue_title}}"
```

**Optional fetches**: when the thing might not exist.
```yaml
- as: guidelines
  action: repos_get_content
  ref: repository
  params:
    path: ".github/CONTRIBUTING.md"
  optional: true
```

Always mark as `optional: true` when the resource might not exist. The executor treats the failure as empty output instead of blowing up the whole run.

### Ordering context actions

Actions run in the order listed. If a later action references an earlier one's output via `{{$step.field}}` substitution, put the earlier action first. Example:

```yaml
context:
  - as: pr
    action: pulls_get
    ref: pull_request
  - as: reviewer_profile
    action: users_get_by_username
    params:
      username: "{{$pr.requested_reviewers.0.login}}"
```

The `reviewer_profile` fetch depends on `pr.requested_reviewers`, so `pr` must be fetched first. The dispatcher validates this — a reference to a step that doesn't exist earlier is a config error.

### How many context actions is too many

A rule of thumb: **3 to 5 context actions per trigger is normal, 8+ is a smell.**

If you need more than 5, ask:
- Are some of them conditional? If yes, move them to runtime tools.
- Are they duplicating payload data? If yes, drop them.
- Is the agent really going to use all of this? If no, cut the unused ones.

More context isn't free. Every action is a network round-trip (tens to hundreds of milliseconds), and the results get stuffed into the agent's opening prompt, consuming tokens. A reasonable agent prompt is measured in kilobytes, not megabytes.

## Layer 4: Instructions

Instructions are the agent's per-trigger briefing. They're concatenated with the system prompt and context data to form the opening message of the conversation. Good instructions are specific about the task, concrete about the inputs, and explicit about the terminal action.

### The instructions anatomy

A well-written instructions block has three parts:

1. **What happened** — a human-readable summary of the event, with the key facts substituted
2. **The data you have** — explicit references to the context bag (`{{$pr}}`, `{{$files}}`, etc.)
3. **What to do with it** — clear direction on the action to take, and when to stop

Example:

```yaml
instructions: |
  A pull request was opened in $refs.repository by $refs.sender.

  ## The pull request
  Title: {{$pr.title}}
  Description: {{$pr.body}}
  Base: {{$pr.base.ref}} ← Head: {{$pr.head.ref}}

  ## Files changed
  {{$files}}

  ## Repository conventions
  {{$guidelines}}

  Read the changed files and leave a single review comment summarizing:
  - Any style violations against the conventions document
  - Any obvious bugs or logic errors
  - Any missing test coverage for new code paths

  If you don't find anything worth flagging, post a single approval comment
  and stop. Do not leave an inline comment on every file — one summary
  comment is enough.
```

The key bits:

- **Concrete facts** up top, not vague context
- **Clear data sources** using `{{$variable}}` syntax
- **Specific instructions** at the bottom, including what NOT to do ("Do not leave an inline comment on every file")
- **Terminal action** is explicit ("post a single review comment and stop")

### Write the terminal action explicitly

The single most common instruction bug is forgetting to tell the agent when it's done. Without a clear terminal action, the agent will over-do it — reply six times, open follow-up issues, comment on unrelated PRs, iterate forever. The terminal action is the contract: "do X, then stop."

Every instructions block should end with something like:

- "Reply with a single comment and stop."
- "Push new commits to the PR branch and post a comment; do not request a re-review."
- "Save the draft as a note on the conversation; do not send it."
- "Label the issue and stop."
- "If the check passed, do nothing and exit."

If you don't include this, the agent has to infer it. LLMs are good at many things; inferring social norms about "when is this task done" is not one of them.

### Use prose, not pseudocode

Tempting anti-pattern: writing instructions as a flowchart.

```
IF pull_request.draft == true:
  do nothing
ELSE IF pull_request.base == "main":
  full review
ELSE:
  quick review
```

Don't do this. Instructions should be prose. If you have flowchart logic, push it into the trigger conditions (so unsupported cases never fire the trigger at all), or let the agent make the judgment call from prose.

```yaml
instructions: |
  Review the pull request. If it targets the main branch, do a full
  review including security and test coverage. For other branches,
  a quick review covering obvious bugs is enough.
```

LLMs handle prose well. They handle pseudocode poorly because they'll second-guess your logic and rewrite it in their head.

### Scope boundaries in instructions

Repeat the key scope boundaries in the instructions. Even if they're in the system prompt, repeating them here makes them more salient during the current task:

```yaml
instructions: |
  Review the pull request.

  Reminder: you are reviewing only for style and obvious bugs.
  Do not comment on architectural decisions; that's the human reviewer's
  job. Do not approve or merge the PR.
```

Agents forget distant context more easily than recent context. Instructions are recent context.

### Instructions vs. system prompt

A common confusion: what goes in the system prompt, and what goes in the per-trigger instructions?

- **System prompt** = the agent's identity and persistent rules. Who it is, what it cares about, what it absolutely won't do, its overall operating style. Written once, applies to every run.
- **Instructions** = the task for THIS specific event. What just happened, what to do about it, what data it has for the job.

Rule of thumb: if it applies to every trigger the agent handles, it belongs in the system prompt. If it's specific to this event or this trigger, it belongs in the instructions.

The system prompt doesn't change between runs. Instructions do. Most of the per-task variability should live in instructions; the identity and the rules should live in the prompt.

## Putting it all together

A complete trigger config for a PR review agent might look like:

```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - pull_request.opened
  - pull_request.synchronize
  - pull_request.ready_for_review

conditions:
  match: all
  rules:
    - path: pull_request.draft
      operator: not_equals
      value: true
    - path: pull_request.base.ref
      operator: equals
      value: main
    - path: sender.login
      operator: not_equals
      value: zira-review-bot[bot]

context:
  - as: pr
    action: pulls_get
    ref: pull_request
  - as: files
    action: pulls_list_files
    ref: pull_request
  - as: guidelines
    action: repos_get_content
    ref: repository
    params:
      path: ".github/CONTRIBUTING.md"
    optional: true

instructions: |
  A PR was updated in $refs.repository.

  ## Pull Request
  Title: {{$pr.title}}
  Author: {{$pr.user.login}}
  Description: {{$pr.body}}

  ## Changed files
  {{$files}}

  ## Contributing guidelines
  {{$guidelines}}

  Review the changes against the guidelines. Leave one summary review
  comment noting style violations, bugs, and missing test coverage. If
  nothing needs fixing, post a single approval. Do not leave inline
  comments on every file.

terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.merged
          operator: equals
          value: true
    instructions: |
      The PR was merged. Post a brief thank-you comment and stop.
  - trigger_keys: [pull_request.closed]
    silent: true
```

Every layer is specific, bounded, and traces back to the scope statement "review PRs targeting main, ignoring drafts and my own activity, using the repo's contributing guidelines." This is what good trigger configuration looks like.

## Where to go from here

- The failure mode you most need to worry about: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- How to think about conversation lifecycle: [03-lifecycle-and-state-management.md](03-lifecycle-and-state-management.md)
- Complete worked examples: [07-worked-examples.md](07-worked-examples.md)
- Common mistakes: [08-anti-patterns.md](08-anti-patterns.md)
