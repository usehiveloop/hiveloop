"use client"

import { useCallback, useRef, useState } from "react"
import { fetchEventSource } from "@microsoft/fetch-event-source"

// SSE frame shape emitted by /v1/system/tasks/{taskName} (the platform's
// rewritten envelope — not OpenAI's). Forwarder writes one
// `data: {"delta":"..."}` per chunk, then a final
// `data: {"done":true,"usage":{...}}`. No `[DONE]` sentinel.
type SystemTaskFrame = {
  delta?: string
  done?: boolean
  usage?: SystemTaskUsage
}

export interface SystemTaskUsage {
  input_tokens: number
  output_tokens: number
}

export interface SystemTaskError {
  error: string
  error_code?: string
  status: number
}

export interface SystemTaskResult {
  text: string
  usage?: SystemTaskUsage
}

interface RunOptions {
  /** Called on every delta. Receives the new chunk only, not the buffer. */
  onDelta?: (delta: string) => void
}

/**
 * Streams a registered platform system task. Generic over task name + args
 * because every system task uses the same wire shape.
 *
 * Behavior:
 *   - `run(args)` POSTs `{ stream: true, args }` to
 *     `/api/proxy/v1/system/tasks/{taskName}` and consumes the SSE response.
 *   - Resolves to `{ text, usage }` once the `done:true` frame arrives.
 *   - Rejects with a `SystemTaskError` on non-2xx (parses the typed
 *     `{error, error_code}` envelope when present).
 *   - `output` mirrors the accumulated text reactively, so a UI can render
 *     the prompt as it streams in.
 *   - Calling `run` again aborts any in-flight request.
 */
export function useSystemTaskStream(taskName: string) {
  const [isStreaming, setIsStreaming] = useState(false)
  const [output, setOutput] = useState("")
  const [usage, setUsage] = useState<SystemTaskUsage | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const abort = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
  }, [])

  const run = useCallback(
    async (args: Record<string, unknown>, options: RunOptions = {}): Promise<SystemTaskResult> => {
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller

      let buffer = ""
      let finalUsage: SystemTaskUsage | undefined
      let resolved = false

      setOutput("")
      setUsage(null)
      setIsStreaming(true)

      return new Promise<SystemTaskResult>((resolve, reject) => {
        fetchEventSource(`/api/proxy/v1/system/tasks/${encodeURIComponent(taskName)}`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Accept: "text/event-stream",
          },
          body: JSON.stringify({ stream: true, args }),
          credentials: "include",
          signal: controller.signal,
          // Default would re-open on visibility / network blips, but we
          // want a one-shot generation, not a long-lived subscription.
          openWhenHidden: true,
          async onopen(response) {
            const ct = response.headers.get("content-type") ?? ""
            if (response.ok && ct.includes("text/event-stream")) return

            // Surface the typed error envelope when present.
            let envelope: { error?: string; error_code?: string } = {}
            try {
              envelope = await response.json()
            } catch {
              /* non-JSON body */
            }
            const err: SystemTaskError = {
              error: envelope.error ?? `system task failed (HTTP ${response.status})`,
              error_code: envelope.error_code,
              status: response.status,
            }
            // Throwing here aborts the lib's reconnect loop. We surface the
            // typed object via the catch in onerror → reject.
            throw err
          },
          onmessage(ev) {
            if (!ev.data) return
            let frame: SystemTaskFrame
            try {
              frame = JSON.parse(ev.data) as SystemTaskFrame
            } catch {
              return
            }
            if (frame.delta) {
              buffer += frame.delta
              setOutput(buffer)
              options.onDelta?.(frame.delta)
            }
            if (frame.done) {
              finalUsage = frame.usage
              if (finalUsage) setUsage(finalUsage)
              resolved = true
              resolve({ text: buffer, usage: finalUsage })
              controller.abort()
            }
          },
          onerror(err) {
            // Throw to stop the lib's auto-reconnect.
            throw err
          },
          onclose() {
            // Some servers/proxies close without a `done` frame. Treat that
            // as success if we accumulated any output, otherwise as an
            // empty-but-clean stream.
            if (!resolved) {
              resolved = true
              resolve({ text: buffer, usage: finalUsage })
            }
          },
        })
          .catch((err) => {
            if (resolved) return
            if (controller.signal.aborted) {
              // Aborted by us (typically because we got `done` and called
              // controller.abort to short-circuit). The fetchEventSource
              // implementation throws its abort signal; treat as resolved.
              resolved = true
              resolve({ text: buffer, usage: finalUsage })
              return
            }
            resolved = true
            reject(err)
          })
          .finally(() => {
            setIsStreaming(false)
            if (abortRef.current === controller) abortRef.current = null
          })
      })
    },
    [taskName],
  )

  return { run, abort, isStreaming, output, usage }
}

export function isSystemTaskError(err: unknown): err is SystemTaskError {
  return typeof err === "object" && err !== null && "status" in err && "error" in err
}
