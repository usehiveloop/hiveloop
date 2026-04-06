"use client"

import { useRef, useState, useEffect, useCallback } from "react"
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
    throw new Error(res.status === 401 ? "Not authenticated" : "Failed to fetch stream token")
  }
  return res.json()
}

/**
 * Hook for streaming conversation events via SSE directly from the backend.
 *
 * Pass a `conversationId` to enable — the hook fetches a valid access token
 * and connects to the SSE stream automatically. Pass `null` to disconnect.
 */
export function useConversationStream(conversationId: string | null) {
  const [connected, setConnected] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)

  const {
    data: token,
    isLoading: tokenLoading,
    error: tokenError,
  } = useQuery<StreamToken>({
    queryKey: ["stream-token"],
    queryFn: fetchStreamToken,
    enabled: conversationId !== null,
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
    if (!conversationId || !token) {
      disconnect()
      return
    }

    setError(null)

    const ctrl = new AbortController()
    abortRef.current = ctrl

    const url = `${API_URL}/v1/conversations/${conversationId}/stream`

    fetchEventSource(url, {
      signal: ctrl.signal,
      headers: {
        "Authorization": `Bearer ${token.access_token}`,
        ...(token.org_id ? { "X-Org-ID": token.org_id } : {}),
      },
      onopen: async (response) => {
        if (response.ok) {
          console.log("[stream] connected", conversationId)
          setConnected(true)
        } else {
          console.error("[stream] open failed", response.status, response.statusText)
          setError(`Stream failed: ${response.status}`)
          throw new Error(`Stream open failed: ${response.status}`)
        }
      },
      onmessage: (event) => {
        console.log("[stream] event", event.event, event.data)
      },
      onerror: (err) => {
        console.error("[stream] error", err)
        if (!ctrl.signal.aborted) {
          setError(err instanceof Error ? err.message : "Stream connection lost")
        }
        // Returning throws to stop retry
        throw err
      },
      onclose: () => {
        console.log("[stream] closed", conversationId)
        setConnected(false)
      },
    }).catch(() => {
      // fetchEventSource throws when we throw in onerror — that's intentional
    })

    return () => {
      ctrl.abort()
      setConnected(false)
    }
  }, [conversationId, token, disconnect])

  useEffect(() => {
    if (tokenError) {
      setError(tokenError instanceof Error ? tokenError.message : "Failed to fetch stream token")
    }
  }, [tokenError])

  return {
    connected,
    connecting: tokenLoading && conversationId !== null,
    error,
  }
}
