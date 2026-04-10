"use client"

import { useRef, useState, useEffect, useCallback } from "react"
import { useQuery } from "@tanstack/react-query"
import { fetchEventSource } from "@microsoft/fetch-event-source"

interface StreamToken {
  access_token: string
  org_id: string | null
  expires_at: number
}

async function fetchStreamToken(): Promise<StreamToken> {
  const res = await fetch("/api/auth/stream-token")
  if (!res.ok) {
    throw new Error(res.status === 401 ? "Not authenticated" : "Failed to fetch stream token")
  }
  return res.json()
}

export interface BuildLog {
  line: string
}

export interface BuildStatus {
  status: "building" | "ready" | "failed"
  message: string
}

export interface BuildStreamEvent {
  id: string
  eventType: "log" | "status"
  data: BuildLog | BuildStatus
}

const API_URL = process.env.NEXT_PUBLIC_API_URL as string

export function useBuildStream(templateId: string | null) {
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [logs, setLogs] = useState<BuildLog[]>([])
  const [status, setStatus] = useState<BuildStatus | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const lastEventIdRef = useRef<string | null>(null)

  const {
    data: token,
    isLoading: tokenLoading,
    error: tokenError,
  } = useQuery<StreamToken>({
    queryKey: ["stream-token"],
    queryFn: fetchStreamToken,
    enabled: templateId !== null,
    staleTime: 3 * 60 * 1000,
    refetchInterval: 4 * 60 * 1000,
    retry: 1,
  })

  const disconnect = useCallback(() => {
    abortRef.current?.abort()
    abortRef.current = null
    setConnected(false)
  }, [])

  useEffect(() => {
    if (!templateId || !token) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      disconnect()
      return
    }

    setError(null)
    setLogs([])
    setStatus(null)
    lastEventIdRef.current = null

    const ctrl = new AbortController()
    abortRef.current = ctrl

    const url = `${API_URL}/v1/sandbox-templates/${templateId}/build-stream`

    const headers: Record<string, string> = {
      "Authorization": `Bearer ${token.access_token}`,
    }
    if (token.org_id) {
      headers["X-Org-ID"] = token.org_id
    }
    if (lastEventIdRef.current) {
      headers["Last-Event-ID"] = lastEventIdRef.current
    }

    console.log("[useBuildStream] Connecting to:", url, { templateId, headers: { ...headers, Authorization: "[REDACTED]" } })

    fetchEventSource(url, {
      signal: ctrl.signal,
      headers,
      onopen: async (response) => {
        console.log("[useBuildStream] onopen:", { ok: response.ok, status: response.status, statusText: response.statusText })
        if (response.ok) {
          setConnected(true)
          console.log("[useBuildStream] Stream connected")
        } else {
          const errorMsg = `Stream failed: ${response.status}`
          console.error("[useBuildStream] onopen error:", errorMsg)
          setError(errorMsg)
          throw new Error(`Stream open failed: ${response.status}`)
        }
      },
      onmessage: (event) => {
        console.log("[useBuildStream] onmessage:", { event: event.event, id: event.id, data: event.data })
        if (!event.data) return

        lastEventIdRef.current = event.id

        if (event.event === "log") {
          try {
            const data = JSON.parse(event.data) as BuildLog
            console.log("[useBuildStream] log event:", data)
            setLogs((prev) => [...prev, data])
          } catch {
            console.error("[useBuildStream] Failed to parse log event:", event.data)
            return
          }
        } else if (event.event === "status") {
          try {
            const data = JSON.parse(event.data) as BuildStatus
            console.log("[useBuildStream] status event:", data)
            setStatus(data)
            if (data.status === "ready" || data.status === "failed") {
              setConnected(false)
              ctrl.abort()
            }
          } catch {
            console.error("[useBuildStream] Failed to parse status event:", event.data)
            return
          }
        }
      },
      onerror: (err) => {
        console.error("[useBuildStream] onerror:", err)
        if (!ctrl.signal.aborted) {
          setError(err instanceof Error ? err.message : "Stream connection lost")
        }
        throw err
      },
      onclose: () => {
        console.log("[useBuildStream] Stream closed")
        setConnected(false)
      },
    }).catch(() => {
      // fetchEventSource throws when we throw in onerror — that's intentional
    })

    return () => {
      ctrl.abort()
      setConnected(false)
    }
  }, [templateId, token, disconnect])

  useEffect(() => {
    if (tokenError) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setError(tokenError instanceof Error ? tokenError.message : "Failed to fetch stream token")
    }
  }, [tokenError])

  return {
    connected,
    connecting: tokenLoading && templateId !== null,
    error,
    logs,
    status,
  }
}
