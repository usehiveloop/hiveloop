import type { components } from "@/lib/api/schema"

type RawEvent = components["schemas"]["conversationEventResponse"]

export interface ConversationEvent {
  id: string
  event_id: string
  event_type: string
  timestamp: string
  sequence_number: number
  data: Record<string, unknown>
}

interface MessageReceivedData {
  content: string
}

interface ResponseStartedData {
  message_id: string
  conversation_id: string
}

interface ResponseCompletedData {
  model: string
  message_id: string
  full_response: string
  input_tokens: number
  output_tokens: number
  timestamp: string
}

interface ToolCallStartedData {
  id: string
  name: string
  arguments: Record<string, unknown>
}

interface ToolCallCompletedData {
  id: string
  tool_name: string
  result: string
  is_error: boolean
  duration_ms: number
}

interface TurnCompletedData {
  turn_number: number
  model: string
  input_tokens: number
  output_tokens: number
  cumulative_input_tokens: number
  cumulative_output_tokens: number
}

export type TimelineItem =
  | { kind: "conversation_created"; id: string; timestamp: string }
  | { kind: "message_received"; id: string; timestamp: string; content: string }
  | {
      kind: "agent_response"
      id: string
      timestamp: string
      content: string
      model?: string
      inputTokens?: number
      outputTokens?: number
      status: "streaming" | "completed"
    }
  | {
      kind: "tool_call"
      id: string
      timestamp: string
      toolName: string
      args: Record<string, unknown>
      status: "running" | "success" | "failed"
      result?: string
      durationMs?: number
      /** Inline preview shown next to the tool name; truncated via CSS. */
      subtitle?: string
      /** Full version of the subtitle, shown in a tooltip on hover. */
      subtitleFull?: string
      /** Small badges shown in the header (e.g. "79 lines", "truncated"). */
      meta?: string[]
    }
  | {
      kind: "turn_summary"
      id: string
      timestamp: string
      turnNumber: number
      model: string
      inputTokens: number
      outputTokens: number
      cumulativeInputTokens: number
      cumulativeOutputTokens: number
    }
  | { kind: "done"; id: string; timestamp: string }

interface BashResult {
  output: string
  exit_code: number
  timed_out: boolean
}

interface ReadResult {
  content: string
  total_lines: number
  lines_read: number
  truncated: boolean
}

/**
 * Unwrap a double-JSON-encoded payload — bridge serialises structured tool
 * results as `JSON.stringify(JSON.stringify(obj))`, so we parse up to two
 * layers until we land on an object. Returns null if the shape isn't object.
 */
function parseDoubleEncodedObject(raw: string | undefined): Record<string, unknown> | null {
  if (!raw) return null
  try {
    let value: unknown = raw
    for (let depth = 0; depth < 3 && typeof value === "string"; depth++) {
      value = JSON.parse(value)
    }
    if (value && typeof value === "object") return value as Record<string, unknown>
  } catch {
    return null
  }
  return null
}

function parseBashResult(raw: string | undefined): BashResult | null {
  const obj = parseDoubleEncodedObject(raw)
  if (!obj || !("output" in obj)) return null
  return {
    output: typeof obj.output === "string" ? obj.output : String(obj.output ?? ""),
    exit_code: typeof obj.exit_code === "number" ? obj.exit_code : 0,
    timed_out: !!obj.timed_out,
  }
}

function parseReadResult(raw: string | undefined): ReadResult | null {
  const obj = parseDoubleEncodedObject(raw)
  if (!obj || !("content" in obj)) return null
  return {
    content: typeof obj.content === "string" ? obj.content : String(obj.content ?? ""),
    total_lines: typeof obj.total_lines === "number" ? obj.total_lines : 0,
    lines_read: typeof obj.lines_read === "number" ? obj.lines_read : 0,
    truncated: !!obj.truncated,
  }
}

/**
 * Read content arrives with "N: " line-number prefixes. Return the first
 * non-empty line with the prefix stripped, suitable for an inline preview.
 */
function firstContentLine(content: string): string {
  for (const rawLine of content.split("\n")) {
    const stripped = rawLine.replace(/^\s*\d+:\s?/, "").trim()
    if (stripped.length > 0) return stripped
  }
  return ""
}

function isBashTool(name: string | undefined): boolean {
  return name === "bash" || name === "Bash"
}

/**
 * Produce a human-friendly display name for a tool call, using arguments
 * to enrich the label where useful. Falls back to the raw tool name.
 */
function formatToolName(name: string, args: Record<string, unknown>): string {
  if (name === "Read") {
    const filePath = typeof args.file_path === "string" ? args.file_path : ""
    const basename = extractBasename(filePath)
    if (basename) return `Reading file: ${basename}`
  }
  return name
}

function extractBasename(path: string): string {
  if (!path) return ""
  const trimmed = path.replace(/\/+$/, "")
  const idx = trimmed.lastIndexOf("/")
  return idx >= 0 ? trimmed.slice(idx + 1) : trimmed
}

/**
 * Convert API events to typed events with narrowed `data` field.
 * The generated schema types `data` as `number[]` because Go's
 * json.RawMessage round-trips poorly through swagger, so we cast here.
 */
export function normalizeEvents(rawEvents: RawEvent[]): ConversationEvent[] {
  return rawEvents
    .map((event) => {
      if (!event.id || !event.event_type || event.sequence_number == null) return null
      const data = (event.data ?? {}) as unknown as Record<string, unknown>
      return {
        id: event.id,
        event_id: event.event_id ?? "",
        event_type: event.event_type,
        timestamp: event.timestamp ?? event.created_at ?? "",
        sequence_number: event.sequence_number,
        data,
      }
    })
    .filter((event): event is ConversationEvent => event !== null)
    .sort((a, b) => a.sequence_number - b.sequence_number)
}

/**
 * Collapse raw events into a chronological list of display items.
 *
 * Merge rules:
 * - `tool_call_started` + `tool_call_completed` (same `data.id`) → one card that
 *   begins "running" and resolves to success/failed. An orphan started becomes
 *   a running card; an orphan completed becomes a success/failed card.
 * - `response_started` is dropped if a matching `response_completed` exists
 *   (same `message_id`); otherwise it becomes a "streaming" agent bubble.
 * - `turn_completed` is surfaced as a small footer between turns.
 * - `conversation_created` / `done` become system markers.
 */
export function buildTimeline(events: ConversationEvent[]): TimelineItem[] {
  const startedTools = new Map<string, ConversationEvent>()
  const completedResponseMessageIds = new Set<string>()

  for (const event of events) {
    if (event.event_type === "tool_call_started") {
      const data = event.data as unknown as ToolCallStartedData
      if (data.id) startedTools.set(data.id, event)
    }
    if (event.event_type === "response_completed") {
      const data = event.data as unknown as ResponseCompletedData
      if (data.message_id) completedResponseMessageIds.add(data.message_id)
    }
  }

  const items: TimelineItem[] = []
  const consumedToolStartIds = new Set<string>()

  for (const event of events) {
    switch (event.event_type) {
      case "conversation_created": {
        items.push({
          kind: "conversation_created",
          id: event.id,
          timestamp: event.timestamp,
        })
        break
      }
      case "message_received": {
        const data = event.data as unknown as MessageReceivedData
        items.push({
          kind: "message_received",
          id: event.id,
          timestamp: event.timestamp,
          content: data.content ?? "",
        })
        break
      }
      case "response_started": {
        const data = event.data as unknown as ResponseStartedData
        if (completedResponseMessageIds.has(data.message_id)) break
        items.push({
          kind: "agent_response",
          id: event.id,
          timestamp: event.timestamp,
          content: "",
          status: "streaming",
        })
        break
      }
      case "response_completed": {
        const data = event.data as unknown as ResponseCompletedData
        items.push({
          kind: "agent_response",
          id: event.id,
          timestamp: event.timestamp,
          content: data.full_response ?? "",
          model: data.model,
          inputTokens: data.input_tokens,
          outputTokens: data.output_tokens,
          status: "completed",
        })
        break
      }
      case "tool_call_started": {
        const data = event.data as unknown as ToolCallStartedData
        const completion = findCompletion(events, data.id)
        if (completion) {
          items.push(toolCallItem(event, completion))
          consumedToolStartIds.add(data.id)
        } else {
          const args = data.arguments ?? {}
          items.push({
            kind: "tool_call",
            id: event.id,
            timestamp: event.timestamp,
            toolName: formatToolName(data.name, args),
            args,
            status: "running",
          })
        }
        break
      }
      case "tool_call_completed": {
        const data = event.data as unknown as ToolCallCompletedData
        if (consumedToolStartIds.has(data.id)) break
        items.push(orphanCompletedItem(event))
        break
      }
      case "turn_completed": {
        const data = event.data as unknown as TurnCompletedData
        items.push({
          kind: "turn_summary",
          id: event.id,
          timestamp: event.timestamp,
          turnNumber: data.turn_number,
          model: data.model,
          inputTokens: data.input_tokens,
          outputTokens: data.output_tokens,
          cumulativeInputTokens: data.cumulative_input_tokens,
          cumulativeOutputTokens: data.cumulative_output_tokens,
        })
        break
      }
      case "done": {
        items.push({ kind: "done", id: event.id, timestamp: event.timestamp })
        break
      }
      default:
        break
    }
  }

  return items
}

function findCompletion(
  events: ConversationEvent[],
  toolCallId: string,
): ConversationEvent | undefined {
  return events.find(
    (event) =>
      event.event_type === "tool_call_completed" &&
      (event.data as unknown as ToolCallCompletedData).id === toolCallId,
  )
}

function toolCallItem(
  started: ConversationEvent,
  completed: ConversationEvent,
): TimelineItem {
  const startData = started.data as unknown as ToolCallStartedData
  const completeData = completed.data as unknown as ToolCallCompletedData
  const rawName = startData.name ?? completeData.tool_name

  if (isBashTool(rawName)) {
    return bashToolCallItem(started, completed)
  }

  const args = startData.arguments ?? {}
  const base = {
    kind: "tool_call" as const,
    id: started.id,
    timestamp: started.timestamp,
    toolName: formatToolName(rawName, args),
    args,
    status: (completeData.is_error ? "failed" : "success") as "success" | "failed",
    result: completeData.result,
    durationMs: completeData.duration_ms,
  }

  if (rawName === "Read" && !completeData.is_error) {
    const parsed = parseReadResult(completeData.result)
    if (parsed) {
      const preview = firstContentLine(parsed.content)
      const meta: string[] = []
      if (parsed.total_lines > 0) meta.push(`${parsed.total_lines} lines`)
      if (parsed.truncated) meta.push("truncated")
      return {
        ...base,
        result: parsed.content,
        subtitle: preview,
        subtitleFull: preview,
        meta,
      }
    }
  }

  return base
}

/**
 * Bash tool calls share the generic tool-call design but with parsed content:
 * the double-JSON-encoded result is flattened to a human-readable output, and
 * the status comes from `exit_code` (more reliable than `is_error` for bash).
 */
function bashToolCallItem(
  started: ConversationEvent,
  completed: ConversationEvent,
): TimelineItem {
  const startData = started.data as unknown as ToolCallStartedData
  const completeData = completed.data as unknown as ToolCallCompletedData
  const parsed = parseBashResult(completeData.result)

  const result = parsed ? formatBashResult(parsed) : completeData.result
  const status: "success" | "failed" =
    parsed?.timed_out || (parsed && parsed.exit_code !== 0) || completeData.is_error
      ? "failed"
      : "success"

  return {
    kind: "tool_call",
    id: started.id,
    timestamp: started.timestamp,
    toolName: "bash",
    args: startData.arguments ?? {},
    status,
    result,
    durationMs: completeData.duration_ms,
  }
}

function orphanCompletedItem(event: ConversationEvent): TimelineItem {
  const data = event.data as unknown as ToolCallCompletedData
  if (isBashTool(data.tool_name)) {
    const parsed = parseBashResult(data.result)
    const result = parsed ? formatBashResult(parsed) : data.result
    const status: "success" | "failed" =
      parsed?.timed_out || (parsed && parsed.exit_code !== 0) || data.is_error ? "failed" : "success"
    return {
      kind: "tool_call",
      id: event.id,
      timestamp: event.timestamp,
      toolName: "bash",
      args: {},
      status,
      result,
      durationMs: data.duration_ms,
    }
  }
  return {
    kind: "tool_call",
    id: event.id,
    timestamp: event.timestamp,
    toolName: data.tool_name,
    args: {},
    status: data.is_error ? "failed" : "success",
    result: data.result,
    durationMs: data.duration_ms,
  }
}

function formatBashResult(parsed: BashResult): string {
  const lines: string[] = []
  if (parsed.timed_out) lines.push("[timed out]")
  if (parsed.exit_code !== 0) lines.push(`[exit ${parsed.exit_code}]`)
  const output = parsed.output.length > 0 ? parsed.output : "(no output)"
  if (lines.length > 0) return lines.join(" ") + "\n\n" + output
  return output
}

export function formatTokens(count: number): string {
  if (count >= 1000) return `${(count / 1000).toFixed(1)}k`
  return String(count)
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function formatTimestamp(iso: string): string {
  if (!iso) return ""
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}
