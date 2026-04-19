#!/usr/bin/env node
// Real-world immortal/checkpoint test for bridge.
//
// Boots the bridge binary, pushes an immortal coding agent backed by OpenRouter
// (model: openrouter/elephant-alpha), gives it a multi-file coding task in a
// temp workspace, and streams SSE events while tracking:
//   - per-turn tool calls + content deltas
//   - chain_started / chain_completed events (token_count, chain_index, journal size)
//   - cumulative tokens from /metrics
//   - journal entries written by the agent
//   - continuity verification after chain handoff
//
// Run:
//   OPENROUTER_API_KEY=sk-or-... node scripts/immortal-real-test.mjs

import { spawn } from "node:child_process";
import { createWriteStream, mkdirSync, existsSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import net from "node:net";

// ────────────────────────────────────────────────────────────────────────────
// Config
// ────────────────────────────────────────────────────────────────────────────

const OPENROUTER_API_KEY = process.env.OPENROUTER_API_KEY;
if (!OPENROUTER_API_KEY) {
  console.error("OPENROUTER_API_KEY is required");
  process.exit(2);
}

const BRIDGE_REPO = resolve(new URL(".", import.meta.url).pathname, "..");
const BRIDGE_BIN = join(BRIDGE_REPO, "target", "debug", "bridge");
const BRIDGE_API_KEY = "immortal-test-bearer";
const AGENT_ID = "immortal-coder";
const MODEL = "moonshotai/kimi-k2.5";
const OPENROUTER_BASE = "https://openrouter.ai/api/v1";

const TOKEN_BUDGET = 5_000; // immortal trigger threshold — small so chain fires in ~2-3 turns
const CARRY_FORWARD_TURNS = 1;

// Agent model is elephant-alpha (user-requested). Checkpoint extraction uses
// a separate, reliable model via the same OpenRouter key — elephant-alpha's
// aggressive upstream rate limit would otherwise cause the checkpoint call
// itself to 429 and abort the handoff mid-flight.
const CHECKPOINT_MODEL = "google/gemini-2.5-flash";

// Gemini-tuned checkpoint prompt. Based on Google's own prompt-design docs:
// - critical rules & role in the SYSTEM prompt first
// - XML-style tags + explicit markdown template
// - per-section length caps (Gemini doesn't self-limit without them)
// - explicit PRUNE directive to counter iterative-refinement drift (Google's
//   docs prefer map/reduce for long-doc summarization for this exact reason;
//   our immortal design is inherently iterative, so we enforce discipline here)
// - "start directly with ## Overall Goal" to suppress Gemini's preamble tic
const GEMINI_CHECKPOINT_PROMPT = `<role>
You are a conversation-checkpoint extractor. Your only job: compress a completed \
portion of an LLM conversation into a DENSE structured checkpoint that another \
assistant will read in a fresh context window to continue the work seamlessly. \
The user never sees your output — it is prompt-context injection only.
</role>

<hard_rules>
1. Your entire output MUST be under 900 tokens. Longer output is a failure.
2. Produce EXACTLY the 7 sections below, in order, using the exact markdown \
headings shown. No preamble, no closing remarks. Start your response DIRECTLY \
with the line "## Overall Goal".
3. When a <previous_checkpoint> is supplied, you MUST actively PRUNE it. DELETE \
items fully superseded by later decisions. DELETE items marked DONE more than \
one chain ago. MERGE near-duplicate bullets. Do NOT preserve content "for \
completeness" — the whole point of a checkpoint is compression, not accumulation.
4. The Overall Goal is the user's CONVERSATION-WIDE objective from the earliest \
user turns. Never narrow it to the most recent topic, even if recent turns focus \
on one sub-area.
5. Every bullet is a concrete specific fact: library names with parameters, \
numeric constants, file paths, decisions with reasons. Forbidden words in bullets: \
"discussed", "covered", "explored", "looked at", "considered". Write the \
conclusion, not the activity.
6. If a section has no relevant content write "- (none)". Do not invent content.
</hard_rules>

<output_template>
## Overall Goal
One sentence, max 30 words — the conversation-wide objective.

## Active Constraints
Bullets. Max 8 items, each ≤20 words. Rules/preferences/limits still binding.

## Key Knowledge
Bullets. Max 12 items, each ≤25 words. Concrete technical facts (libraries + \
versions + parameters, schema details, endpoints, file paths, numeric constants). \
No generalities.

## Work Trail
Bullets. Max 8 items, each in the form "\`<artifact>\`: <what changed + why>". \
Drop items fully superseded.

## Key Decisions
Bullets. Max 8 items, each ≤25 words. Named decisions + one-phrase rationale.

## Task State
Numbered list. Each line tagged [DONE] / [IN PROGRESS] / [TODO]. Mark exactly one \
[IN PROGRESS] with "<-- CURRENT FOCUS".

## Transition Context
One paragraph, 2-3 sentences, ≤70 words. Address the assistant directly: where \
things stopped, what to do next.
</output_template>

<pruning_discipline>
When previous_checkpoint(s) are present:
- KEEP: decisions still governing forward work; facts still needed; constraints \
not yet met.
- DROP: tasks marked DONE more than one chain ago; details superseded by later \
decisions; duplicated bullets; narrative framing; prior Transition Context \
paragraphs.
- MERGE: near-duplicate bullets into one tighter bullet.
</pruning_discipline>

Read the entire conversation and any previous checkpoints below, then produce \
your checkpoint. Remember: start with "## Overall Goal" on the first line.`;

const ts = new Date().toISOString().replace(/[:.]/g, "-");
const WORKSPACE = join(tmpdir(), `bridge-immortal-test-${ts}`);
const LOG_DIR = join(tmpdir(), `bridge-immortal-logs-${ts}`);
mkdirSync(WORKSPACE, { recursive: true });
mkdirSync(LOG_DIR, { recursive: true });

// ────────────────────────────────────────────────────────────────────────────
// Colored logging
// ────────────────────────────────────────────────────────────────────────────

const C = {
  reset: "\x1b[0m",
  dim: "\x1b[2m",
  bold: "\x1b[1m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
};

const stamp = () => {
  const d = new Date();
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}:${String(d.getSeconds()).padStart(2, "0")}.${String(d.getMilliseconds()).padStart(3, "0")}`;
};

const log = (color, tag, msg) => {
  console.log(`${C.dim}${stamp()}${C.reset} ${color}${tag}${C.reset} ${msg}`);
};

const step = (msg) => log(C.cyan + C.bold, "STEP", msg);
const info = (msg) => log(C.blue, "INFO", msg);
const ok = (msg) => log(C.green, "OK  ", msg);
const warn = (msg) => log(C.yellow, "WARN", msg);
const err = (msg) => log(C.red, "ERR ", msg);
const evt = (type, msg) => log(C.magenta, `SSE ${type.padEnd(18)}`, msg);

// ────────────────────────────────────────────────────────────────────────────
// Utilities
// ────────────────────────────────────────────────────────────────────────────

async function findFreePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, "127.0.0.1", () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

async function waitForHealth(base, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const r = await fetch(`${base}/health`);
      if (r.ok) return;
    } catch {}
    await sleep(200);
  }
  throw new Error(`bridge /health not ready within ${timeoutMs}ms`);
}

async function jsonReq(method, url, { body, auth } = {}) {
  const headers = { "content-type": "application/json" };
  if (auth) headers.authorization = `Bearer ${auth}`;
  const r = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await r.text();
  let data;
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = { raw: text };
  }
  return { status: r.status, data };
}

// ────────────────────────────────────────────────────────────────────────────
// Agent definition (OpenRouter + immortal)
// ────────────────────────────────────────────────────────────────────────────

const AGENT_DEFINITION = {
  id: AGENT_ID,
  name: "Immortal Coder (OpenRouter / elephant-alpha)",
  system_prompt: `You are a senior API architect engaged in a multi-turn design session with the \
user. Your job is to produce detailed, implementation-ready design documents in TEXT — you are \
NOT writing code files or running commands unless explicitly told to. You have a persistent journal \
via journal_write — use it exactly ONCE per turn, at the end, to record the single most important \
decision from that turn. Beyond journal_write, DO NOT call any tools (no bash, no read, no ls, no \
glob, no rg, no write, no edit). Keep every response focused on the architectural question asked.`,
  provider: {
    provider_type: "open_ai",
    model: MODEL,
    api_key: OPENROUTER_API_KEY,
    base_url: OPENROUTER_BASE,
  },
  tools: [],
  mcp_servers: [],
  skills: [],
  permissions: {
    // Let the agent run autonomously — no approval gating
    bash: "allow",
    write: "allow",
    edit: "allow",
    multiedit: "allow",
    apply_patch: "allow",
    read: "allow",
    glob: "allow",
    ls: "allow",
    rg: "allow",
    ast_grep: "allow",
    todowrite: "allow",
    todoread: "allow",
    journal_write: "allow",
    journal_read: "allow",
  },
  config: {
    max_tokens: 4096,
    max_turns: 40,
    temperature: 0.3,
    immortal: {
      token_budget: TOKEN_BUDGET,
      carry_forward_turns: CARRY_FORWARD_TURNS,
      // Cap the carry-forward tail at 30% of budget. Prevents one tool-heavy
      // turn from stuffing the fresh-chain context with its whole tool loop.
      carry_forward_budget_fraction: 0.3,
      // Single-phase checkpoint extraction. For strong summarizer models the
      // verification pass rarely improves output and doubles the cost.
      verify_checkpoint: false,
      // Tighter output cap (1200 toks) reinforces the "under 900 tokens" hard
      // rule in the prompt. 180s call timeout.
      checkpoint_max_tokens: 1200,
      checkpoint_timeout_secs: 180,
      // Only the last 2 chain checkpoints feed the next extraction — older
      // ones are considered subsumed by later snapshots.
      max_previous_checkpoints: 2,
      // Custom prompt tuned for Gemini 2.5 Flash (overrides bridge's default).
      checkpoint_prompt: GEMINI_CHECKPOINT_PROMPT,
      checkpoint_provider: {
        provider_type: "open_ai",
        model: CHECKPOINT_MODEL,
        api_key: OPENROUTER_API_KEY,
        base_url: OPENROUTER_BASE,
      },
    },
  },
};

// ────────────────────────────────────────────────────────────────────────────
// Tracking state
// ────────────────────────────────────────────────────────────────────────────

const state = {
  conversationId: null,
  turns: [], // per-turn { index, toolCalls, contentChars, reasoningChars, duration }
  currentTurn: null,
  toolCallsTotal: 0,
  chainEvents: [], // { chain_started, chain_completed, chain_failed, started_at, duration_ms }
  chainFailureCount: 0,
  pressureWarnings: [], // { cumulative_tool_output_bytes, threshold_bytes, turn_index }
  journalWrites: [],
  allEvents: [], // every event (also written to disk)
  lastAgentError: null, // set by the SSE dispatcher on each error event
};

const allEventsFile = createWriteStream(join(LOG_DIR, "events.jsonl"));

function startTurn(label) {
  state.currentTurn = {
    index: state.turns.length + 1,
    label,
    toolCalls: [],
    contentChars: 0,
    reasoningChars: 0,
    startedAt: Date.now(),
    endedAt: null,
  };
}
function endTurn() {
  if (!state.currentTurn) return;
  state.currentTurn.endedAt = Date.now();
  state.currentTurn.duration = state.currentTurn.endedAt - state.currentTurn.startedAt;
  state.turns.push(state.currentTurn);
  const t = state.currentTurn;
  info(
    `turn ${t.index} [${t.label}] complete: ${t.toolCalls.length} tool calls, ` +
      `${t.contentChars} content chars, ${t.reasoningChars} reasoning chars, ` +
      `${(t.duration / 1000).toFixed(1)}s`,
  );
  state.currentTurn = null;
}

// ────────────────────────────────────────────────────────────────────────────
// SSE parser
// ────────────────────────────────────────────────────────────────────────────

async function streamSse(convId, base, onEvent) {
  const r = await fetch(`${base}/conversations/${convId}/stream`, {
    headers: { accept: "text/event-stream" },
  });
  if (!r.ok) throw new Error(`SSE open failed: ${r.status}`);

  const reader = r.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) return;
    buf += dec.decode(value, { stream: true });
    let idx;
    // SSE frames separated by blank line
    while ((idx = buf.indexOf("\n\n")) !== -1) {
      const raw = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      const lines = raw.split("\n");
      let ev = null;
      let dataLines = [];
      for (const line of lines) {
        if (line.startsWith("event:")) ev = line.slice(6).trim();
        else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
      }
      if (!ev && dataLines.length === 0) continue;
      const dataStr = dataLines.join("\n");
      let payload;
      try {
        payload = dataStr ? JSON.parse(dataStr) : {};
      } catch {
        payload = { raw: dataStr };
      }
      allEventsFile.write(JSON.stringify({ ev, payload, t: Date.now() }) + "\n");
      state.allEvents.push({ ev, payload });
      await onEvent(ev, payload);
      // NOTE: "done" fires at the end of every turn, not only at conversation end.
      // The server keeps the SSE channel open across turns — so we keep reading.
    }
  }
}

// ────────────────────────────────────────────────────────────────────────────
// Event dispatcher — human-readable live commentary
// ────────────────────────────────────────────────────────────────────────────

function describeToolArgs(name, args) {
  if (!args || typeof args !== "object") return "";
  // Compact pretty-print for common tools
  const keys = {
    write: ["file_path"],
    read: ["file_path"],
    edit: ["file_path"],
    multiedit: ["file_path"],
    bash: ["command"],
    glob: ["pattern", "path"],
    rg: ["pattern"],
    ast_grep: ["pattern"],
    ls: ["path"],
    todowrite: ["todos"],
    journal_write: ["content", "category"],
  }[name];
  if (!keys) return JSON.stringify(args).slice(0, 120);
  const parts = [];
  for (const k of keys) {
    if (args[k] != null) {
      let v = typeof args[k] === "string" ? args[k] : JSON.stringify(args[k]);
      if (v.length > 80) v = v.slice(0, 80) + "…";
      parts.push(`${k}=${JSON.stringify(v)}`);
    }
  }
  return parts.join(" ");
}

async function dispatchEvent(ev, payload) {
  const inner = payload?.data ?? payload;

  switch (ev) {
    case "conversation_created":
      evt(ev, `conv_id=${payload.conversation_id ?? state.conversationId}`);
      break;

    case "message_received": {
      const t = (inner.content ?? "").slice(0, 80).replace(/\n/g, " ");
      evt(ev, `"${t}${inner.content?.length > 80 ? "…" : ""}"`);
      break;
    }

    case "message_start":
      // startTurn is owned by sendTurn — don't double-start here.
      evt(ev, "assistant started");
      break;

    case "content_delta": {
      const txt = inner.text ?? inner.content ?? inner.delta ?? "";
      if (state.currentTurn) state.currentTurn.contentChars += txt.length;
      // Only log occasionally to avoid spam
      break;
    }

    case "reasoning_delta": {
      const txt = inner.text ?? "";
      if (state.currentTurn) state.currentTurn.reasoningChars += txt.length;
      break;
    }

    case "tool_call_start": {
      const name = inner.name ?? inner.tool_name ?? "?";
      const args = inner.arguments ?? inner.args ?? inner.input ?? {};
      if (state.currentTurn) state.currentTurn.toolCalls.push(name);
      state.toolCallsTotal++;
      const summary = describeToolArgs(name, args);
      evt(ev, `${C.bold}${name}${C.reset} ${summary}`);
      if (name === "journal_write") {
        state.journalWrites.push({
          content: args.content,
          category: args.category,
          turn: state.currentTurn?.index,
        });
      }
      break;
    }

    case "tool_call_result": {
      const name = inner.tool_name ?? inner.name ?? "?";
      const result = inner.result ?? inner.output ?? "";
      const ok_ = inner.is_error === true ? "FAIL" : "ok";
      const preview =
        typeof result === "string"
          ? result.slice(0, 80).replace(/\n/g, " ")
          : JSON.stringify(result).slice(0, 80);
      evt(ev, `${name} → ${ok_} (${preview}${preview.length >= 80 ? "…" : ""})`);
      break;
    }

    case "todo_updated": {
      const todos = inner.todos ?? [];
      const open = todos.filter((t) => t.status !== "completed").length;
      evt(ev, `${todos.length} items (${open} open)`);
      break;
    }

    case "message_end":
      evt(ev, "assistant finished message");
      break;

    case "turn_completed": {
      const toks = {
        input: inner.input_tokens,
        output: inner.output_tokens,
        cum_input: inner.cumulative_input_tokens,
        cum_output: inner.cumulative_output_tokens,
        history_tokens: inner.history_tokens_estimate,
        history_messages: inner.history_message_count,
        cum_tool_calls: inner.cumulative_tool_calls,
        turn_latency_ms: inner.turn_latency_ms,
        journal_committed: inner.journal_entries_committed,
      };
      if (state.currentTurn) state.currentTurn.tokens = toks;
      const summary = toks.input != null
        ? `input=${toks.input} output=${toks.output} cum_input=${toks.cum_input} hist_tokens=${toks.history_tokens ?? "-"} hist_msgs=${toks.history_messages ?? "-"}${toks.journal_committed ? " journal_committed=" + toks.journal_committed : ""}`
        : "(no usage)";
      evt(ev, `turn done — ${summary}`);
      endTurn();
      break;
    }

    case "chain_started": {
      const { chain_index, token_count, reason, budget, verify_enabled } = inner;
      console.log();
      log(
        C.yellow + C.bold,
        "CHAIN_STARTED",
        `chain_index=${chain_index} reason=${reason} token_count=${token_count} (budget=${budget ?? TOKEN_BUDGET}) verify=${verify_enabled}`,
      );
      // This event now fires BEFORE the expensive checkpoint LLM call so we
      // can accurately measure handoff latency.
      state.chainEvents.push({ chain_started: inner, started_at: Date.now() });
      break;
    }

    case "chain_completed": {
      const {
        chain_index,
        journal_entry_count,
        carry_forward_messages,
        carry_forward_tokens,
        checkpoint_bytes,
        verified,
        duration_ms,
      } = inner;
      log(
        C.green + C.bold,
        "CHAIN_COMPLETED",
        `chain_index=${chain_index} journal_entries=${journal_entry_count} carry_fwd_msgs=${carry_forward_messages} carry_fwd_tokens=${carry_forward_tokens} ckpt_bytes=${checkpoint_bytes} verified=${verified} duration=${duration_ms}ms`,
      );
      const last = state.chainEvents[state.chainEvents.length - 1];
      if (last) {
        last.chain_completed = inner;
        last.duration_ms = duration_ms ?? (Date.now() - last.started_at);
      }
      console.log();
      break;
    }

    case "chain_failed": {
      const { chain_index, reason, token_count, duration_ms } = inner;
      log(
        C.red + C.bold,
        "CHAIN_FAILED",
        `chain_index=${chain_index} reason="${(reason || "").slice(0, 120)}" token_count=${token_count} duration=${duration_ms}ms`,
      );
      state.chainFailureCount++;
      const last = state.chainEvents[state.chainEvents.length - 1];
      if (last && !last.chain_completed) {
        last.chain_failed = inner;
        last.duration_ms = duration_ms ?? (Date.now() - last.started_at);
      }
      break;
    }

    case "context_pressure_warning": {
      const { cumulative_tool_output_bytes, threshold_bytes, reason } = inner;
      log(
        C.yellow,
        "PRESSURE",
        `tool-output bytes ${cumulative_tool_output_bytes}/${threshold_bytes} reason=${reason}`,
      );
      state.pressureWarnings.push({
        cumulative_tool_output_bytes,
        threshold_bytes,
        reason,
        turn_index: state.currentTurn?.index,
      });
      break;
    }

    case "error":
      state.lastAgentError = inner;
      err(`${ev}: ${JSON.stringify(inner).slice(0, 240)}`);
      break;

    case "done":
      evt(ev, "stream signalled done");
      break;

    default:
      evt(ev, JSON.stringify(inner).slice(0, 80));
  }
}

// ────────────────────────────────────────────────────────────────────────────
// Main run
// ────────────────────────────────────────────────────────────────────────────

async function main() {
  step("pre-flight");
  if (!existsSync(BRIDGE_BIN)) {
    err(`bridge binary not found at ${BRIDGE_BIN} — run 'cargo build -p bridge'`);
    process.exit(1);
  }
  info(`workspace: ${WORKSPACE}`);
  info(`logs:      ${LOG_DIR}`);
  info(`agent model:      ${MODEL}`);
  info(`checkpoint model: ${CHECKPOINT_MODEL}`);
  info(`budget:           ${TOKEN_BUDGET} tokens (carry_forward_turns=${CARRY_FORWARD_TURNS})`);

  const port = await findFreePort();
  const base = `http://127.0.0.1:${port}`;
  info(`bridge port: ${port}`);

  const bridgeStdoutFile = createWriteStream(join(LOG_DIR, "bridge.stdout.log"));
  const bridgeStderrFile = createWriteStream(join(LOG_DIR, "bridge.stderr.log"));

  step("starting bridge");
  let shuttingDown = false;
  const bridgeProc = spawn(BRIDGE_BIN, [], {
    cwd: WORKSPACE,
    env: {
      ...process.env,
      BRIDGE_LISTEN_ADDR: `127.0.0.1:${port}`,
      BRIDGE_CONTROL_PLANE_API_KEY: BRIDGE_API_KEY,
      BRIDGE_CONTROL_PLANE_URL: "", // not needed
      BRIDGE_LOG_LEVEL: "info",
      BRIDGE_LOG_FORMAT: "text",
      // Keep storage inside the logs dir so we can inspect chain_links table afterwards
      BRIDGE_STORAGE_PATH: join(LOG_DIR, "bridge.db"),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  bridgeProc.stdout.pipe(bridgeStdoutFile);
  bridgeProc.stderr.pipe(bridgeStderrFile);

  bridgeProc.on("exit", (code, sig) => {
    if (!shuttingDown) {
      err(`bridge exited unexpectedly (code=${code}, signal=${sig})`);
      process.exit(1);
    }
  });

  const shutdown = () => {
    if (shuttingDown) return;
    shuttingDown = true;
    try {
      bridgeProc.kill("SIGTERM");
    } catch {}
  };
  process.on("SIGINT", () => {
    shutdown();
    process.exit(130);
  });
  process.on("SIGTERM", () => {
    shutdown();
    process.exit(143);
  });

  try {
    step("waiting for /health");
    await waitForHealth(base, 30_000);
    ok("bridge healthy");

    step("pushing immortal-coder agent");
    writeFileSync(
      join(LOG_DIR, "agent.json"),
      JSON.stringify(AGENT_DEFINITION, null, 2),
    );
    const pushResp = await jsonReq("PUT", `${base}/push/agents/${AGENT_ID}`, {
      body: AGENT_DEFINITION,
      auth: BRIDGE_API_KEY,
    });
    if (pushResp.status >= 300) {
      err(`push failed: ${pushResp.status} ${JSON.stringify(pushResp.data)}`);
      throw new Error("push failed");
    }
    ok(`agent pushed (status=${pushResp.status}, body=${JSON.stringify(pushResp.data)})`);

    step("creating conversation");
    const createResp = await jsonReq(
      "POST",
      `${base}/agents/${AGENT_ID}/conversations`,
      { body: {} },
    );
    if (createResp.status >= 300) {
      err(`create conversation failed: ${createResp.status} ${JSON.stringify(createResp.data)}`);
      throw new Error("create failed");
    }
    state.conversationId = createResp.data.conversation_id;
    ok(`conversation: ${state.conversationId}`);

    // Open the SSE stream in a background task; messages we send will be
    // processed by bridge and emit events that flow to this stream.
    const streamTask = (async () => {
      try {
        await streamSse(state.conversationId, base, dispatchEvent);
      } catch (e) {
        warn(`SSE stream ended: ${e.message}`);
      }
    })();

    // Give the SSE connection a beat to attach before sending the first message.
    await sleep(500);

    // ────────────────────────────────────────────────────────────────────
    // Multi-turn coding flow — smaller chunks so upstream rate-limits
    // don't eat whole turns. Each sendTurn waits for turn_completed and
    // auto-retries on agent_error (upstream stealth model throttling).
    // ────────────────────────────────────────────────────────────────────

    // Design-heavy multi-turn flow. Each turn produces a long assistant
    // response (few or zero tool calls) so one LLM call grows history by
    // ~1–3k tokens. That keeps us well under elephant-alpha's tight upstream
    // rate limit while still filling the 10k-token immortal budget across
    // several successful turns. Chain should trigger around turn 5–7.
    const messages = [
      {
        label: "design: endpoints + request/response contracts",
        content: `Design a complete Node.js task-management REST API in TEXT (no code files). \
For every endpoint, list: method, path, auth requirement, request-body shape with field types, \
response-body shape with field types, and all status codes. Cover auth, users, tasks CRUD, \
health. Pick a URL versioning scheme, a content-type convention, and a pagination style, and \
justify each. Aim for ~700 words. At the end, call journal_write ONCE capturing the core API \
shape decision. Don't call any other tool.`,
      },
      {
        label: "design: database schema + indexes",
        content: `Now produce the full data model. For each table (users, tasks, refresh_tokens), \
spell out every column with SQL type, nullability, default, index. Include all foreign keys \
and cascade rules. Write the complete CREATE TABLE + CREATE INDEX statements as a fenced \
SQL code block. Write a short rationale paragraph after each table. ~700 words. At the end \
call journal_write ONCE capturing the schema decision. Don't call any other tool.`,
      },
      {
        label: "design: authentication + password handling",
        content: `Design the authentication subsystem in depth: password hashing library and \
cost parameters, JWT claim set and expiry, refresh-token rotation, logout/revocation, \
timing-attack defenses on login, and rate-limiting on /auth/login. Include a full signed-in \
request sequence diagram in ASCII. ~700 words. End with journal_write ONCE capturing the \
auth-library choice + parameters. Don't call any other tool.`,
      },
      {
        label: "design: error handling + validation contract",
        content: `Describe the error-handling contract in detail: the envelope shape for error \
responses, the mapping of error classes to HTTP status codes, how zod validation errors become \
400 responses, how to attach request IDs, how async errors propagate through Express 4, and \
where the global error handler sits relative to routers. ~650 words. End with journal_write \
ONCE capturing the error-envelope shape. Don't call any other tool.`,
      },
      {
        label: "design: testing + CI strategy",
        content: `Describe the testing + CI strategy: unit vs integration scope, SQLite DB \
seeding/teardown, JWT-secret handling in tests, what gets mocked vs real, how the suite runs \
against a clean DB each time, and the shape of CI pipeline jobs. ~600 words. End with \
journal_write ONCE capturing the test-DB strategy. Don't call any other tool.`,
      },
      {
        label: "design: deployment + observability",
        content: `Describe the deployment and observability plan: container image build, \
environment variables and secrets, zero-downtime rollout, health + readiness endpoints, \
structured logging format, metrics (what to export + why), and distributed tracing. ~600 words. \
End with journal_write ONCE capturing the deployment target. Don't call any other tool.`,
      },
      {
        label: "continuity: journal recall after probable chain",
        content: `No design this turn — a recall test. Using journal_read and whatever context \
you still have, answer briefly:
1. Which password hashing library and cost did we lock in?
2. What is the shape of the error-response envelope?
3. What is the test-DB strategy?
4. What deployment target did we pick?
5. List every journal entry you've written so far, in order.`,
      },
    ];

    for (let i = 0; i < messages.length; i++) {
      const m = messages[i];
      step(`sending Turn ${i + 1} — ${m.label}`);
      await sendTurn(base, state.conversationId, m.content, m.label);
      await printMetrics(base, `after Turn ${i + 1}`);
      // Long breather to let upstream rate limit reset.
      if (i < messages.length - 1) {
        info("sleeping 60s before next turn (rate-limit cooling)");
        await sleep(60_000);
      }
    }

    // ────────────────────────────────────────────────────────────────────
    // Final report
    // ────────────────────────────────────────────────────────────────────
    console.log();
    console.log(`${C.bold}═══════════════ FINAL REPORT ═══════════════${C.reset}`);
    console.log();

    console.log(`${C.bold}Chain handoffs:${C.reset} ${state.chainEvents.length} triggered, ${state.chainFailureCount} failed`);
    state.chainEvents.forEach((ce, i) => {
      const status = ce.chain_completed ? "ok" : (ce.chain_failed ? "FAILED" : "?");
      const detail = ce.chain_completed
        ? `journal=${ce.chain_completed.journal_entry_count} carry_msgs=${ce.chain_completed.carry_forward_messages} carry_toks=${ce.chain_completed.carry_forward_tokens} ckpt=${ce.chain_completed.checkpoint_bytes}B verified=${ce.chain_completed.verified}`
        : (ce.chain_failed ? `reason="${(ce.chain_failed.reason || "").slice(0, 80)}"` : "pending");
      console.log(
        `  [${i + 1}] [${status}] chain_index=${ce.chain_started.chain_index}, ` +
          `pre_chain_tokens=${ce.chain_started.token_count}, duration=${ce.duration_ms ?? "?"}ms, ${detail}`,
      );
    });

    if (state.pressureWarnings.length) {
      console.log();
      console.log(`${C.bold}Context pressure warnings:${C.reset} ${state.pressureWarnings.length}`);
      state.pressureWarnings.forEach((w, i) => {
        console.log(
          `  [${i + 1}] turn ${w.turn_index}: tool-output ${w.cumulative_tool_output_bytes}B >= ${w.threshold_bytes}B (${w.reason})`,
        );
      });
    }

    console.log();
    console.log(`${C.bold}Turns:${C.reset}`);
    state.turns.forEach((t) => {
      console.log(
        `  Turn ${t.index} [${t.label}]: ${t.toolCalls.length} tool calls, ` +
          `${t.contentChars} content chars, ${t.reasoningChars} reasoning chars, ` +
          `${(t.duration / 1000).toFixed(1)}s`,
      );
      const toolCounts = {};
      t.toolCalls.forEach((n) => (toolCounts[n] = (toolCounts[n] ?? 0) + 1));
      const sorted = Object.entries(toolCounts).sort((a, b) => b[1] - a[1]);
      if (sorted.length) {
        console.log(`      tools: ${sorted.map(([n, c]) => `${n}×${c}`).join(", ")}`);
      }
    });

    console.log();
    console.log(`${C.bold}Total tool calls:${C.reset} ${state.toolCallsTotal}`);
    console.log(`${C.bold}Journal writes by agent:${C.reset} ${state.journalWrites.length}`);
    state.journalWrites.forEach((j, i) => {
      const c = (j.content ?? "").slice(0, 100).replace(/\n/g, " ");
      console.log(`  [${i + 1}] (turn ${j.turn}) [${j.category ?? "none"}] ${c}${(j.content ?? "").length > 100 ? "…" : ""}`);
    });

    console.log();
    const finalMetrics = await jsonReq("GET", `${base}/metrics`);
    const agentM = finalMetrics.data?.agents?.find((a) => a.agent_id === AGENT_ID);
    if (agentM) {
      console.log(`${C.bold}Agent metrics:${C.reset}`);
      console.log(`  total_requests=${agentM.total_requests}`);
      console.log(`  failed_requests=${agentM.failed_requests}`);
      console.log(`  input_tokens=${agentM.input_tokens}`);
      console.log(`  output_tokens=${agentM.output_tokens}`);
      console.log(`  total_tokens=${agentM.total_tokens}`);
      console.log(`  tool_calls=${agentM.tool_calls}`);
      console.log(`  avg_latency_ms=${agentM.avg_latency_ms?.toFixed?.(1) ?? agentM.avg_latency_ms}`);
      if (Array.isArray(agentM.tool_call_details) && agentM.tool_call_details.length) {
        console.log(`  per-tool:`);
        for (const td of agentM.tool_call_details) {
          console.log(
            `    ${td.tool_name}: calls=${td.total_calls} ok=${td.successes} fail=${td.failures} avg=${td.avg_latency_ms?.toFixed?.(1) ?? td.avg_latency_ms}ms`,
          );
        }
      }
    }

    console.log();
    console.log(`${C.bold}Artifacts:${C.reset}`);
    console.log(`  workspace:     ${WORKSPACE}`);
    console.log(`  event log:     ${join(LOG_DIR, "events.jsonl")}`);
    console.log(`  bridge logs:   ${join(LOG_DIR, "bridge.stdout.log")} (& .stderr.log)`);
    console.log(`  bridge db:     ${join(LOG_DIR, "bridge.db")}`);
    console.log(`  agent def:     ${join(LOG_DIR, "agent.json")}`);

    // Keep the SSE promise from dangling forever
    streamTask.catch(() => {});
  } finally {
    step("shutting down bridge");
    shutdown();
    await sleep(300);
    try {
      bridgeProc.kill("SIGKILL");
    } catch {}
    allEventsFile.end();
  }
}

async function waitForTurnComplete(timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  const targetCount = state.turns.length + 1;
  while (Date.now() < deadline) {
    if (state.turns.length >= targetCount) return;
    await sleep(500);
  }
  warn(`timed out waiting for turn to complete after ${timeoutMs / 1000}s`);
}

/**
 * Send a user message, wait for turn_completed, auto-retry up to N times
 * if the turn ended with an upstream 429 (stealth-model rate-limited).
 * Uses exponential backoff.
 */
async function sendTurn(base, convId, content, label, { maxRetries = 4 } = {}) {
  let attempt = 0;
  while (true) {
    attempt++;
    state.lastAgentError = null;
    startTurn(`${label}${attempt > 1 ? ` (retry ${attempt - 1})` : ""}`);

    // On 429 retry, re-send the ORIGINAL message. The turn's prior mutations
    // were rolled back by bridge, so from the agent's POV nothing happened.
    const body = { content };

    const resp = await jsonReq("POST", `${base}/conversations/${convId}/messages`, { body });
    if (resp.status >= 300) {
      err(`send failed: ${resp.status} ${JSON.stringify(resp.data)}`);
      endTurn();
      throw new Error(`send failed (status=${resp.status})`);
    }

    await waitForTurnComplete(15 * 60_000);

    const e = state.lastAgentError;
    if (!e) return; // clean completion

    const msg = e.message ?? "";
    const is429 = /429|Too Many Requests|rate.?limited/i.test(msg);
    if (!is429 || attempt > maxRetries) {
      warn(`turn ended with non-retryable error (or too many retries): ${msg.slice(0, 200)}`);
      return;
    }

    const backoff = 30_000 * Math.pow(1.5, attempt - 1);
    warn(`upstream 429 — backing off ${Math.round(backoff / 1000)}s before retry ${attempt}`);
    await sleep(backoff);
  }
}

async function printMetrics(base, label) {
  try {
    const m = await jsonReq("GET", `${base}/metrics`);
    const a = m.data?.agents?.find((x) => x.agent_id === AGENT_ID);
    if (a) {
      log(
        C.blue,
        "METRICS",
        `${label}: requests=${a.total_requests} input=${a.input_tokens} output=${a.output_tokens} total=${a.total_tokens} tool_calls=${a.tool_calls}`,
      );
    }
  } catch (e) {
    warn(`metrics fetch failed: ${e.message}`);
  }
}

main().catch((e) => {
  err(e.stack ?? e.message);
  process.exit(1);
});
