"use client"

import { useMemo, useState } from "react"
import { useParams } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import ScrollToBottom from "react-scroll-to-bottom"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BrowserIcon,
  CommandLineIcon,
  MoreHorizontalIcon,
  SentIcon,
  LinkSquare02Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import {
  buildTimeline,
  normalizeEvents,
  type TimelineItem,
} from "../../_lib/conversation-events"
import { ConversationTimeline } from "../../_components/conversation/timeline"

function BrowserPanel() {
  return (
    <div className="flex flex-col h-full border-b border-border">
      <div className="flex items-center gap-2.5 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-full bg-destructive/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-chart-1/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-green-500/40" />
        </div>
        <div className="flex-1 flex items-center rounded-xl bg-background border border-border px-3 py-1.5">
          <HugeiconsIcon icon={LinkSquare02Icon} size={11} className="text-muted-foreground/40 mr-2" />
          <span className="text-[11px] text-muted-foreground/40 font-mono truncate">No active browser session</span>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4 flex items-center justify-center text-[12px] text-muted-foreground/40">
        Browser panel is not wired up yet
      </div>
    </div>
  )
}

function TerminalPanel() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <HugeiconsIcon icon={CommandLineIcon} size={12} className="text-muted-foreground" />
        <span className="text-[11px] font-mono text-muted-foreground">Terminal</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-foreground p-4 flex items-center justify-center">
        <span className="font-mono text-[11px] text-background/40">No active terminal session</span>
      </div>
    </div>
  )
}

function EmptyTimeline() {
  return (
    <div className="flex items-center justify-center py-20 text-[13px] text-muted-foreground/40">
      No events yet. The agent will stream events here as it works.
    </div>
  )
}

function TimelineSkeleton() {
  return (
    <div className="flex flex-col gap-4 py-6">
      {[0, 1, 2].map((i) => (
        <div key={i} className="flex gap-3">
          <Skeleton className="h-7 w-7 rounded-full shrink-0" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        </div>
      ))}
    </div>
  )
}

function deriveTitle(items: TimelineItem[], fallback: string): string {
  const firstMessage = items.find((item) => item.kind === "message_received")
  if (firstMessage && firstMessage.kind === "message_received") {
    const firstLine = firstMessage.content.split("\n").find((line) => line.trim().length > 0) ?? ""
    const cleaned = firstLine.replace(/^#+\s*/, "").trim()
    if (cleaned.length > 0) return cleaned.slice(0, 80)
  }
  return fallback
}

function deriveStatus(items: TimelineItem[]): "active" | "ended" {
  const last = items[items.length - 1]
  if (!last) return "active"
  if (last.kind === "done") return "ended"
  if (last.kind === "agent_response" && last.status === "completed") return "ended"
  return "active"
}

export default function ConversationPage() {
  const params = useParams()
  const conversationId = params.conversationId as string

  const [showBrowser, setShowBrowser] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const panelsOpen = showBrowser || showTerminal

  const { data, isLoading, error } = $api.useQuery(
    "get",
    "/v1/conversations/{convID}/events",
    {
      params: { path: { convID: conversationId }, query: { limit: 100 } },
    },
    { enabled: !!conversationId },
  )

  const { data: conversation } = $api.useQuery(
    "get",
    "/v1/conversations/{convID}",
    {
      params: { path: { convID: conversationId } },
    },
    { enabled: !!conversationId, refetchInterval: 5000 },
  )

  const timeline = useMemo(() => {
    const events = normalizeEvents(data?.data ?? [])
    return buildTimeline(events)
  }, [data])

  const hasMore = data?.has_more ?? false
  const title = conversation?.name || deriveTitle(timeline, conversationId.slice(0, 8))
  const status = deriveStatus(timeline)
  const lastAgentResponse = [...timeline].reverse().find((item) => item.kind === "agent_response")
  const modelLabel =
    lastAgentResponse && lastAgentResponse.kind === "agent_response" ? lastAgentResponse.model : undefined

  return (
    <>
      <div className="flex flex-col w-full max-w-[728px] shrink-0 border-r border-border">
        <div className="flex items-center justify-between px-8 py-3.5 border-b border-border shrink-0">
          <div className="flex items-center gap-3 min-w-0">
            <motion.div
              animate={
                status === "active"
                  ? { scale: [1, 1.15, 1] }
                  : { scale: 1 }
              }
              transition={{ repeat: status === "active" ? Infinity : 0, duration: 2.5, ease: "easeInOut" }}
              className={`h-2 w-2 rounded-full shrink-0 ${status === "active" ? "bg-green-500" : "bg-muted-foreground/30"}`}
            />
            <h2 className="text-[15px] font-semibold text-foreground truncate">{title}</h2>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <Button
              variant={showBrowser ? "secondary" : "outline"}
              size="sm"
              onClick={() => setShowBrowser(!showBrowser)}
              className="h-7 text-xs"
            >
              <HugeiconsIcon icon={BrowserIcon} size={13} data-icon="inline-start" />
              Browser
            </Button>
            <Button
              variant={showTerminal ? "secondary" : "outline"}
              size="sm"
              onClick={() => setShowTerminal(!showTerminal)}
              className="h-7 text-xs"
            >
              <HugeiconsIcon icon={CommandLineIcon} size={13} data-icon="inline-start" />
              Terminal
            </Button>
            <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors ml-1">
              <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
            </button>
          </div>
        </div>

        <ScrollToBottom
          className="flex-1 min-h-0"
          scrollViewClassName="px-8 py-6"
          followButtonClassName="hidden"
          initialScrollBehavior="auto"
        >
          {isLoading ? (
            <TimelineSkeleton />
          ) : error ? (
            <div className="py-10 text-center text-[13px] text-destructive/70">
              Failed to load conversation events.
            </div>
          ) : timeline.length === 0 ? (
            <EmptyTimeline />
          ) : (
            <>
              <ConversationTimeline items={timeline} />
              {hasMore && (
                <div className="mt-6 text-center text-[11px] text-muted-foreground/40 font-mono">
                  More events available — pagination not yet wired.
                </div>
              )}
            </>
          )}
        </ScrollToBottom>

        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.2, type: "spring", stiffness: 400, damping: 30 }}
          className="shrink-0 px-8 pb-6"
        >
          <div className="rounded-2xl border border-border bg-muted/10 p-1.5 transition-colors focus-within:border-primary/30">
            <Textarea
              placeholder="Composer is not wired up yet"
              disabled
              className="border-0 bg-transparent min-h-[60px] max-h-32 focus-visible:ring-0 focus-visible:border-transparent text-[14px]"
            />
            <div className="flex items-center justify-between px-2 pt-1">
              <span className="font-mono text-[9px] text-muted-foreground/20">
                {modelLabel ?? "—"}
              </span>
              <button
                disabled
                className="h-7 w-7 rounded-lg bg-primary/40 text-primary-foreground flex items-center justify-center"
              >
                <HugeiconsIcon icon={SentIcon} size={13} />
              </button>
            </div>
          </div>
        </motion.div>
      </div>

      <AnimatePresence>
        {panelsOpen && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ type: "spring", stiffness: 400, damping: 35 }}
            className="flex flex-col flex-1 min-w-0 overflow-hidden"
          >
            {showBrowser && (
              <div className={showTerminal ? "h-[60%]" : "h-full"}>
                <BrowserPanel />
              </div>
            )}
            {showTerminal && (
              <div className={showBrowser ? "h-[40%]" : "h-full"}>
                <TerminalPanel />
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
