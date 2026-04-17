"use client"

import { useState } from "react"
import { AnimatePresence, motion } from "motion/react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Wrench01Icon,
  ArrowDown01Icon,
  Robot01Icon,
  SparklesIcon,
  CheckmarkCircle02Icon,
  AlertCircleIcon,
  FlashIcon,
} from "@hugeicons/core-free-icons"
import {
  type TimelineItem,
  formatDuration,
  formatTimestamp,
  formatTokens,
} from "../../_lib/conversation-events"

const LONG_CONTENT_THRESHOLD = 1200

interface CollapsibleMarkdownProps {
  content: string
}

function CollapsibleMarkdown({ content }: CollapsibleMarkdownProps) {
  const [expanded, setExpanded] = useState(false)
  const isLong = content.length > LONG_CONTENT_THRESHOLD
  const displayed = expanded || !isLong ? content : content.slice(0, LONG_CONTENT_THRESHOLD) + "\n\n…"

  return (
    <div className="text-[13px] leading-[1.6] text-foreground [&_pre]:my-2 [&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-[11px] [&_pre]:overflow-x-auto [&_code]:font-mono [&_code]:text-[11px] [&_p]:my-2 [&_ul]:my-2 [&_ul]:ml-5 [&_ul]:list-disc [&_ol]:my-2 [&_ol]:ml-5 [&_ol]:list-decimal [&_table]:my-2 [&_table]:w-full [&_table]:border-collapse [&_th]:border [&_th]:border-border/40 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left [&_td]:border [&_td]:border-border/40 [&_td]:px-2 [&_td]:py-1 [&_h1]:my-2 [&_h1]:text-[15px] [&_h1]:font-semibold [&_h2]:my-2 [&_h2]:text-[14px] [&_h2]:font-semibold [&_h3]:my-2 [&_h3]:text-[13px] [&_h3]:font-semibold">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{displayed}</ReactMarkdown>
      {isLong && (
        <button
          onClick={() => setExpanded(!expanded)}
          className="mt-2 text-[11px] font-medium text-primary hover:underline cursor-pointer"
        >
          {expanded ? "Show less" : `Show all (${content.length.toLocaleString()} chars)`}
        </button>
      )}
    </div>
  )
}

interface SystemMarkerProps {
  timestamp: string
}

function ConversationCreatedItem({ timestamp }: SystemMarkerProps) {
  return (
    <div className="flex items-center justify-center gap-2 py-3">
      <div className="h-px flex-1 bg-border/30" />
      <div className="flex items-center gap-2 rounded-full bg-muted/30 px-3 py-1">
        <HugeiconsIcon icon={SparklesIcon} size={10} className="text-muted-foreground/50" />
        <span className="text-[10px] text-muted-foreground/60 font-mono uppercase tracking-wider">
          Conversation started
        </span>
        <span className="text-[10px] text-muted-foreground/30 font-mono">{formatTimestamp(timestamp)}</span>
      </div>
      <div className="h-px flex-1 bg-border/30" />
    </div>
  )
}

function DoneItem({ timestamp }: SystemMarkerProps) {
  return (
    <div className="flex items-center justify-center gap-2 py-3">
      <div className="h-px flex-1 bg-border/30" />
      <span className="text-[10px] text-muted-foreground/40 font-mono uppercase tracking-wider">
        Ended · {formatTimestamp(timestamp)}
      </span>
      <div className="h-px flex-1 bg-border/30" />
    </div>
  )
}

interface TriggerMessageCardProps {
  content: string
  timestamp: string
  index: number
}

function extractTriggerPreview(content: string): string {
  for (const rawLine of content.split("\n")) {
    const stripped = rawLine.replace(/^#+\s*/, "").replace(/^[-*]\s+/, "").trim()
    if (stripped.length > 0) return stripped
  }
  return ""
}

function TriggerMessageCard({ content, timestamp, index }: TriggerMessageCardProps) {
  const [expanded, setExpanded] = useState(false)
  const preview = extractTriggerPreview(content)
  const lineCount = content.split("\n").length

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: Math.min(index * 0.02, 0.2), type: "spring", stiffness: 400, damping: 28 }}
      className="my-2"
    >
      <div className="rounded-xl border border-border/50 overflow-hidden">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-left hover:bg-muted/20 transition-colors cursor-pointer"
        >
          <div className="h-5 w-5 rounded-lg flex items-center justify-center shrink-0 bg-primary/10">
            <HugeiconsIcon icon={SparklesIcon} size={11} className="text-primary" />
          </div>
          <div className="flex-1 min-w-0 flex items-center gap-2">
            <span className="font-mono text-[11px] font-medium text-foreground/70 shrink-0">Trigger</span>
            {preview && (
              <span
                className="font-mono text-[11px] text-muted-foreground/50 truncate min-w-0"
                title={preview}
              >
                <span className="text-muted-foreground/30 mr-1">·</span>
                {preview}
              </span>
            )}
          </div>
          <span className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-muted/60 text-muted-foreground/70 shrink-0">
            {lineCount} lines
          </span>
          <span className="font-mono text-[10px] text-muted-foreground/30 shrink-0">
            {formatTimestamp(timestamp)}
          </span>
          <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.15 }}>
            <HugeiconsIcon icon={ArrowDown01Icon} size={10} className="text-muted-foreground/25" />
          </motion.div>
        </button>
        <AnimatePresence>
          {expanded && (
            <motion.div
              initial={{ height: 0 }}
              animate={{ height: "auto" }}
              exit={{ height: 0 }}
              transition={{ type: "spring", stiffness: 500, damping: 35 }}
              className="overflow-hidden"
            >
              <div className="border-t border-border/30 px-4 py-3 max-h-[60vh] overflow-auto">
                <div className="text-[13px] leading-[1.6] text-foreground [&_pre]:my-2 [&_pre]:rounded-lg [&_pre]:bg-muted/60 [&_pre]:p-3 [&_pre]:text-[11px] [&_pre]:overflow-x-auto [&_code]:font-mono [&_code]:text-[11px] [&_p]:my-2 [&_ul]:my-2 [&_ul]:ml-5 [&_ul]:list-disc [&_ol]:my-2 [&_ol]:ml-5 [&_ol]:list-decimal [&_table]:my-2 [&_table]:w-full [&_table]:border-collapse [&_th]:border [&_th]:border-border/40 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left [&_td]:border [&_td]:border-border/40 [&_td]:px-2 [&_td]:py-1 [&_h1]:my-2 [&_h1]:text-[15px] [&_h1]:font-semibold [&_h2]:my-2 [&_h2]:text-[14px] [&_h2]:font-semibold [&_h3]:my-2 [&_h3]:text-[13px] [&_h3]:font-semibold">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  )
}

interface AgentResponseItemProps {
  content: string
  timestamp: string
  model?: string
  inputTokens?: number
  outputTokens?: number
  status: "streaming" | "completed"
  index: number
}

function AgentResponseItem({
  content,
  timestamp,
  model,
  inputTokens,
  outputTokens,
  status,
  index,
}: AgentResponseItemProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: Math.min(index * 0.02, 0.2), type: "spring", stiffness: 400, damping: 28 }}
      className="flex gap-3 py-3"
    >
      <div className="h-7 w-7 rounded-full bg-primary/20 border border-primary/10 flex items-center justify-center shrink-0 mt-0.5">
        <HugeiconsIcon icon={Robot01Icon} size={13} className="text-primary" />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-baseline gap-2 mb-1.5">
          <span className="text-[13px] font-semibold text-primary">Agent</span>
          <span className="text-[10px] text-muted-foreground/30 font-mono">{formatTimestamp(timestamp)}</span>
          {model && <span className="text-[10px] text-muted-foreground/25 font-mono">· {model}</span>}
        </div>
        {status === "streaming" && !content ? (
          <div className="flex items-center gap-1.5 py-2">
            <span className="h-1.5 w-1.5 rounded-full bg-primary/60 animate-[bounce_1s_ease-in-out_infinite]" />
            <span className="h-1.5 w-1.5 rounded-full bg-primary/60 animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
            <span className="h-1.5 w-1.5 rounded-full bg-primary/60 animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
          </div>
        ) : (
          <div className="min-w-0 overflow-hidden">
            <CollapsibleMarkdown content={content} />
          </div>
        )}
        {status === "completed" && inputTokens != null && outputTokens != null && (
          <div className="mt-2 flex items-center gap-3 text-[10px] text-muted-foreground/30 font-mono">
            <span>↓ {formatTokens(inputTokens)}</span>
            <span>↑ {formatTokens(outputTokens)}</span>
          </div>
        )}
      </div>
    </motion.div>
  )
}

interface ToolCallItemProps {
  toolName: string
  args: Record<string, unknown>
  status: "running" | "success" | "failed"
  result?: string
  durationMs?: number
  subtitle?: string
  subtitleFull?: string
  meta?: string[]
  index: number
}

function ToolCallItem({
  toolName,
  args,
  status,
  result,
  durationMs,
  subtitle,
  subtitleFull,
  meta,
  index,
}: ToolCallItemProps) {
  const [expanded, setExpanded] = useState(false)

  const statusColor =
    status === "running" ? "text-primary" : status === "success" ? "text-green-500" : "text-destructive"
  const statusBg =
    status === "running" ? "bg-primary/10" : status === "success" ? "bg-green-500/8" : "bg-destructive/8"
  const borderColor = status === "running" ? "border-primary/20" : "border-border/50"

  const hasBody = Object.keys(args).length > 0 || !!result

  return (
    <motion.div
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: Math.min(index * 0.02, 0.2), type: "spring", stiffness: 400, damping: 28 }}
      className="ml-10 my-1.5"
    >
      <div className={`rounded-xl border overflow-hidden ${borderColor}`}>
        <button
          onClick={() => hasBody && setExpanded(!expanded)}
          disabled={!hasBody}
          className={`flex items-center gap-2.5 w-full px-3.5 py-2.5 text-left transition-colors ${
            hasBody ? "hover:bg-muted/20 cursor-pointer" : "cursor-default"
          }`}
        >
          <div className={`h-5 w-5 rounded-lg flex items-center justify-center shrink-0 ${statusBg}`}>
            <HugeiconsIcon
              icon={
                status === "running"
                  ? Wrench01Icon
                  : status === "success"
                    ? CheckmarkCircle02Icon
                    : AlertCircleIcon
              }
              size={11}
              className={`${statusColor} ${status === "running" ? "animate-spin" : ""}`}
            />
          </div>
          <div className="flex-1 min-w-0 flex items-center gap-2">
            <span className="font-mono text-[11px] font-medium text-foreground/70 shrink-0 truncate max-w-[55%]">
              {toolName}
            </span>
            {subtitle && (
              <span
                className="font-mono text-[11px] text-muted-foreground/50 truncate min-w-0"
                title={subtitleFull ?? subtitle}
              >
                <span className="text-muted-foreground/30 mr-1">·</span>
                {subtitle}
              </span>
            )}
          </div>
          {meta && meta.length > 0 && (
            <div className="flex items-center gap-1 shrink-0">
              {meta.map((badge) => (
                <span
                  key={badge}
                  className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-muted/60 text-muted-foreground/70"
                >
                  {badge}
                </span>
              ))}
            </div>
          )}
          {status === "running" ? (
            <div className="flex items-center gap-1 shrink-0">
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
            </div>
          ) : (
            durationMs != null && (
              <span className="font-mono text-[10px] text-muted-foreground/30 shrink-0">
                {formatDuration(durationMs)}
              </span>
            )
          )}
          {hasBody && (
            <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.15 }}>
              <HugeiconsIcon icon={ArrowDown01Icon} size={10} className="text-muted-foreground/25" />
            </motion.div>
          )}
        </button>
        <AnimatePresence>
          {expanded && hasBody && (
            <motion.div
              initial={{ height: 0 }}
              animate={{ height: "auto" }}
              exit={{ height: 0 }}
              transition={{ type: "spring", stiffness: 500, damping: 35 }}
              className="overflow-hidden"
            >
              <div className="border-t border-border/30 px-3.5 py-2.5 flex flex-col gap-2">
                {Object.keys(args).length > 0 && (
                  <div className="rounded-lg bg-muted/40 p-2.5">
                    {Object.entries(args).map(([key, value]) => (
                      <div key={key} className="font-mono text-[10px] leading-relaxed">
                        <span className="text-muted-foreground/50">{key}: </span>
                        <span className="text-foreground/70 whitespace-pre-wrap break-all">
                          {formatArg(value)}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
                {result && (
                  <div className="rounded-lg bg-muted/40 p-2.5 max-h-80 overflow-auto">
                    <pre className="font-mono text-[10px] text-foreground/70 whitespace-pre-wrap break-all leading-relaxed">
                      {result}
                    </pre>
                  </div>
                )}
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </motion.div>
  )
}

interface TurnSummaryItemProps {
  turnNumber: number
  model: string
  inputTokens: number
  outputTokens: number
}

function TurnSummaryItem({ turnNumber, model, inputTokens, outputTokens }: TurnSummaryItemProps) {
  return (
    <div className="flex items-center justify-center gap-2 py-1.5">
      <div className="flex items-center gap-2 rounded-full bg-muted/20 px-3 py-1 text-[10px] font-mono text-muted-foreground/40">
        <HugeiconsIcon icon={FlashIcon} size={10} className="text-muted-foreground/40" />
        <span>turn {turnNumber}</span>
        <span>·</span>
        <span>{model}</span>
        <span>·</span>
        <span>↓ {formatTokens(inputTokens)}</span>
        <span>↑ {formatTokens(outputTokens)}</span>
      </div>
    </div>
  )
}

function formatArg(value: unknown): string {
  if (typeof value === "string") return value
  if (value == null) return String(value)
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

interface ConversationTimelineProps {
  items: TimelineItem[]
}

export function ConversationTimeline({ items }: ConversationTimelineProps) {
  return (
    <div className="flex flex-col gap-1">
      {items.map((item, index) => {
        switch (item.kind) {
          case "conversation_created":
            return <ConversationCreatedItem key={item.id} timestamp={item.timestamp} />
          case "message_received":
            return (
              <TriggerMessageCard
                key={item.id}
                content={item.content}
                timestamp={item.timestamp}
                index={index}
              />
            )
          case "agent_response":
            return (
              <AgentResponseItem
                key={item.id}
                content={item.content}
                timestamp={item.timestamp}
                model={item.model}
                inputTokens={item.inputTokens}
                outputTokens={item.outputTokens}
                status={item.status}
                index={index}
              />
            )
          case "tool_call":
            return (
              <ToolCallItem
                key={item.id}
                toolName={item.toolName}
                args={item.args}
                status={item.status}
                result={item.result}
                durationMs={item.durationMs}
                subtitle={item.subtitle}
                subtitleFull={item.subtitleFull}
                meta={item.meta}
                index={index}
              />
            )
          case "turn_summary":
            return (
              <TurnSummaryItem
                key={item.id}
                turnNumber={item.turnNumber}
                model={item.model}
                inputTokens={item.inputTokens}
                outputTokens={item.outputTokens}
              />
            )
          case "done":
            return <DoneItem key={item.id} timestamp={item.timestamp} />
        }
      })}
    </div>
  )
}
