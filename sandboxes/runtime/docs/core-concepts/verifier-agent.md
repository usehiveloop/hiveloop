# Verifier Agent

A second, small LLM that judges, after every terminal-text turn, whether your main agent really finished or stopped prematurely. If it stopped early, the verifier injects a synthetic user message to nudge it forward — without the main agent realizing the prompt was machine-generated.

Off by default. Opt in per agent via `config.verifier`.

---

## Why It Exists

Smaller / cheaper models routinely hand the conversation back to the user when they shouldn't. Common patterns:

- "I've drafted the migration. Let me know if you'd like me to run it."
- "I've added the new endpoint. Next, I'd implement the tests — should I proceed?"
- "Done — here's a summary of the changes." (without actually running the tests the user asked for)

Each of those is a turn that *looks* complete but isn't. A human catching this is the standard failure mode. The verifier automates the catch: a fast classifier reads the conversation and either lets the turn finalize or fires a synthetic follow-up that pushes the agent to keep going.

---

## How It Works

The verifier runs once per "session" — a session is one full pass through `rig`'s internal multi-turn loop, which can include many tool calls. When that loop returns to bridge with a terminal text response, bridge calls the verifier *before* finalizing the turn.

```
agent emits text → bridge runs verifier → verdict
                                          ├── completed / users_turn → finalize, emit turn_completed
                                          └── needs_work + high      → inject synthetic user message,
                                                                       resume the same turn
```

The verifier sees the agent's own system prompt, every user message, every assistant text response, and every tool call (name + arguments) and tool result — with assistant text, arguments, and results head/tail-elided at 1000 characters per side so a single huge tool result doesn't blow up the verifier's context.

---

## Configuration

Attach to any agent via `config.verifier`:

```json
{
  "id": "my-coding-agent",
  "name": "Coding Agent",
  "system_prompt": "...",
  "provider": { "...": "..." },
  "config": {
    "max_turns": 100,
    "verifier": {
      "enabled": true,
      "primary": {
        "provider": "open_ai",
        "model": "openai/gpt-5.4-nano",
        "api_key": "${OPENROUTER_API_KEY}",
        "base_url": "https://openrouter.ai/api/v1"
      },
      "max_reprompts_per_turn": 2,
      "require_high_confidence": true,
      "blocking": true,
      "max_input_tokens": 300000,
      "timeout_ms": 20000
    }
  }
}
```

### `VerifierAgentConfig` Fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `enabled` | No | boolean | Master switch. When `false`, the block is parsed but ignored. | `false` |
| `primary` | Yes (when enabled) | object | The model that produces the verdict. See `VerifierModel` below. | — |
| `fallback` | No | object | Reserved for a future second-tier model. Not used in current implementation. | — |
| `max_reprompts_per_turn` | No | integer | Hard cap on synthetic re-prompts per terminal turn. After the cap, the next verdict is treated as `proceed` regardless of content — prevents runaway loops. | `2` |
| `require_high_confidence` | No | boolean | When `true`, only `needs_work + high` triggers a re-prompt; `needs_work + low` is treated as `proceed`. Recommended on. | `true` |
| `blocking` | No | boolean | Reserved. Currently ignored (the verifier always blocks turn finalization while it runs). | `true` |
| `max_input_tokens` | No | integer | Soft cap on the projection size sent to the verifier. Today informational; the projection is sized by char count + elision, not by this field. | `10000` |
| `timeout_ms` | No | integer | HTTP timeout for the verifier API call. On timeout the verifier emits `verifier_error` and proceeds. | `5000` |

### `VerifierModel` Fields

| Field | Required | Type | Description | Default |
|-------|----------|------|-------------|---------|
| `provider` | Yes | string | One of `open_ai` (any OpenAI-compatible endpoint, including OpenRouter, Together, Fireworks, vLLM, etc.) or `gemini`. | — |
| `model` | Yes | string | Model identifier as the upstream expects it (e.g. `openai/gpt-5.4-nano` on OpenRouter, `gpt-5-nano` direct). | — |
| `api_key` | Yes | string | API key. Supports `${ENV_VAR}` substitution — the variable is resolved at conversation-start time from bridge's process environment. | — |
| `base_url` | No | string | Override the upstream URL. Required when `provider = open_ai` points at anything other than OpenAI's own endpoint. For Gemini, currently required (the OpenAI-compatible endpoint is used). | OpenAI default |

> **Wire-format note:** `provider` is `"open_ai"` (with an underscore), matching the main agent's `ProviderType::OpenAI` wire format. The natural `"openai"` is **not** accepted — bridge's serde rename keeps both enums consistent. See [the OpenAI variant in `crates/core/src/provider.rs`](https://github.com/useportal/bridge/blob/main/crates/core/src/provider.rs) for the source of truth.

---

## Verdict Shape

The verifier model is constrained by a frozen JSON schema and always returns:

```json
{
  "verdict": "users_turn" | "completed" | "needs_work",
  "confidence": "low" | "high",
  "instruction": "..."
}
```

- **`users_turn`** — the agent reasonably handed control back. Asked a clarifying question, answered a non-task message, or acknowledged completion and is awaiting the next instruction.
- **`completed`** — the agent finished a task and stated so explicitly.
- **`needs_work`** — the agent stopped before genuinely finishing what was asked.
- **`confidence`** — `high` when evidence is unambiguous, `low` for terse-but-correct or borderline cases. With `require_high_confidence: true`, `low` is treated like `users_turn`.
- **`instruction`** — populated only on `needs_work`. A second-person directive listing what the agent skipped and what to do next. Empty string for the other verdicts.

### How the Reprompt Is Synthesized

When the verdict is `needs_work + high` and the cap is not exhausted, bridge appends a synthetic user message of the form:

```
The previous turn was flagged as incomplete by the verifier.

<verifier's instruction text, verbatim>
```

That message is pushed onto rig's history *and* persisted to storage, then the same turn resumes. The agent sees what looks like a normal user follow-up and continues working.

---

## What the Verifier Sees

Sensitive question: can the verifier be poisoned by tool output? The projection is deliberately structured so it sees what's needed to write a precise instruction, but elides oversized payloads:

| Message kind | What's projected |
|--------------|------------------|
| User text | Verbatim, never elided |
| Assistant text | Verbatim if ≤ ~2000 chars; otherwise head 1000 + `[middle elided]` + tail 1000 |
| Tool call | `{ name, arguments }` — same elision rule on the JSON-serialized arguments |
| Tool result | Body — same elision rule |
| System messages | Never projected (the agent's system prompt is sent as a separate field) |

The total user payload is built as:

```
## Agent system prompt
<the agent's own system prompt>

## Conversation
<JSON-serialized projected messages>
```

Plus a stable verifier system prompt + frozen verdict JSON schema (defined as constant byte sequences in `llm::VERIFIER_SYSTEM_PROMPT` / `llm::VERIFIER_VERDICT_SCHEMA`). Those bytes never change between calls, so OpenAI's prefix cache hits from the second call onward. The cache identifier is exposed as `prefix_hash` in the `verifier_verdict` event for observability.

---

## Telemetry

Every verifier invocation emits SSE events on the conversation stream and webhook events to subscribers:

- **`verifier_started`** — fired immediately before the upstream call.
- **`verifier_verdict`** — fired with the parsed verdict, model used, latency, input / cached / output tokens, and the cache `prefix_hash`.
- **`verifier_error`** — fired on parse failure, HTTP error, or timeout. The conversation continues as if the verdict was `proceed` (errors never block).

See [SSE Events → Verifier Events](../api-reference/sse-events.md#verifier-events) for the exact field shapes.

---

## Recommended Configurations

**Light supervision** — fast model, narrow scope. Good for cheap nudge-when-needed:

```json
{
  "verifier": {
    "enabled": true,
    "primary": {
      "provider": "open_ai",
      "model": "openai/gpt-5.4-nano",
      "api_key": "${OPENROUTER_API_KEY}",
      "base_url": "https://openrouter.ai/api/v1"
    },
    "max_reprompts_per_turn": 1,
    "require_high_confidence": true,
    "timeout_ms": 10000
  }
}
```

**Aggressive completion-pushing** — more reprompts allowed, longer timeout:

```json
{
  "verifier": {
    "enabled": true,
    "primary": {
      "provider": "open_ai",
      "model": "openai/gpt-5-mini",
      "api_key": "${OPENAI_API_KEY}"
    },
    "max_reprompts_per_turn": 3,
    "require_high_confidence": false,
    "timeout_ms": 30000
  }
}
```

> Setting `require_high_confidence: false` raises the false-positive rate — agents will get re-prompted on borderline calls. Use only if the agent is robust to occasional unwarranted nudges.

---

## Operational Notes

- **`BRIDGE_VERIFIER_DISABLED=1`** in bridge's environment force-disables the verifier for all conversations regardless of agent config. Use to kill verifier traffic in an outage without redeploying agent definitions.
- **Build failures are non-fatal.** If the verifier client can't construct (bad URL, missing env var, malformed config), bridge logs a warning and disables verification for that conversation only. The agent runs unaffected.
- **Verifier-call failures are non-fatal.** Timeouts, HTTP errors, and parse failures emit `verifier_error` and proceed as if the verdict was `users_turn`. The verifier never blocks the agent.
- **One call per session, not per tool call.** A "session" is one pass through rig's multi-turn loop. An agent making 50 tool calls in a single session triggers exactly one verifier call. If you want per-tool verification, that's a much larger architectural change.
- **Cost.** Per call: typically 5k–30k input tokens (the projection) + ~20 output tokens (the JSON verdict). On `gpt-5.4-nano` at OpenRouter prices, that's fractions of a cent. Cache hits on the schema prefix reduce cost further on subsequent calls in the same conversation.

---

## See Also

- [Agents → Configuration](agents.md#verifier)
- [SSE Events → Verifier Events](../api-reference/sse-events.md#verifier-events)
- [Environment Variables → BRIDGE_VERIFIER_DISABLED](../reference/environment-variables.md#bridge_verifier_disabled)
