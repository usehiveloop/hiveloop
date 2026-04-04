"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { MessageInput } from "@/components/message-input"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  StopIcon,
  Tick02Icon,
  Cancel02Icon,
  ArrowDown01Icon,
  ArrowUp01Icon,
  Wrench01Icon,
} from "@hugeicons/core-free-icons"
import type { Run, RunEvent } from "../_data/agent-detail"

type RunPanelProps = {
  run: Run
  onClose: () => void
}

function formatTokens(n: number) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}k`
  return n.toString()
}

function SystemMessage({ event }: { event: RunEvent }) {
  return (
    <div className="rounded-xl bg-muted p-4">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">System</span>
      <p className="text-sm text-muted-foreground mt-1.5 leading-relaxed">{event.content}</p>
    </div>
  )
}

function UserMessage({ event }: { event: RunEvent }) {
  return (
    <div className="rounded-xl bg-primary/10 p-4 ml-8">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-foreground/60">You</span>
      <p className="text-sm text-foreground mt-1.5 leading-relaxed">{event.content}</p>
    </div>
  )
}

function AgentMessage({ event }: { event: RunEvent }) {
  return (
    <div className="rounded-xl border border-border p-4">
      <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-primary">Agent</span>
      <p className="text-sm text-foreground mt-1.5 leading-relaxed">{event.content}</p>
    </div>
  )
}

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
        className="grid transition-all duration-200 ease-out"
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
            {/* Arguments */}
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

            {/* Response */}
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

            {/* Running state */}
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

            {/* Meta */}
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
      {/* Header — always visible */}
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

      {/* Expandable details */}
      <div
        className="grid transition-all duration-200 ease-out"
        style={{ gridTemplateRows: expanded ? "1fr" : "0fr" }}
      >
        <div className="overflow-hidden">
          <div className={`border-t px-4 py-3 flex flex-col gap-3 ${isPending ? "border-yellow-500/20" : "border-border"}`}>
            {/* Full arguments */}
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

      {/* Approve / Deny — always visible */}
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

function ThinkingIndicator() {
  return (
    <div className="flex items-center gap-2 px-4 py-3">
      <div className="flex items-center gap-1">
        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
        <span className="h-1.5 w-1.5 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
      </div>
      <span className="text-xs text-muted-foreground">Agent is thinking...</span>
    </div>
  )
}

function EventRenderer({ event }: { event: RunEvent }) {
  switch (event.type) {
    case "system":
      return <SystemMessage event={event} />
    case "user":
      return <UserMessage event={event} />
    case "agent":
      return <AgentMessage event={event} />
    case "tool_call":
      return <ToolCallEvent event={event} />
    case "approval":
      return <ApprovalEvent event={event} />
    case "thinking":
      return <ThinkingIndicator />
    case "error":
      return (
        <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-destructive">Error</span>
          <p className="text-sm text-destructive mt-1.5">{event.content}</p>
        </div>
      )
    default:
      return null
  }
}

export function RunPanel({ run, onClose }: RunPanelProps) {
  const [closing, setClosing] = useState(false)
  const isActive = run.status === "running" || run.status === "waiting_approval"

  function handleClose() {
    setClosing(true)
    setTimeout(onClose, 200)
  }

  return (
    <>
      {/* Backdrop — click to close */}
      <div
        className={`fixed inset-0 z-[60] transition-opacity duration-200 ${closing ? "opacity-0" : "opacity-100"}`}
        onClick={handleClose}
      />

      {/* Panel */}
      <div className={`fixed inset-4 sm:inset-6 lg:left-auto lg:inset-y-6 lg:right-6 lg:w-[580px] z-[70] flex flex-col rounded-2xl border border-border bg-background shadow-2xl shadow-black/20 transition-all duration-200 ${closing ? "opacity-0 translate-x-4" : "animate-in slide-in-from-right-4 fade-in duration-200"}`}>
      {/* Panel header */}
      <div className="flex items-center justify-between shrink-0 px-5 py-3.5 border-b border-border rounded-t-2xl">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            {isActive && <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />}
            <h2 className="text-sm font-semibold text-foreground truncate">{run.subject}</h2>
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">
            {run.identity} · {isActive ? `Running · ${run.duration}` : `${run.status === "error" ? "Failed" : "Completed"} · ${run.duration}`}
          </p>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {isActive && (
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
      <div className="flex-1 overflow-y-auto px-5 py-4">
        <div className="flex flex-col gap-3">
          {run.events.map((event) => (
            <EventRenderer key={event.id} event={event} />
          ))}
        </div>
      </div>

      {/* Footer */}
      <div className="shrink-0 rounded-b-2xl">
        {/* Token stats */}
        <div className="flex items-center justify-between px-4 py-1 text-xs text-muted-foreground">
          <span className="flex items-center gap-3 tabular-nums">
            <span className="flex items-center gap-1"><HugeiconsIcon icon={ArrowDown01Icon} size={12} /> {formatTokens(run.tokensIn)}</span>
            <span className="flex items-center gap-1"><HugeiconsIcon icon={ArrowUp01Icon} size={12} /> {formatTokens(run.tokensOut)}</span>
          </span>
          <span className="tabular-nums">${run.cost.toFixed(2)} <span className="text-muted-foreground/50">/ $5.00</span></span>
        </div>

        {/* Message input */}
        {isActive && (
          <div className="px-3 pb-3">
            <MessageInput placeholder="Send a message to the run..." />
          </div>
        )}

        {/* Completed state */}
        {!isActive && (
          <div className="flex items-center justify-center py-3">
            <span className="text-xs text-muted-foreground">
              {run.status === "error" ? "This run failed." : "This run has completed."}
            </span>
          </div>
        )}
      </div>
    </div>
    </>
  )
}
