"use client"

import { useCallback, useRef, useState } from "react"
import { fetchEventSource } from "@microsoft/fetch-event-source"

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
  onDelta?: (delta: string) => void
}

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
          openWhenHidden: true,
          async onopen(response) {
            const ct = response.headers.get("content-type") ?? ""
            if (response.ok && ct.includes("text/event-stream")) return

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
            throw err
          },
          onclose() {
            if (!resolved) {
              resolved = true
              resolve({ text: buffer, usage: finalUsage })
            }
          },
        })
          .catch((err) => {
            if (resolved) return
            if (controller.signal.aborted) {
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
