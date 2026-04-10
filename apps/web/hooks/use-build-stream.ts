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

    fetchEventSource(url, {
      signal: ctrl.signal,
      headers,
      onopen: async (response) => {
        if (response.ok) {
          setConnected(true)
        } else {
          setError(`Stream failed: ${response.status}`)
          throw new Error(`Stream open failed: ${response.status}`)
        }
      },
      onmessage: (event) => {
        if (!event.data) return

        lastEventIdRef.current = event.id

        if (event.event === "log") {
          try {
            const data = JSON.parse(event.data) as BuildLog
            setLogs((prev) => [...prev, data])
          } catch {
            return
          }
        } else if (event.event === "status") {
          try {
            const data = JSON.parse(event.data) as BuildStatus
            setStatus(data)
            if (data.status === "ready" || data.status === "failed") {
              setConnected(false)
              ctrl.abort()
            }
          } catch {
            return
          }
        }
      },
      onerror: (err) => {
        if (!ctrl.signal.aborted) {
          setError(err instanceof Error ? err.message : "Stream connection lost")
        }
        throw err
      },
      onclose: () => {
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
