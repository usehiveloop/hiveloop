"use client"

import { useState } from "react"
import { Badge } from "@/components/ui/badge"
import type { MockDecision } from "./mock-data"

interface DecisionsTabProps {
  decisions: MockDecision[]
}

function formatRelativeTime(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000)
  if (seconds < 60) return "just now"
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export function DecisionsTab({ decisions }: DecisionsTabProps) {
  const [expandedId, setExpandedId] = useState<string | null>(null)

  if (decisions.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20">
        <p className="text-sm text-muted-foreground">No routing decisions yet.</p>
        <p className="text-[12px] text-muted-foreground/70 mt-1">Decisions appear here as events flow through the router.</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      {/* Header */}
      <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
        <span className="w-16 shrink-0">Time</span>
        <span className="flex-1 min-w-0">Event</span>
        <span className="w-20 shrink-0">Mode</span>
        <span className="w-40 shrink-0">Agents</span>
        <span className="w-14 shrink-0 text-right">Latency</span>
      </div>

      {decisions.map((decision) => {
        const isExpanded = expandedId === decision.id
        return (
          <div key={decision.id}>
            <button
              type="button"
              onClick={() => setExpandedId(isExpanded ? null : decision.id)}
              className={`w-full hidden md:flex items-center gap-3 rounded-xl border px-4 py-2.5 transition-colors text-left cursor-pointer ${
                isExpanded ? "border-primary/20 bg-primary/5" : "border-border hover:border-primary/40"
              }`}
            >
              <span className="w-16 shrink-0 text-[11px] text-muted-foreground font-mono tabular-nums">
                {formatRelativeTime(decision.createdAt)}
              </span>
              <span className="flex-1 min-w-0 text-sm font-medium text-foreground truncate">
                {decision.eventType}
              </span>
              <span className="w-20 shrink-0">
                <Badge variant={decision.routingMode === "triage" ? "default" : "outline"} className="text-[10px]">
                  {decision.routingMode}
                </Badge>
              </span>
              <span className="w-40 shrink-0 text-[11px] text-muted-foreground truncate">
                {decision.selectedAgents.length === 0 ? (
                  <span className="text-muted-foreground/50">(none)</span>
                ) : (
                  decision.selectedAgents.join(", ")
                )}
              </span>
              <span className="w-14 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {decision.latencyMs}ms
              </span>
            </button>

            {/* Mobile row */}
            <button
              type="button"
              onClick={() => setExpandedId(isExpanded ? null : decision.id)}
              className={`w-full flex md:hidden flex-col gap-2 rounded-xl border px-4 py-3 transition-colors text-left cursor-pointer ${
                isExpanded ? "border-primary/20 bg-primary/5" : "border-border hover:border-primary/40"
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium text-foreground">{decision.eventType}</span>
                <Badge variant={decision.routingMode === "triage" ? "default" : "outline"} className="text-[10px]">
                  {decision.routingMode}
                </Badge>
              </div>
              <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                <span>{decision.selectedAgents.length === 0 ? "(none)" : decision.selectedAgents.join(", ")}</span>
                <span className="font-mono tabular-nums">{formatRelativeTime(decision.createdAt)} &middot; {decision.latencyMs}ms</span>
              </div>
            </button>

            {/* Expanded detail */}
            {isExpanded && (
              <div className="mx-4 mb-2 mt-1 rounded-lg bg-muted/50 p-4 text-[12px]">
                <div className="grid grid-cols-2 gap-x-8 gap-y-2">
                  <div>
                    <span className="text-muted-foreground">Event type</span>
                    <p className="font-mono text-foreground mt-0.5">{decision.eventType}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Routing mode</span>
                    <p className="text-foreground mt-0.5">{decision.routingMode}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Resource key</span>
                    <p className="font-mono text-foreground mt-0.5 break-all">{decision.resourceKey}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Latency</span>
                    <p className="font-mono text-foreground mt-0.5">{decision.latencyMs}ms</p>
                  </div>
                  <div className="col-span-2">
                    <span className="text-muted-foreground">Intent summary</span>
                    <p className="text-foreground mt-0.5">{decision.intentSummary}</p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Selected agents</span>
                    <p className="text-foreground mt-0.5">
                      {decision.selectedAgents.length === 0 ? "(none)" : decision.selectedAgents.join(", ")}
                    </p>
                  </div>
                  <div>
                    <span className="text-muted-foreground">Enrichment steps</span>
                    <p className="font-mono text-foreground mt-0.5">{decision.enrichmentSteps}</p>
                  </div>
                  {decision.routingMode === "triage" && (
                    <div>
                      <span className="text-muted-foreground">LLM turns</span>
                      <p className="font-mono text-foreground mt-0.5">{decision.turnCount}</p>
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )
      })}

      <p className="text-[11px] text-muted-foreground/50 text-center mt-4 mb-2">
        Showing {decisions.length} most recent decisions
      </p>
    </div>
  )
}
