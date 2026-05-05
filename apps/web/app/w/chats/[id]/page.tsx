"use client"

import { useEffect, useRef, useState } from "react"
import { useParams, useRouter, useSearchParams } from "next/navigation"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowUp02Icon, Loading03Icon } from "@hugeicons/core-free-icons"
import { PageHeader } from "@/components/page-header"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { apiUrl } from "@/lib/api/client"

type Msg = { id: string; role: string; content: string; created_at: string }

export default function ChatSessionPage() {
  const params = useParams<{ id: string }>()
  const sessionId = params.id
  const search = useSearchParams()
  const router = useRouter()

  const [messages, setMessages] = useState<Msg[]>([])
  const [streaming, setStreaming] = useState<string>("")
  const [draft, setDraft] = useState("")
  const [streamActive, setStreamActive] = useState(false)
  const esRef = useRef<EventSource | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const { data: detail, isLoading: detailLoading } = $api.useQuery(
    "get",
    "/v1/chats/{id}",
    { params: { path: { id: sessionId } } },
  )

  useEffect(() => {
    if (detail?.messages) {
      setMessages(
        detail.messages.map((m) => ({
          id: m.id ?? "",
          role: m.role ?? "",
          content: m.content ?? "",
          created_at: m.created_at ?? "",
        })),
      )
    }
  }, [detail])

  useEffect(() => {
    const initial = search.get("stream")
    if (!initial) return
    openStream(decodeURIComponent(initial))
    router.replace(`/w/chats/${sessionId}`)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    scrollRef.current?.scrollTo({
      top: scrollRef.current.scrollHeight,
      behavior: "smooth",
    })
  }, [messages, streaming])

  useEffect(
    () => () => {
      esRef.current?.close()
    },
    [],
  )

  function openStream(streamPath: string) {
    esRef.current?.close()
    const url = apiUrl(streamPath)
    const es = new EventSource(url, { withCredentials: false })
    esRef.current = es
    setStreamActive(true)
    let assembled = ""

    es.onmessage = (ev) => {
      if (ev.data === "[DONE]") {
        finalize(assembled)
        return
      }
      try {
        const chunk = JSON.parse(ev.data)
        const delta = chunk?.choices?.[0]?.delta?.content ?? ""
        if (delta) {
          assembled += delta
          setStreaming(assembled)
        }
      } catch {
        // ignore non-JSON keep-alive lines
      }
    }
    es.addEventListener("error", () => {
      es.close()
      finalize(assembled)
    })

    function finalize(text: string) {
      esRef.current = null
      setStreaming("")
      setStreamActive(false)
      if (text) {
        setMessages((m) => [
          ...m,
          {
            id: `local-${Date.now()}`,
            role: "assistant",
            content: text,
            created_at: new Date().toISOString(),
          },
        ])
      }
    }
  }

  const sendMessage = $api.useMutation("post", "/v1/chats/{id}/messages")

  function handleSend() {
    if (!draft.trim() || sendMessage.isPending || streamActive) return
    const text = draft.trim()
    setDraft("")
    setMessages((m) => [
      ...m,
      {
        id: `local-${Date.now()}`,
        role: "user",
        content: text,
        created_at: new Date().toISOString(),
      },
    ])
    sendMessage.mutate(
      {
        params: { path: { id: sessionId } },
        body: { message: text },
      },
      {
        onSuccess: (data) => {
          if (data.stream_url) openStream(data.stream_url)
        },
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to send message"))
        },
      },
    )
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <>
      <PageHeader title="Chat" />
      <div className="mx-auto flex h-[calc(100vh-64px)] w-full max-w-3xl flex-col px-6 pt-6">
        <div ref={scrollRef} className="flex-1 space-y-4 overflow-y-auto pb-6">
          {detailLoading && messages.length === 0 ? (
            <div className="flex justify-center py-10 text-muted-foreground">
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-5 animate-spin"
              />
            </div>
          ) : null}
          {messages.map((m) => (
            <MessageBubble key={m.id} role={m.role} content={m.content} />
          ))}
          {streaming ? (
            <MessageBubble role="assistant" content={streaming} streaming />
          ) : null}
        </div>

        <div className="sticky bottom-6 mt-4 rounded-2xl border border-border bg-background transition-colors focus-within:border-foreground/30">
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={streamActive ? "Waiting for reply…" : "Reply…"}
            disabled={streamActive}
            className="block h-[100px] w-full resize-none bg-transparent px-4 pt-4 text-[14.5px] text-foreground outline-none placeholder:text-muted-foreground/70 disabled:opacity-50"
          />
          <div className="flex items-center justify-end gap-2 px-3 pt-1 pb-3">
            <button
              type="button"
              onClick={handleSend}
              disabled={!draft.trim() || sendMessage.isPending || streamActive}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-30"
            >
              {sendMessage.isPending || streamActive ? (
                <HugeiconsIcon
                  icon={Loading03Icon}
                  size={14}
                  className="animate-spin"
                />
              ) : (
                <HugeiconsIcon icon={ArrowUp02Icon} size={16} />
              )}
            </button>
          </div>
        </div>
      </div>
    </>
  )
}

function MessageBubble({
  role,
  content,
  streaming,
}: {
  role: string
  content: string
  streaming?: boolean
}) {
  const isUser = role === "user"
  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[80%] whitespace-pre-wrap rounded-2xl px-4 py-2.5 text-[14.5px] leading-relaxed ${
          isUser
            ? "bg-primary text-primary-foreground"
            : "bg-muted text-foreground"
        }`}
      >
        {content}
        {streaming ? (
          <span className="ml-0.5 inline-block h-3 w-1 animate-pulse bg-current align-text-bottom" />
        ) : null}
      </div>
    </div>
  )
}
