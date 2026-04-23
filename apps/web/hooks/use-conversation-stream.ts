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

export interface StreamMessage {
  id: string
  role: "user" | "agent" | "error"
  content: string
  messageId?: string
  timestamp: string
  model?: string
  inputTokens?: number
  outputTokens?: number
}

export interface TokenStats {
  inputTokens: number
  outputTokens: number
  model?: string
  turnNumber?: number
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
  const [messages, setMessages] = useState<StreamMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [tokenStats, setTokenStats] = useState<TokenStats>({ inputTokens: 0, outputTokens: 0 })
  const abortRef = useRef<AbortController | null>(null)
  const streamingMessageIdRef = useRef<string | null>(null)

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

  const addUserMessage = useCallback((content: string) => {
    const message: StreamMessage = {
      id: `user-${Date.now()}`,
      role: "user",
      content,
      timestamp: new Date().toISOString(),
    }
    setMessages((prev) => [...prev, message])
  }, [])

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
    setMessages([])
    setTokenStats({ inputTokens: 0, outputTokens: 0 })
    setIsStreaming(false)
    streamingMessageIdRef.current = null

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
          setConnected(true)
        } else {
          setError(`Stream failed: ${response.status}`)
          throw new Error(`Stream open failed: ${response.status}`)
        }
      },
      onmessage: (event) => {
        if (!event.data) return

        let parsed: {
          event_type: string
          data: Record<string, unknown>
          timestamp: string
          event_id: string
        }
        try {
          parsed = JSON.parse(event.data)
        } catch {
          return
        }

        const { event_type, data, timestamp, event_id } = parsed

        switch (event_type) {
          case "message_received": {
            const content = data.content as string
            if (content) {
              setMessages((prev) => [
                ...prev,
                {
                  id: event_id,
                  role: "user",
                  content,
                  timestamp,
                },
              ])
            }
            break
          }

          case "response_started": {
            const messageId = data.message_id as string
            streamingMessageIdRef.current = messageId
            setIsStreaming(true)
            setMessages((prev) => [
              ...prev,
              {
                id: event_id,
                role: "agent",
                content: "",
                messageId,
                timestamp,
              },
            ])
            break
          }

          case "response_chunk": {
            const delta = data.delta as string
            const messageId = data.message_id as string
            if (!delta) break

            setMessages((prev) => {
              const idx = prev.findLastIndex(
                (msg) => msg.role === "agent" && msg.messageId === messageId,
              )
              if (idx === -1) return prev

              const updated = [...prev]
              updated[idx] = { ...updated[idx], content: updated[idx].content + delta }
              return updated
            })
            break
          }

          case "response_completed": {
            const messageId = data.message_id as string
            const fullResponse = data.full_response as string | undefined
            const inputTokens = data.input_tokens as number | undefined
            const outputTokens = data.output_tokens as number | undefined
            const model = data.model as string | undefined

            setMessages((prev) => {
              const idx = prev.findLastIndex(
                (msg) => msg.role === "agent" && msg.messageId === messageId,
              )
              if (idx === -1) return prev

              const updated = [...prev]
              const existing = updated[idx]
              updated[idx] = {
                ...existing,
                content: existing.content || fullResponse || "",
                inputTokens,
                outputTokens,
                model,
              }
              return updated
            })

            streamingMessageIdRef.current = null
            setIsStreaming(false)
            break
          }

          case "turn_completed": {
            setTokenStats({
              inputTokens: (data.cumulative_input_tokens as number) ?? 0,
              outputTokens: (data.cumulative_output_tokens as number) ?? 0,
              model: data.model as string | undefined,
              turnNumber: data.turn_number as number | undefined,
            })
            break
          }

          case "agent_error": {
            const errorData = data as { message?: string; code?: string }
            setMessages((prev) => [
              ...prev,
              {
                id: event_id,
                role: "error",
                content: errorData.message ?? "An unknown error occurred",
                timestamp,
              },
            ])
            streamingMessageIdRef.current = null
            setIsStreaming(false)
            break
          }

          case "done": {
            setIsStreaming(false)
            streamingMessageIdRef.current = null
            break
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
        setIsStreaming(false)
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
    messages,
    isStreaming,
    tokenStats,
    addUserMessage,
  }
}
