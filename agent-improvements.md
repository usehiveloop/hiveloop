# Agent improvements — audit of conversation `495e803a-0c58-4371-87ad-ee641b88f00a`

Kira's full processing of the `extractErrorMessage` build-failure incident on
2026-04-19. Stallone had already filed a well-written issue with root cause,
every affected file, a recommended fix, and the exact failing build output.
The ask was effectively: *"make this param optional and call it a day."*

Despite that, Kira took **24 turns**, **~42 minutes**, **91 tool calls**,
consumed **5.5M input tokens**, and triggered **11 `agent_error` events**.
This doc catalogs every improvement we should make, ordered by impact.

---

## Top-line numbers

| Metric | Value | Note |
|---|---|---|
| Turns | 24 | |
| Tool calls | 91 | 30 bash, 15 Read, 10 RipGrep, 11 memory_retain, 7 journal_write, 6 memory_recall, 2 AstGrep, 1 failed sub_agent… |
| Cumulative input tokens | 5,513,663 | |
| Cumulative output tokens | 3,950 | |
| Cache hit ratio (every turn) | **0.0** | Zero caching, 24/24 turns |
| `agent_error` events | 11 | All `tool_requirement_violated` — no real errors |
| `chain_started` events | 1 | Triggered at 141,926 tokens (our 70% budget) |
| `message_received` events | 26 | 1 trigger + 25 async webhooks flooding the conversation |
| History messages (peak) | 162 | Before the single chain reset that dropped it to 10 |
| Sub-agent invocations | 1 | `codebase-explorer`, **is_error=true, 120s wall** |
| Peak-turn input tokens | 2,102,179 | Turn 15 — ingesting the CI webhook flood in one shot |
| Peak-turn latency | 408s | Turn 0 (initial exploration) |
| Cost at $1/$3.2 per 1M (glm-5.1) | **~$5.52** | For a bug fix Stallone had already solved on paper |

---

## 1. Zero prompt cache hits across 24 turns [CRITICAL]

**Observation.** Every `turn_completed` event reports `cumulative_cache_hit_ratio: 0.0`. We sent 5.5M input tokens and cached **none** of them. On a provider that offers cache reads at ~10% of normal input price, this alone is a ~9× cost multiplier.

**Likely cause.** Something at the head of the prompt changes every turn — the most common culprits are:

1. A timestamp / request id baked into the system prompt.
2. Webhook messages inserted into history whose content is unique per turn (cache prefix breaks on the very first divergent token).
3. Bridge's `<system-reminder>` blocks (for immortal, tool-requirements reminders, attachments) getting inserted before stable content.
4. The immortal `carry_forward` block being computed fresh per turn instead of pinned.
5. glm-5.1 on openrouter may not actually support prompt caching — OpenRouter's caching story is provider-specific. Worth confirming in Bridge's request inspector.

**What to do.**
- Audit the prompt-prefix that Bridge sends to the model. Anything non-stable within the first ~1k tokens kills caching for the whole turn.
- Instrument: log `input_tokens` **and** `cached_input_tokens` per turn at Bridge. If all providers return 0, it's our prompt. If some do, the shape is fine and it's provider-side.
- Verify glm-5.1 via openrouter reports `cached_tokens` at all. Fall back to Gemini 2.5 Flash or Anthropic Haiku for checkpoint and compare.

---

## 2. Tool-requirement violations are all false positives [HIGH]

**Observation.** All 11 `agent_error` events are `tool_requirement_violated` at turns 6, 12, 18, 21, 24 — exactly on our cadence of 5. In every case the missing tool was `memory_recall`, `memory_retain`, or `journal_write`. Kira called these tools **28 times total** (recall 6, retain 11, journal 7+) — just not in the windows Bridge expected.

**Likely cause.** The "every N turns" cadence enforcement treats a tool-call as satisfying the current window only if it happens inside that window's turn bounds, but we inject the memory/journal requirements *universally* even for agents whose natural flow doesn't align with the turn boundaries. A reasoning-heavy agent that calls `memory_retain` at the end of its work stream naturally slips out of a 5-turn window.

Secondary effect: each violation generates a `<system-reminder>` attached to the next user message. That's extra input tokens and pollutes the context with "you forgot to call X" prose, which pushes the model into defensive tool-calling instead of goal-focused work.

**What to do.**
- **Loosen the cadence to `every_n_turns=10` or drop it entirely** for the default injector. The real signal is "has this agent called memory at all in the last N turns" — 5 is too tight for agents that do exploration-heavy turns.
- **Change enforcement to `warn`** for the default tool-requirement injector, not `next_turn_reminder`. Warn logs the miss without polluting context. Upgrade to reminder only for agents that opt in.
- **Per-agent opt-out** — add a `DisableDefaultToolRequirements bool` to the agent config so agents whose purpose doesn't involve the memory/journal loop (ephemeral one-shot agents, system agents that run a single tool, etc.) can skip the injection entirely.
- Track: how often does a violated requirement actually correlate with agent quality degradation? If zero, the enforcement's value proposition is undermined.

---

## 3. The subscribed conversation received 25 unsolicited webhook events [HIGH]

**Observation.** Between Kira pushing the PR and closing it, the conversation ingested:
- 2× `issue_comment.created` from `x-ziraloop` (legit — user asked Kira to check things)
- 23× `check_run.completed` / `check_suite.completed` / `workflow_run.completed` / `check_run.created` — all from Kira's own PR's CI
- 2× `pull_request.closed` / `issues.closed` at the end

The CI events are the single biggest noise source. GitHub Actions fires 4–6 webhook events **per workflow run** (`check_run.created`, `check_run.completed` per matrix cell, `check_suite.completed`, `workflow_run.completed`), and there are 3–4 workflows per PR. 20+ events per PR.

Each event is a full `# Webhook event:` markdown message appended to Kira's history. Turn 15 alone ingested 2.1M input tokens — almost certainly because it was the first turn after a flood of these events hit the history.

**Likely cause.** Kira's subscription on PR #17 fires on every matching event key (e.g. `check_suite.completed`). The filter is "this event touches my PR", not "this event tells me something new".

**What to do.**
- **Conclusion-aware filtering**: drop `check_*.completed` events where `conclusion=success` — the agent doesn't need to hear about every passing check, only about state *changes* and *failures*. The summary_refs would keep `check_*.completed conclusion=failure|cancelled|timed_out` and drop the rest at dispatch time.
- **Event-key coalescing**: when N+ identical-key events arrive for the same resource within 30 seconds, dispatch a single coalesced "N CI events, all success" summary instead of N separate messages.
- **Completion-only events**: never subscribe to `check_run.created` by default — it's a started-signal that pairs with the completed event, which is always enough on its own.
- **Per-subscription noise knobs**: expose "only notify on failure/changed-state" as a first-class subscription option in the UI.

---

## 4. Two inconsistent webhook message formats confuse the agent [MEDIUM]

**Observation.** The initial `issues.opened` message arrived as **raw catalog-ref dump**:
```
issue_body: ## Summary...
repository_id: 1176924311
organization: ziraloop
sender_id: 273760739
...
```

Subsequent events came through the new markdown envelope:
```markdown
# Webhook event: check_run.completed
- **provider**: github-app
- **resource**: github/ziraloop/ziraloop/pull/17
...
## Summary
...
## Available paths in attached full payload
...
```

Two formats for the same conceptual thing (a webhook delivery) in the same conversation.

**Likely cause.** Trigger dispatch (router trigger → agent) uses the deterministic enrichment pipeline that produces the flat `key: value` dump. Subscription dispatch uses the markdown envelope we built for `full_message` attachments. Different code paths, separate formatting.

**What to do.**
- Move both paths to the markdown envelope so every webhook the agent ever sees has the same shape.
- The initial trigger message should also benefit from `full_message` on-disk attachment (currently the whole enriched payload lives in the user message — it's a 50KB+ blob).
- Same `summary_refs`-driven curation and `payload_paths` outline for both paths.

---

## 5. The chain reset fired too late [MEDIUM]

**Observation.** `chain_started` fired at turn 22/23 with `token_count: 143,626` and `budget: 141,926`. By that point we'd already spent **5.5M cumulative input tokens**. One reset, very late.

**Likely cause.**
- Our 70% budget is measured against the *model's context window*, not against the conversation's cumulative cost. A 202k-window model (glm-5.1) means we wait until ~141k tokens of in-flight prompt before we reset — good for latency-per-turn, bad for cumulative cost.
- No early-reset heuristic for "the conversation has gone on for 20+ turns with lots of tool calls".

**What to do.**
- Reset earlier: 50% of context, not 70%. In the glm-5.1 case: 100k instead of 141k.
- Add a "turn count" or "cumulative input tokens" trigger for immortal reset alongside the "in-flight tokens" one. E.g. reset at `min(0.5 × context_window, 1M cumulative)`. That would have triggered the reset around turn 10–12 here, saving ~3M tokens.
- Once reset fires, the carry-forward ("3 messages, 4.5k tokens") is excellent — the checkpoint extraction worked. We just did it once, too late.

---

## 6. Sub-agent failed silently after 120s wasted [MEDIUM]

**Observation.** At turn 12, Kira invoked a `codebase-explorer` subagent with the prompt *"search for functions that changed signatures and broke callers"* — a meta-task, not the fix itself. The subagent ran exactly 120,001 ms (hit a 120s timeout) and returned `is_error: true`. Kira moved on without surfacing what happened.

**Likely cause.**
- Scope creep inside the agent: Kira decided to look for "similar bugs" when Stallone's issue had already listed every affected file.
- Sub-agent timeout is lenient (120s) and its failure doesn't bubble informative detail to the parent. Parent just sees "it errored" and keeps going.

**What to do.**
- Make sub-agent timeouts configurable and default to 45s for exploration subagents, 180s for code-writing ones. 120s is the worst default — long enough to block, short enough to be useless.
- Bubble sub-agent error content into the parent's context so it can react (retry, change approach, or bail). Currently it's effectively invisible.
- This is also a system-prompt fix for Kira: when the issue lists the affected files explicitly, skip the sub-agent discovery phase.

---

## 7. History message count grows unboundedly (44 → 162) [MEDIUM]

**Observation.** History messages per turn: 44, 47, 50, 53, 56, 59, 66, 69, 72, 75, 78, 81, 91, 94, 97, 132, 137, 142, 148, 151, 154, 159, 162. The +35 jump between turns 14 and 15 is the CI webhook flood being bulk-appended.

At 162 messages, `history_tokens_estimate` was ~143k. Each turn pays this as a fresh input. Multiply by 24 turns = the 5.5M we saw.

**What to do.**
- `HistoryStrip` config exists on `AgentConfig` but we don't auto-inject it for non-system agents. It strips old tool-result bodies from LLM input while keeping a recoverable spill file on disk. Auto-inject for every agent the same way we do immortal + tool requirements.
- Tune strip depth to "keep last 5 tool results, strip the rest". That would have cut the history tokens on every turn by 60–80%.
- For tool results that are also stored via `full_message` attachment, the spill is redundant — HistoryStrip integrates nicely here.

---

## 8. No visible recovery from `journal_write` disabled [LOW]

**Observation.** Stallone had `journal_write` in `disabled_tools` (existing agent config) — we just fixed this in commit `4571a64`. But Kira does **not** have it disabled, so Kira auto-gets the journal requirement and calls it 7 times. Fine here, but:

If the agent author later disables journal_write on Kira, the next push silently drops the requirement. Good. But they won't see why the memory-loop they thought they had is now partial.

**What to do.**
- Surface in the admin UI: which default tool requirements are active vs. disabled for this agent, with the reason ("disabled_tools contains journal_write → journal_write requirement skipped"). Make the interaction visible.

---

## 9. Turn 15 latency was 295 seconds (5 min) [LOW, symptom]

**Observation.** Turn 15 took 295s. Input: 2.1M tokens, output: 1.2k. That's the first turn after the CI event flood. The model spent 5 minutes reading 2.1M tokens of mostly-boring webhook notifications to produce a tiny output.

This is a symptom of #3 (webhook spam) and #1 (no caching) compounding.

**What to do.** Fixed by #3 + #1.

---

## 10. Agent did redundant exploration despite a complete fix spec [LOW, prompt]

**Observation.** Stallone's issue body:
- Listed every affected file with line numbers
- Gave the exact recommended fix (make `context` optional)
- Explained why that preserves intent

Kira still:
- Ran `tsc` twice (seq 29, 43) — redundant with the issue's build log
- Did `ls`, `cat package.json`, checked `ziraloop/ziraloop`, `apps/web`, `lib/` structure (seqs 31, 33, 35)
- Installed dependencies (seq 37 — took 1m 30s)
- Grepped for call sites (seq 516) — Stallone already listed them
- Invoked a codebase-explorer subagent (seq 417) — meta-investigation

Lots of verification work that would be fine if the issue was vague. Here the issue was crystal clear.

**What to do.**
- Prompt-level: when the incoming issue provides a "Recommended fix" section with code, try that fix FIRST, verify with `tsc`, and only explore if the fix doesn't work. A two-line branch in Kira's system prompt.
- Structural: the issue template Stallone produces could include a `[x] Fix verified locally` checkbox that Kira reads as a signal to skip exploration when checked. Feeding the fix-spec forward.

---

## 11. No conversation status transitions [LOW]

**Observation.** The conversation record's `status` is still `"active"` and `name` is `null`. We don't auto-title conversations, and we don't auto-close them when their resource is closed (`pull_request.closed` fired — we could've ended the conversation there).

**What to do.**
- Auto-name conversations after the first turn (cheap LLM call, already considered in the notes.txt).
- When a subscribed resource transitions to a terminal state (PR merged, issue closed, deployment restored), emit an `end` / `archive` signal to the conversation so it stops accepting webhooks. Otherwise every future webhook for that PR keeps landing on this conversation forever.

---

## 12. We send full webhook bodies to bridge even when the agent subscribed to a summary [LOW]

**Observation.** Each `# Webhook event: check_run.completed` message is ~1.2–1.5KB. With 20+ CI events, that's ~30KB of webhook notifications per PR. Then on top of that, `full_message` ships the entire raw 10k+ JSON payload as an attachment (agent can read on demand).

So every CI event lands twice: as a summary in context, AND as an attachment on disk. For successful routine CI events, neither is actionable. We're paying to send both.

**What to do.**
- **Don't ship `full_message` for events whose summary_refs cover everything** the agent could need. Today we always attach; a conclusion=success check_run has no useful detail beyond what's in the summary.
- Per-event-type `skip_attachment: true` flag in the catalog, defaulting on for high-frequency success events (check_run, check_suite, workflow_run when conclusion=success).

---

## Prioritized fix list

### Tier 1 — biggest cost impact
1. **Find and fix the zero cache-hit problem** — audit the prompt prefix, confirm provider supports caching. ~9× cost multiplier.
2. **Filter success-conclusion CI events** at dispatch time. This alone would have dropped ~20 messages from this conversation and prevented the 2.1M-token turn 15.
3. **Auto-inject HistoryStrip** on every pushed agent (just like we do immortal). Cuts history tokens per turn by 60–80%.
4. **Reset immortal at 50% budget (not 70%)** + add a cumulative-tokens trigger.

### Tier 2 — correctness / UX
5. **Relax tool-requirement defaults** — cadence to 10 turns, enforcement to `warn`, add per-agent opt-out.
6. **Unify trigger-dispatch and subscription-dispatch message formats** (both on markdown envelope).
7. **Ship `full_message` only for events that don't have a complete `summary_refs`** (or explicit failure events).
8. **Coalesce same-key events** arriving within 30s for the same resource.

### Tier 3 — nice-to-have
9. Shorter sub-agent default timeout (45s) + bubble errors to parent.
10. Auto-name conversations after turn 0.
11. End conversations when subscribed resource closes.
12. Surface "default tool requirement disabled because of disabled_tools" in the admin UI.
13. System-prompt tweak for Kira: try recommended fix first, explore only if needed.

### Tier 4 — observability
14. Per-turn cache-read token logging (separate from input_tokens) for every provider.
15. Dashboard: cost-per-conversation, cache-hit rate, chain-reset rate.
16. Alert when a conversation exceeds a cumulative-token threshold (e.g. 2M).
