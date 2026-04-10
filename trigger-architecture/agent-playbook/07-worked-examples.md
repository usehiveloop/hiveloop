# 07 — Worked Examples

This doc is a gallery of realistic agent configurations, each with the full trigger YAML plus commentary on why each choice was made. Every example is an agent someone would actually want to build — nothing contrived. Read these when you're designing a new agent and looking for a starting point, or when you want to understand the connection between the principles in the earlier docs and what the final YAML looks like.

Five examples:

1. **Style-guide enforcer** — reviews PRs for style violations, closes on merge
2. **Issue triage bot** — labels and assigns new bug reports
3. **CI failure responder** — investigates broken builds on its own branches
4. **Autonomous coding agent** — picks up Linear tasks and opens PRs
5. **Customer support drafter** — writes reply drafts for Intercom conversations

Each includes the one-sentence scope, the trigger configuration, a walkthrough of the conditions and context choices, and what specifically to watch out for.

## Example 1: Style-guide enforcer

**One-sentence scope**: "This agent exists to review new or updated pull requests for style violations against the repo's CONTRIBUTING.md, and it stops working when the PR is merged or closed."

**Agent setup**:
- Name: `style-reviewer`
- Sandbox type: `shared` (no filesystem state needed between runs)
- System prompt: short, identity-focused, mentions "only comment on style, not architecture"

**Trigger row**:

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
      operator: one_of
      value: [main, master, develop]
    - path: sender.login
      operator: not_equals
      value: style-reviewer[bot]
    - path: pull_request.user.type
      operator: not_equals
      value: Bot

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
  A pull request was updated in $refs.repository.

  ## Pull Request
  Title: {{$pr.title}}
  Author: {{$pr.user.login}}
  Description: {{$pr.body}}

  ## Files changed
  {{$files}}

  ## Repository style guide (may be empty)
  {{$guidelines}}

  Review the changes for STYLE violations only. Style includes:
  formatting, naming conventions, import ordering, comment style,
  documentation patterns. Do not comment on architecture, algorithm
  choices, or business logic — those are the human reviewer's job.

  If the repository has a CONTRIBUTING.md, use it as the source of
  truth. Otherwise, use common conventions for the language you see.

  Post exactly ONE summary review comment with all your findings.
  Do not leave inline comments on individual files. If you find no
  style issues, post a single approval comment saying so and stop.

terminate_on:
  - trigger_keys: [pull_request.closed]
    silent: true
```

### Commentary

**Why only drafts, main/master/develop branches, and non-bot senders**: scope. Drafts aren't ready for review. Feature branches targeting non-main destinations might be WIP or experimental. Bot PRs (Dependabot, Renovate) create automated churn the style reviewer shouldn't care about. Self-exclusion via `sender.login` closes the obvious loop.

**Why fetch the PR via `pulls_get` even though the payload has it**: the webhook payload has a pull_request object, but it's slightly outdated (mergeable state, latest reviewers list, up-to-date statuses). `pulls_get` returns the current state. Small fetch, worth it.

**Why `guidelines` is optional**: not every repo has a CONTRIBUTING.md. Making it optional means the run continues with an empty variable if the file doesn't exist, rather than failing.

**Why silent terminate on close**: the agent has nothing useful to say when a PR closes or merges. The review thread is already posted; a "goodbye" comment would be noise. Silent close is the right choice.

**What to watch out for**: the agent might be tempted to comment on architecture anyway — LLMs naturally want to be helpful. Reinforce the style-only scope in the system prompt too, not just in the instructions. A weekly review of the agent's comments catches scope drift.

## Example 2: Issue triage bot

**One-sentence scope**: "This agent exists to label and assign new bug reports based on content analysis, and it stops working once the issue is labeled (one-shot behavior)."

**Agent setup**:
- Name: `bug-triager`
- Sandbox type: `shared`
- System prompt: identity + list of available labels + assignment rules

**Trigger row**:

```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - issues.opened

conditions:
  match: all
  rules:
    - path: issue.pull_request
      operator: not_exists
    - path: sender.login
      operator: not_equals
      value: bug-triager[bot]
    - path: sender.type
      operator: not_equals
      value: Bot

context:
  - as: issue
    action: issues_get
    ref: issue
  - as: labels
    action: issues_list_labels_for_repo
    ref: repository
  - as: similar
    action: search_issues_and_pull_requests
    params:
      q: "repo:$refs.owner/$refs.repo is:issue state:open {{$refs.issue_title}}"

instructions: |
  A new issue was opened in $refs.repository.

  ## Issue
  Title: {{$refs.issue_title}}
  Body: {{$issue.body}}
  Reporter: {{$issue.user.login}}

  ## Available labels
  {{$labels}}

  ## Similar open issues
  {{$similar}}

  Analyze the issue. Apply ALL labels from the available list that
  apply. If the issue looks like a duplicate of one in "similar open
  issues", add the `duplicate` label and link to the original. If
  the issue is vague and needs more information, add the `needs-info`
  label and post a comment asking specific questions. Otherwise, just
  label appropriately and stop.

  Do NOT assign the issue to anyone — that's a human decision.

terminate_on:
  - trigger_keys: [issues.labeled]
    conditions:
      match: all
      rules:
        - path: label.name
          operator: not_equals
          value: needs-info
    silent: true
```

### Commentary

**Why only `issues.opened` and not the full `issues.*` family**: the job is one-shot triage of NEW issues. We don't re-triage when someone edits the title, and we don't re-label when someone closes. If we wanted re-triage, we'd add `issues.edited` — but that's a different job scope, probably a different agent.

**Why `issue.pull_request not_exists`**: GitHub's `issue_comment` event fires for both issues AND PR comments, but `issues.opened` is issue-only. This condition is belt-and-suspenders, just in case a PR-like payload sneaks through.

**Why terminate on `issues.labeled` (but not `needs-info`)**: the agent's job is done once the issue is labeled. Any manual relabeling is a signal that the agent shouldn't keep firing. The exception is `needs-info` — that's a label the agent itself might add, and we don't want to terminate immediately after adding it because the user might come back with more info and the agent should re-triage.

**Why `similar` context action**: finding duplicates is one of the agent's main jobs. Pre-fetching similar issues gives the LLM the context to notice patterns. Note the template substitution `{{$refs.issue_title}}` inside the search query — this uses the title ref as a search term.

**What to watch out for**: the agent will label things incorrectly sometimes. Label accuracy improves with a more specific system prompt that lists what each label means. Consider a "confidence check" step: if the agent isn't sure, it applies `needs-triage` and asks a human to review rather than guessing.

## Example 3: CI failure responder

**One-sentence scope**: "This agent exists to investigate failing CI runs on its own branches, diagnose the failure, and push a fix; it stops when the PR merges or is closed."

**Agent setup**:
- Name: `ci-fixer`
- Sandbox type: **`dedicated`** — this agent needs filesystem access to check out the branch, run tools, make commits
- System prompt: detailed debugging methodology, list of available tools (git, language-specific debuggers, test runners)

**Trigger row 1 — on CI failure**:

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
      value: "^zira/"
    - path: workflow_run.head_repository.fork
      operator: not_equals
      value: true

context:
  - as: run
    action: actions_get_workflow_run
    ref: workflow
  - as: jobs
    action: actions_list_jobs_for_workflow_run
    ref: workflow

instructions: |
  A CI run failed on a branch you own in $refs.repository.

  ## Failed run
  Name: {{$run.name}}
  Conclusion: {{$run.conclusion}}
  Branch: {{$run.head_branch}}
  Commit: {{$run.head_sha}}
  URL: {{$run.html_url}}

  ## Jobs
  {{$jobs}}

  Investigate the failure. Check out the branch locally, reproduce
  the failure if possible, identify the root cause, and push a fix.

  If you can't diagnose the failure in under 5 tool calls, post a
  comment on the associated pull request explaining what you tried
  and asking for human help. Do not spin indefinitely.

  If you push a fix, post a brief comment explaining what was wrong
  and what you changed. Then stop — the next CI run will re-trigger
  you if the fix didn't work.

terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.head.ref
          operator: matches
          value: "^zira/"
    silent: true
```

### Commentary

**Why dedicated sandbox**: this agent actually does work — checks out code, runs tests, makes commits. Shared sandboxes don't give it the isolation it needs. Dedicated sandboxes cost more per run but are the only sensible choice here.

**Why filter forks out**: CI runs on PRs from forks don't have push access to the fork's branch. The agent can't push a fix there. Excluding forks avoids wasted runs.

**Why the `^zira/` branch prefix filter**: this is how the agent knows "this is one of my branches" without needing to track ownership in a database. Any branch prefixed `zira/` is the agent's territory. Other branches belong to humans and the agent shouldn't touch them.

**Why "under 5 tool calls"**: runaway prevention. Without a cap, the agent can get lost in a diagnostic rabbit hole, spending huge amounts of tokens trying to solve an unsolvable problem. Five tool calls is a soft cap the agent is asked to respect; it's not enforced by the trigger system, but it's a clear instruction the LLM will usually honor.

**Why post-instructions "Then stop"**: prevents the agent from over-engineering the fix or trying multiple approaches in one run. The next CI run will re-trigger the agent if needed; incremental progress beats getting stuck.

**Why silent terminate on PR close**: by the time the PR closes, any CI history is irrelevant. No need for a goodbye run.

**What to watch out for**: this agent is authorized to push commits. If it starts making wrong fixes, the cost is real — broken main branches, merge conflicts, user frustration. A good safety addition: an explicit rule in the system prompt that says "if you've pushed three fixes in the last hour and CI is still failing, stop and ask for help." Circuit breakers matter here more than for comment-only agents.

## Example 4: Autonomous coding agent (Linear → PR)

**One-sentence scope**: "This agent exists to pick up Linear issues assigned to it, implement the requested changes, and open a pull request; it stops when the PR is merged or closed."

This is the most ambitious example in the playbook. It's the agent we discussed in [06-multi-provider-patterns.md](../06-multi-provider-patterns.md). It spans multiple trigger rows because it reacts to events from two providers (Linear and GitHub).

**Agent setup**:
- Name: `linear-coder`
- Sandbox type: **`dedicated`** — heavy lifting, needs full environment
- System prompt: long, describes the full workflow: read issue → plan → implement → test → open PR → respond to reviews → push fixes → celebrate merge

**Trigger row 1 — pick up Linear issue**:

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
      value: zira-coder@company.com
    - path: data.labels.*.name
      operator: one_of
      value: [zira-auto, auto-implement]

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

  ## Issue
  Title: {{$issue.title}}
  Description: {{$issue.description}}
  Status: {{$issue.state.name}}
  Priority: {{$issue.priority}}

  ## Prior comments
  {{$comments}}

  Read the issue carefully. If the requirements are clear, begin
  implementation following your system prompt workflow. If the
  requirements are ambiguous, post a comment on the Linear issue
  asking specific questions and stop — do not begin implementation
  until you have answers.

  When you start implementation, post a status comment on the Linear
  issue acknowledging what you're working on. When you open a PR, post
  another comment linking to it.

  Your git branches should be prefixed `zira/linear/` followed by the
  issue identifier, e.g., `zira/linear/lin-123`. This is how the
  follow-up triggers know the PRs are yours.
```

**Trigger row 2 — respond to PR reviews**:

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
      value: linear-coder[bot]
    - path: sender.type
      operator: not_equals
      value: Bot

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
  A reviewer left feedback on PR #$refs.pull_number in $refs.repository
  (a PR you opened from a Linear task).

  ## Pull Request
  {{$pr.title}}
  Base: {{$pr.base.ref}} ← Head: {{$pr.head.ref}}

  ## Files
  {{$files}}

  ## Review comments on this PR
  {{$review_comments}}

  Read the feedback carefully. For each concrete request for changes:
  - If you agree, plan the edits, make them, push a new commit
  - If you disagree, leave a reply explaining your reasoning and ask
    for confirmation before proceeding

  Post a single summary comment when you push new commits explaining
  what you changed. Do not request a re-review — the reviewer will
  re-review if/when they're ready.

  If you've already pushed 3+ fix commits on this PR in the last hour,
  stop and ask for human guidance instead of pushing another. Rapid
  iteration is a sign something is wrong with your approach.
```

**Trigger row 3 — respond to CI failures (reuses the ci-fixer pattern)**:

```yaml
connection_id: <github-connection-uuid>
trigger_keys:
  - workflow_run.completed

conditions:
  match: all
  rules:
    - path: workflow_run.conclusion
      operator: one_of
      value: [failure, timed_out]
    - path: workflow_run.head_branch
      operator: matches
      value: "^zira/linear/"

context:
  - as: run
    action: actions_get_workflow_run
    ref: workflow

instructions: |
  CI failed on $refs.workflow_name for a branch you own.

  {{$run}}

  Investigate, fix, push. Same rules as PR review responses: no more
  than 3 fixes per hour without asking for help.

terminate_on:
  - trigger_keys: [pull_request.closed]
    conditions:
      match: all
      rules:
        - path: pull_request.head.ref
          operator: matches
          value: "^zira/linear/"
    silent: true
```

### Commentary

**Why three trigger rows on one agent**: each row has different conditions, different context, and different instructions, but they're all facets of the same job. Splitting into three rows is cleaner than one mega-row with per-event branches.

**Why branch prefix as the ownership signal**: the agent creates PRs from branches named `zira/linear/<issue-id>`. Any other agent or human who creates a similarly-named branch would confuse this, so make sure the prefix is unique to the agent. A better alternative is to use a combination (branch prefix + PR author) to double-check, but branch prefix alone is usually sufficient.

**Why no cross-provider conversation continuation**: this is the limitation discussed at length in [../06-multi-provider-patterns.md](../06-multi-provider-patterns.md). The Linear conversation and the PR conversation are separate threads. The agent re-orients on the PR by reading the PR body (which contains the Linear issue link) and can use its Linear tools mid-run if it needs more context.

**Why the "3 fixes per hour" soft limit**: runaway prevention. Without it, a broken test that the agent can't diagnose leads to a push-fail-push-fail loop. Three pushes per hour is arbitrary but reasonable — fast enough to make real progress, slow enough to catch unproductive thrashing.

**Why the terminate rule**: once the PR is closed or merged, the agent has nothing more to do with it. Silent termination avoids a pointless "goodbye" comment on a merged PR.

**What to watch out for**: this agent has the most complex scope in the gallery. Early versions will misbehave in unexpected ways. Start with narrow permissions (maybe only certain repos, certain labels) and expand as the agent proves itself. Never give it merge permissions unless you have circuit breakers in place. Review its PR output manually for the first 50+ runs before trusting it to run unattended.

## Example 5: Customer support reply drafter

**One-sentence scope**: "This agent exists to draft replies to customer messages in Intercom conversations, save them as notes for admin review, and stop when an admin takes over or the conversation closes."

**Agent setup**:
- Name: `support-drafter`
- Sandbox type: `shared`
- System prompt: tone guidelines, company-specific knowledge (product names, policies, common FAQs), explicit instructions to NEVER send messages directly

**Trigger row**:

```yaml
connection_id: <intercom-connection-uuid>
trigger_keys:
  - conversation.user.replied
  - conversation.user.created

conditions:
  match: all
  rules:
    - path: data.item.state
      operator: equals
      value: open
    - path: data.item.conversation_rating
      operator: not_exists

context:
  - as: conversation
    action: retrieve_conversation
    params:
      conversation_id: $refs.conversation_id
      display_as: plaintext

instructions: |
  A customer sent a message in an Intercom conversation.

  ## Conversation
  ID: {{$conversation.id}}
  Subject: {{$conversation.title}}
  Assigned admin: {{$conversation.admin_assignee_id}}

  ## Full message history
  {{$conversation.conversation_parts}}

  Read the latest customer message in context. Draft a helpful,
  concise, empathetic reply. Do NOT send it directly — save it as
  a note on the conversation so the assigned admin can review
  before posting.

  If the customer is asking a factual product question you're
  confident about, draft a direct answer. If they're describing a
  bug, draft a response that acknowledges the issue and asks for
  details (error messages, reproduction steps, browser/OS). If
  their question is outside your scope (billing disputes, account
  changes), say so in the draft and recommend escalating to the
  right team. Never fabricate policy, pricing, or technical facts.

  Start the draft with "[AI DRAFT]" so admins know it wasn't
  human-written. End with a blank line so admins can easily edit.

terminate_on:
  - trigger_keys: [conversation.admin.replied]
    silent: true
  - trigger_keys: [conversation.admin.closed]
    silent: true
```

### Commentary

**Why only `open` conversations**: closed conversations don't need agent attention. The condition filters them out.

**Why `conversation_rating not_exists`**: if the customer already rated the conversation, it's effectively done. Drafting more replies would be wasted work.

**Why "save as a note, not send directly"**: this is a safety-first design. The agent never interacts with customers directly. An admin always reviews before anything goes out. This makes the agent useful without making it high-risk. Many support drafter agents start this way and graduate to direct-send after a trust period.

**Why terminate on admin reply or close**: as soon as a human takes over, the agent's job is done. Silent close because there's nothing useful to say at handoff.

**Why `retrieve_conversation` as the only context action**: this one fetch gives the agent the full message history, admin assignment, and conversation metadata. It's enough to draft a reply.

**Why the tone and scope reminders in instructions**: LLMs can drift on tone when they see angry customer messages or complex questions. Reinforcing the tone and scope in every run's instructions catches drift.

**What to watch out for**: the biggest risk is the agent being authoritative about things it shouldn't be (pricing, policy, legal). The system prompt should EXPLICITLY forbid this, and the instructions reinforce it. A good monthly audit: review the most recent 50 drafts and look for factual errors or tone problems. Correct the system prompt as drift is discovered.

Also watch for: the Intercom catalog currently lacks full refs/ref_bindings support (see [../09-known-limitations.md](../09-known-limitations.md)). This YAML assumes refs like `$refs.conversation_id` exist — if they don't in your catalog yet, you'll need to add them before this agent works.

## Reading the examples as a gallery

If you're designing an agent, pick the example closest to your use case and adapt it. The patterns reuse:

- **Self-exclusion conditions** appear in every example
- **Optional context fetches** appear for anything that might be missing (CONTRIBUTING.md, prior comments, similar issues)
- **Silent termination** appears when there's no value in a goodbye message
- **Graceful termination** appears when the agent has something meaningful to say at close
- **Branch-prefix ownership signals** appear in every multi-event GitHub agent
- **Circuit-breaker language in instructions** appears in every action-taking agent ("stop after N attempts")
- **Explicit terminal actions** appear in every instructions block

These patterns aren't accidents — they're the shapes that emerged from building and debugging real agents. Following them doesn't guarantee a good agent, but it avoids the bulk of the common mistakes.

## Where to go from here

- The design principles that drive these choices: [01-agent-design-principles.md](01-agent-design-principles.md)
- The safety patterns every example relies on: [04-safety-and-loop-prevention.md](04-safety-and-loop-prevention.md)
- Common mistakes to watch for when adapting these: [08-anti-patterns.md](08-anti-patterns.md)
