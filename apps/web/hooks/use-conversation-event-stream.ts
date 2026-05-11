"use client"

import { useEffect, useRef } from "react"
import { useQuery } from "@tanstack/react-query"
import { fetchEventSource } from "@microsoft/fetch-event-source"

const API_URL = process.env.NEXT_PUBLIC_API_URL as string

interface StreamToken {
  access_token: string
  org_id: string | null
  expires_at: number
}

async function fetchStreamToken(): Promise<StreamToken> {
  const res = await fetch("/api/auth/stream-token")
  if (!res.ok) {
    throw new Error(
      res.status === 401 ? "Not authenticated" : "Failed to fetch stream token"
    )
  }
  return res.json()
}

interface RawStreamEvent {
  event_type?: string
  event_id?: string
  sequence_number?: number
  timestamp?: string
  data?: unknown
}

// Lightweight SSE subscriber for a conversation. Connects on mount, parses
// each event, and invokes onEvent (defaults to console.log). No state, no
// reconciliation — that comes later. This is the observation pass.
export function useConversationEventStream(
  conversationId: string | null,
  onEvent?: (event: RawStreamEvent) => void
) {
  const onEventRef = useRef(onEvent)
  useEffect(() => {
    onEventRef.current = onEvent
  }, [onEvent])

  const { data: token } = useQuery<StreamToken>({
    queryKey: ["stream-token"],
    queryFn: fetchStreamToken,
    enabled: conversationId !== null,
    staleTime: 3 * 60 * 1000,
    refetchInterval: 4 * 60 * 1000,
    retry: 1,
  })

  useEffect(() => {
    if (!conversationId || !token) return

    const ctrl = new AbortController()
    const url = `${API_URL}/v1/conversations/${conversationId}/stream`

    fetchEventSource(url, {
      signal: ctrl.signal,
      headers: {
        Authorization: `Bearer ${token.access_token}`,
        ...(token.org_id ? { "X-Org-ID": token.org_id } : {}),
      },
      onopen: async (response) => {
        if (response.ok) {
          return
        }
        console.error(
          `[stream] open failed conv=${conversationId} status=${response.status}`
        )
        throw new Error(`stream open failed: ${response.status}`)
      },
      onmessage: (msg) => {
        if (!msg.data) return
        let parsed: RawStreamEvent
        try {
          parsed = JSON.parse(msg.data)
        } catch {
          console.warn("[stream] bad json", msg.data)
          return
        }
        onEventRef.current?.(parsed)
      },
      onerror: (err) => {
        if (ctrl.signal.aborted) return
        console.warn("[stream] error", err)
        // Throwing aborts the auto-retry; let it retry by returning.
      },
      onclose: () => {},
    }).catch(() => {
      // fetchEventSource swallows non-thrown errors; explicit catch to silence
      // the unhandled promise warning if onopen throws.
    })

    return () => {
      ctrl.abort()
    }
  }, [conversationId, token])
}
