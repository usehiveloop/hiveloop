"use client"

import { useState, useRef, useEffect, useCallback } from "react"
import { Button } from "@/components/ui/button"
import { MessageInput } from "@/components/message-input"
import { Streamdown } from "streamdown"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  StopIcon,
  Tick02Icon,
  Cancel02Icon,
  ArrowDown01Icon,
  ArrowUp01Icon,
  Wrench01Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useConversationStream, type StreamMessage } from "@/hooks/use-conversation-stream"
import type { RunEvent } from "../_data/agent-detail"

interface RunPanelProps {
  conversationId: string
  onClose: () => void
}

function formatTokens(count: number) {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`
  if (count >= 1_000) return `${(count / 1_000).toFixed(0)}k`
  return count.toString()
}

function UserMessage({ message }: { message: StreamMessage }) {
  return (
    <div className="rounded-xl bg-primary/10 p-4 ml-8">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-foreground/60">You</span>
      <p className="text-sm text-foreground mt-1.5 leading-relaxed">{message.content}</p>
    </div>
  )
}

function AgentMessage({ message, isStreaming }: { message: StreamMessage; isStreaming: boolean }) {
  return (
    <div className="rounded-xl border border-border p-4">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-primary">Agent</span>
      <div className="mt-1.5 text-sm text-foreground leading-relaxed prose prose-sm prose-neutral dark:prose-invert max-w-none prose-p:my-1.5 prose-headings:mt-4 prose-headings:mb-2 prose-li:my-0.5 prose-pre:my-2 prose-ul:my-1.5 prose-ol:my-1.5">
        {message.content ? (
          <Streamdown isAnimating={isStreaming}>{message.content}</Streamdown>
        ) : isStreaming ? (
          <div className="flex items-center gap-1 py-1">
            <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
            <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
            <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
          </div>
        ) : null}
      </div>
    </div>
  )
}

function ErrorMessage({ message }: { message: StreamMessage }) {
  return (
    <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-destructive">Error</span>
      <p className="text-sm text-destructive mt-1.5">{message.content}</p>
    </div>
  )
}

function MessageRenderer({ message, isLastAgent, isStreaming }: { message: StreamMessage; isLastAgent: boolean; isStreaming: boolean }) {
  switch (message.role) {
    case "user":
      return <UserMessage message={message} />
    case "agent":
      return <AgentMessage message={message} isStreaming={isLastAgent && isStreaming} />
    case "error":
      return <ErrorMessage message={message} />
    default:
      return null
  }
}

function useAutoScroll(deps: unknown[]) {
  const containerRef = useRef<HTMLDivElement>(null)
  const shouldAutoScrollRef = useRef(true)

  const handleScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return

    const distanceFromBottom = container.scrollHeight - container.scrollTop - container.clientHeight
    shouldAutoScrollRef.current = distanceFromBottom < 80
  }, [])

  useEffect(() => {
    if (!shouldAutoScrollRef.current) return
    const container = containerRef.current
    if (!container) return

    requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return { containerRef, handleScroll }
}

// ─── Legacy event renderers (kept for static/mock data) ─────────────────────

function ToolCallEvent({ event }: { event: RunEvent }) {
  const [expanded, setExpanded] = useState(false)
  const status = event.toolResult?.status
  const isRunning = status === "running"
  const isSuccess = status === "success"

  let parsedResponse: string | undefined
  if (event.toolResult?.response) {
    try {
      parsedResponse = JSON.stringify(JSON.parse(event.toolResult.response), null, 2)
    } catch {
      parsedResponse = event.toolResult.response
    }
  }

  return (
    <div className={`rounded-xl border overflow-hidden transition-colors ${isRunning ? "border-primary/30 bg-primary/[0.02]" : "border-border"}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-3 w-full px-4 py-3 text-left hover:bg-muted/50 transition-colors cursor-pointer"
      >
        <HugeiconsIcon
          icon={Wrench01Icon}
          size={14}
          className={`shrink-0 ${isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"}`}
        />
        <div className="flex-1 min-w-0">
          <span className="font-mono text-xs font-medium text-foreground">{event.toolName}</span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          {isRunning ? (
            <span className="font-mono text-[11px] text-primary">Running...</span>
          ) : event.toolResult ? (
            <span className="font-mono text-[11px] text-muted-foreground">{event.toolResult.duration}</span>
          ) : null}
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            size={14}
            className={`text-muted-foreground transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
          />
        </div>
      </button>

      <div
        className={`grid transition-all duration-200 ease-out ${
          expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]"
        }`}
      >
        <div className="overflow-hidden">
          <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
            {event.toolParams && Object.keys(event.toolParams).length > 0 && (
              <div>
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Arguments</span>
                <div className="mt-1.5 rounded-lg bg-muted p-3">
                  <div className="flex flex-col gap-1">
                    {Object.entries(event.toolParams).map(([key, value]) => (
                      <div key={key} className="flex gap-2 font-mono text-[11px]">
                        <span className="text-muted-foreground shrink-0">{key}:</span>
                        <span className="text-foreground break-all">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {parsedResponse && (
              <div>
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Response</span>
                <div className="mt-1.5 rounded-lg bg-muted p-3 overflow-x-auto">
                  <pre className="font-mono text-[11px] text-foreground whitespace-pre-wrap break-all leading-relaxed">
                    {parsedResponse}
                  </pre>
                </div>
              </div>
            )}

            {isRunning && (
              <div className="flex items-center gap-2 py-1">
                <div className="flex items-center gap-1">
                  <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
                  <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
                  <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
                </div>
                <span className="font-mono text-[11px] text-muted-foreground">Waiting for response...</span>
              </div>
            )}

            {!isRunning && (
              <div className="flex items-center gap-4 text-[11px] text-muted-foreground font-mono">
                <span>Status: <span className={isSuccess ? "text-green-500" : "text-destructive"}>{status}</span></span>
                <span>Duration: {event.toolResult?.duration}</span>
                <span>Time: {event.timestamp}</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function ApprovalEvent({ event }: { event: RunEvent }) {
  const [expanded, setExpanded] = useState(false)
  const isPending = event.approvalStatus === "pending"

  return (
    <div className={`rounded-xl border overflow-hidden ${isPending ? "border-yellow-500/30 bg-yellow-500/5" : "border-border"}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-3 w-full px-4 py-3 text-left cursor-pointer"
      >
        <span className={`h-2 w-2 rounded-full shrink-0 ${isPending ? "bg-yellow-500 animate-pulse" : "bg-muted-foreground"}`} />
        <span className="font-mono text-xs font-medium text-foreground flex-1 min-w-0 truncate">{event.toolName}</span>
        <span className="font-mono text-[10px] font-medium uppercase tracking-[0.5px] text-yellow-500 shrink-0">
          {isPending ? "Approval needed" : event.approvalStatus}
        </span>
        <HugeiconsIcon
          icon={ArrowDown01Icon}
          size={14}
          className={`text-muted-foreground transition-transform duration-200 shrink-0 ${expanded ? "rotate-180" : ""}`}
        />
      </button>

      <div
        className={`grid transition-all duration-200 ease-out ${
          expanded ? "grid-rows-[1fr]" : "grid-rows-[0fr]"
        }`}
      >
        <div className="overflow-hidden">
          <div className={`border-t px-4 py-3 flex flex-col gap-3 ${isPending ? "border-yellow-500/20" : "border-border"}`}>
            {event.toolParams && Object.keys(event.toolParams).length > 0 && (
              <div>
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Arguments</span>
                <div className="mt-1.5 rounded-lg bg-muted p-3">
                  <div className="flex flex-col gap-1.5">
                    {Object.entries(event.toolParams).map(([key, value]) => (
                      <div key={key} className="flex flex-col gap-0.5 font-mono text-[11px]">
                        <span className="text-muted-foreground">{key}</span>
                        <span className="text-foreground break-all whitespace-pre-wrap">{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {isPending && (
        <div className={`flex items-center gap-2 px-4 py-3 border-t ${isPending ? "border-yellow-500/20" : "border-border"}`}>
          <Button size="sm" variant="default" className="h-7 text-xs">
            <HugeiconsIcon icon={Tick02Icon} size={12} data-icon="inline-start" />
            Approve
          </Button>
          <Button size="sm" variant="outline" className="h-7 text-xs">
            <HugeiconsIcon icon={Cancel02Icon} size={12} data-icon="inline-start" />
            Deny
          </Button>
        </div>
      )}
    </div>
  )
}

function LegacyEventRenderer({ event }: { event: RunEvent }) {
  switch (event.type) {
    case "tool_call":
      return <ToolCallEvent event={event} />
    case "approval":
      return <ApprovalEvent event={event} />
    default:
      return null
  }
}

// ─── Main panel ─────────────────────────────────────────────────────────────

export function RunPanel({ conversationId, onClose }: RunPanelProps) {
  const [closing, setClosing] = useState(false)
  const {
    connected,
    connecting,
    error,
    messages,
    isStreaming,
    tokenStats,
    addUserMessage,
  } = useConversationStream(conversationId)

  const lastAgentIdx = messages.findLastIndex((msg) => msg.role === "agent")
  const isActive = connected || connecting

  const { containerRef, handleScroll } = useAutoScroll([messages, isStreaming])

  const sendMessage = $api.useMutation("post", "/v1/conversations/{convID}/messages")

  function handleSendMessage(content: string) {
    addUserMessage(content)

    sendMessage.mutate(
      { params: { path: { convID: conversationId } }, body: { content } },
      {
        onError: (err) => {
          toast.error(extractErrorMessage(err, "Failed to send message"))
        },
      },
    )
  }

  function handleClose() {
    setClosing(true)
    setTimeout(onClose, 200)
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className={`fixed inset-0 z-[60] transition-opacity duration-200 ${closing ? "opacity-0" : "opacity-100"}`}
        onClick={handleClose}
      />

      {/* Panel */}
      <div className={`fixed inset-4 sm:inset-6 lg:left-auto lg:inset-y-6 lg:right-6 lg:w-[580px] z-[70] flex flex-col rounded-2xl border border-border bg-background shadow-2xl shadow-black/20 transition-all duration-200 ${closing ? "opacity-0 translate-x-4" : "animate-in slide-in-from-right-4 fade-in duration-200"}`}>
        {/* Header */}
        <div className="flex items-center justify-between shrink-0 px-5 py-3.5 border-b border-border rounded-t-2xl">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2">
              {isActive && <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />}
              <h2 className="text-sm font-semibold text-foreground truncate">
                {connecting ? "Connecting..." : "Run"}
              </h2>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5 font-mono truncate">
              {conversationId}
            </p>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            {connected && (
              <Button variant="destructive" size="sm" className="h-7 text-xs">
                <HugeiconsIcon icon={StopIcon} size={12} data-icon="inline-start" />
                Kill run
              </Button>
            )}
            <button onClick={handleClose} className="flex items-center justify-center h-8 w-8 rounded-lg hover:bg-muted transition-colors">
              <HugeiconsIcon icon={Cancel01Icon} size={16} className="text-muted-foreground" />
            </button>
          </div>
        </div>

        {/* Event stream */}
        <div
          ref={containerRef}
          onScroll={handleScroll}
          className="flex-1 overflow-y-auto px-5 py-4"
        >
          {connecting && (
            <div className="flex flex-col items-center justify-center py-12 gap-3">
              <HugeiconsIcon icon={Loading03Icon} size={24} className="text-muted-foreground animate-spin" />
              <p className="text-sm text-muted-foreground">Connecting to stream...</p>
            </div>
          )}

          {error && !messages.length && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-destructive">Connection error</span>
              <p className="text-sm text-destructive mt-1.5">{error}</p>
            </div>
          )}

          {connected && messages.length === 0 && !error && (
            <div className="flex flex-col items-center justify-center py-12 gap-2">
              <div className="flex items-center gap-1">
                <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
                <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
                <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
              </div>
              <p className="text-sm text-muted-foreground">Waiting for agent...</p>
            </div>
          )}

          {messages.length > 0 && (
            <div className="flex flex-col gap-3">
              {messages.map((message, index) => (
                <MessageRenderer
                  key={message.id}
                  message={message}
                  isLastAgent={index === lastAgentIdx}
                  isStreaming={isStreaming}
                />
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="shrink-0 rounded-b-2xl">
          {/* Token stats */}
          <div className="flex items-center justify-between px-4 py-1 text-xs text-muted-foreground">
            <span className="flex items-center gap-3 tabular-nums">
              <span className="flex items-center gap-1">
                <HugeiconsIcon icon={ArrowDown01Icon} size={12} />
                {formatTokens(tokenStats.inputTokens)}
              </span>
              <span className="flex items-center gap-1">
                <HugeiconsIcon icon={ArrowUp01Icon} size={12} />
                {formatTokens(tokenStats.outputTokens)}
              </span>
            </span>
            {tokenStats.model && (
              <span className="font-mono text-[10px] text-muted-foreground/50 truncate max-w-48">
                {tokenStats.model}
              </span>
            )}
          </div>

          {/* Message input */}
          {isActive && (
            <div className="px-3 pb-3">
              <MessageInput
                placeholder="Send a message to the run..."
                onSend={handleSendMessage}
                disabled={sendMessage.isPending}
              />
            </div>
          )}

        </div>
      </div>
    </>
  )
}
