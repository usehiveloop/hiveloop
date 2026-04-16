"use client"

import { useState } from "react"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Search01Icon,
  Add01Icon,
  Wrench01Icon,
  ArrowDown01Icon,
  BrowserIcon,
  CommandLineIcon,
  ArrowUp01Icon,
  Cancel01Icon,
  MoreHorizontalIcon,
  Robot01Icon,
  SentIcon,
  LinkSquare02Icon,
  ArrowLeft01Icon,
  ArrowRight01Icon,
  ArrowReloadHorizontalIcon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import {
  sidebarConversations,
  activeConversationMessages,
  terminalOutput,
  browserContent,
  type ConversationSummary,
  type MessageItem,
} from "../_data/conversation-mock"

/* ────────────────────────────────────────────────────────────
   Variant 7 — Devin-Inspired Compact Sidebar

   Layout: Original spec (sidebar left, canvas right, panels top-right)
   Sidebar style: Compact rows with inline status pills,
                  agent info header, search bar, session stats footer
   Messages: Avatar-based layout (like Slack/Discord threads)
   Animations: layoutId tab indicator, slide-in session drawer
   ──────────────────────────────────────────────────────────── */

function ConversationItem({ conversation, isActive, onClick }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
}) {
  const statusColors: Record<string, string> = {
    active: "bg-green-500 text-green-500",
    ended: "bg-muted-foreground/20 text-muted-foreground",
    error: "bg-destructive text-destructive",
  }
  const dotColor = statusColors[conversation.status]?.split(" ")[0] ?? "bg-muted-foreground/20"

  return (
    <button
      onClick={onClick}
      className={`relative flex items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-all cursor-pointer w-full ${
        isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
      }`}
    >
      {isActive && (
        <motion.div
          layoutId="v7-active-bg"
          className="absolute inset-0 rounded-lg bg-primary/8 border border-primary/12"
          style={{ zIndex: -1 }}
          transition={{ type: "spring", stiffness: 500, damping: 35 }}
        />
      )}
      <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${dotColor}`} />
      <div className="flex-1 min-w-0">
        <span className="text-[13px] font-medium truncate block">{conversation.title}</span>
        <span className="text-[10px] text-muted-foreground/40 font-mono">{conversation.date}</span>
      </div>
      {conversation.status === "active" && (
        <span className="inline-flex items-center rounded-full bg-green-500/10 px-1.5 py-0.5 text-[9px] font-mono font-medium text-green-600">
          Live
        </span>
      )}
      {conversation.status === "error" && (
        <span className="inline-flex items-center rounded-full bg-destructive/10 px-1.5 py-0.5 text-[9px] font-mono font-medium text-destructive">
          Error
        </span>
      )}
    </button>
  )
}

function ToolCallMessage({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

  return (
    <div className="ml-10">
      <div className={`rounded-xl border overflow-hidden ${isRunning ? "border-primary/20 bg-primary/[0.02]" : "border-border"}`}>
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-left hover:bg-muted/30 transition-colors cursor-pointer"
        >
          <div className={`h-5 w-5 rounded-lg flex items-center justify-center shrink-0 ${isRunning ? "bg-primary/10" : isSuccess ? "bg-green-500/10" : "bg-destructive/10"}`}>
            <HugeiconsIcon icon={Wrench01Icon} size={10} className={isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"} />
          </div>
          <span className="font-mono text-[11px] font-medium text-foreground flex-1 truncate">{message.toolName}</span>
          {isRunning ? (
            <span className="font-mono text-[10px] text-primary">running...</span>
          ) : (
            <span className="font-mono text-[10px] text-muted-foreground">{message.toolDuration}</span>
          )}
          <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.15 }}>
            <HugeiconsIcon icon={ArrowDown01Icon} size={11} className="text-muted-foreground/50" />
          </motion.div>
        </button>
        <AnimatePresence>
          {expanded && (
            <motion.div initial={{ height: 0 }} animate={{ height: "auto" }} exit={{ height: 0 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} className="overflow-hidden">
              <div className="border-t border-border px-3.5 py-2.5 flex flex-col gap-2">
                {message.toolParams && (
                  <div className="rounded-lg bg-muted p-2.5">
                    {Object.entries(message.toolParams).map(([key, value]) => (
                      <div key={key} className="flex gap-2 font-mono text-[10px]">
                        <span className="text-muted-foreground">{key}:</span>
                        <span className="text-foreground break-all">{value}</span>
                      </div>
                    ))}
                  </div>
                )}
                {message.toolResponse && (
                  <div className="rounded-lg bg-muted p-2.5 overflow-x-auto">
                    <pre className="font-mono text-[10px] text-foreground whitespace-pre-wrap break-all leading-relaxed">{message.toolResponse}</pre>
                  </div>
                )}
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}

function MessageBubble({ message }: { message: MessageItem }) {
  switch (message.role) {
    case "system":
      return (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="py-2">
          <div className="rounded-xl bg-muted/40 border border-border/50 px-4 py-2.5">
            <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/50 block mb-1">System</span>
            <p className="text-[12px] text-muted-foreground leading-relaxed">{message.content}</p>
          </div>
        </motion.div>
      )
    case "user":
      return (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ type: "spring", stiffness: 400, damping: 30 }} className="flex gap-3 py-3">
          <div className="h-7 w-7 rounded-full bg-primary/15 flex items-center justify-center shrink-0 mt-0.5">
            <span className="text-[10px] font-bold text-primary">Y</span>
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2 mb-1">
              <span className="text-[13px] font-semibold text-foreground">You</span>
              <span className="text-[10px] text-muted-foreground/40 font-mono">{message.timestamp}</span>
            </div>
            <p className="text-sm text-foreground leading-relaxed">{message.content}</p>
          </div>
        </motion.div>
      )
    case "agent":
      return (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ type: "spring", stiffness: 400, damping: 30 }} className="flex gap-3 py-3 bg-muted/20 -mx-5 px-5 rounded-xl">
          <div className="h-7 w-7 rounded-full bg-primary/20 flex items-center justify-center shrink-0 mt-0.5">
            <HugeiconsIcon icon={Robot01Icon} size={12} className="text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2 mb-1">
              <span className="text-[13px] font-semibold text-primary">Agent</span>
              <span className="text-[10px] text-muted-foreground/40 font-mono">{message.timestamp}</span>
            </div>
            <div className="text-sm text-foreground leading-relaxed whitespace-pre-wrap">{message.content}</div>
          </div>
        </motion.div>
      )
    case "tool_call":
      return <ToolCallMessage message={message} />
    default:
      return null
  }
}

function BrowserPanel() {
  return (
    <div className="flex flex-col h-full border-b border-border">
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-full bg-destructive/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-chart-1/40" />
          <span className="h-2.5 w-2.5 rounded-full bg-green-500/40" />
        </div>
        <div className="flex items-center gap-0.5 ml-1">
          <button className="h-5 w-5 rounded flex items-center justify-center hover:bg-muted"><HugeiconsIcon icon={ArrowLeft01Icon} size={10} className="text-muted-foreground/40" /></button>
          <button className="h-5 w-5 rounded flex items-center justify-center hover:bg-muted"><HugeiconsIcon icon={ArrowRight01Icon} size={10} className="text-muted-foreground/40" /></button>
          <button className="h-5 w-5 rounded flex items-center justify-center hover:bg-muted"><HugeiconsIcon icon={ArrowReloadHorizontalIcon} size={10} className="text-muted-foreground/40" /></button>
        </div>
        <div className="flex-1 flex items-center rounded-lg bg-background border border-border px-2.5 py-1">
          <HugeiconsIcon icon={LinkSquare02Icon} size={10} className="text-green-600 mr-1.5" />
          <span className="text-[10px] text-muted-foreground font-mono truncate">{browserContent.url}</span>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <div className="flex flex-col items-center justify-center gap-3 py-8">
          <div className="w-52 h-8 rounded-lg bg-muted animate-pulse" />
          <div className="w-40 h-7 rounded-lg bg-muted/60" />
          <div className="w-44 h-7 rounded-lg bg-muted/60" />
          <div className="w-32 h-8 rounded-lg bg-primary/15" />
        </div>
        <div className="border-t border-border pt-2 mt-2">
          <span className="font-mono text-[9px] font-medium uppercase tracking-[1px] text-muted-foreground mb-1.5 block">Console</span>
          {browserContent.consoleErrors.map((error, errorIndex) => (
            <div key={errorIndex} className={`rounded px-2 py-1 mb-1 font-mono text-[10px] ${
              error.level === "error" ? "bg-destructive/5 text-destructive" : "bg-chart-1/5 text-chart-1"
            }`}>{error.message}</div>
          ))}
        </div>
      </div>
    </div>
  )
}

function TerminalPanel() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <HugeiconsIcon icon={CommandLineIcon} size={11} className="text-muted-foreground" />
        <span className="text-[10px] font-mono text-muted-foreground">Terminal</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-foreground p-3">
        <pre className="font-mono text-[10px] leading-[1.7] text-background whitespace-pre-wrap">{terminalOutput}</pre>
      </div>
    </div>
  )
}

export default function ConversationV7() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [showBrowser, setShowBrowser] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const panelsOpen = showBrowser || showTerminal

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden bg-background">
      {/* ── Left: Sidebar ─────────────────────────────────── */}
      <aside className="flex flex-col w-[260px] shrink-0 border-r border-border bg-sidebar h-full">
        {/* Agent header */}
        <div className="flex items-center gap-2.5 px-3 py-3 border-b border-border">
          <div className="h-8 w-8 rounded-xl bg-primary/15 flex items-center justify-center">
            <HugeiconsIcon icon={Robot01Icon} size={15} className="text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <h2 className="text-[13px] font-semibold text-foreground truncate">Issue Triage Agent</h2>
            <span className="text-[10px] text-muted-foreground font-mono">claude-sonnet-4</span>
          </div>
          <button className="h-7 w-7 rounded-lg hover:bg-primary/10 flex items-center justify-center transition-colors">
            <HugeiconsIcon icon={Add01Icon} size={13} className="text-primary" />
          </button>
        </div>

        {/* Search */}
        <div className="px-3 py-2 border-b border-border">
          <div className="flex items-center gap-2 rounded-lg bg-muted/40 px-2.5 py-1.5">
            <HugeiconsIcon icon={Search01Icon} size={13} className="text-muted-foreground/40" />
            <span className="text-[11px] text-muted-foreground/40">Search conversations...</span>
          </div>
        </div>

        {/* List */}
        <div className="flex-1 overflow-y-auto px-2 py-2">
          {Object.entries(sidebarConversations).map(([dateGroup, conversations]) => (
            <div key={dateGroup} className="mb-3">
              <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 px-3 mb-1 block">{dateGroup}</span>
              <div className="flex flex-col">
                {conversations.map((conversation) => (
                  <ConversationItem
                    key={conversation.id}
                    conversation={conversation}
                    isActive={conversation.id === activeConversation}
                    onClick={() => setActiveConversation(conversation.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* Footer stats */}
        <div className="px-3 py-2.5 border-t border-border">
          <div className="flex items-center justify-between text-[10px] text-muted-foreground/40 font-mono">
            <span>11 conversations</span>
            <span>$0.42 today</span>
          </div>
        </div>
      </aside>

      {/* ── Right: Canvas + panels ────────────────────────── */}
      <div className="flex flex-1 min-w-0">
        <div className="flex flex-col flex-1 min-w-0">
          <div className="flex items-center justify-between px-5 py-3 border-b border-border shrink-0">
            <div className="flex items-center gap-3 min-w-0">
              <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-foreground truncate">Debug Safari login regression</h2>
                <p className="text-[11px] text-muted-foreground font-mono mt-0.5">12.4k in &middot; 4.8k out &middot; $0.04</p>
              </div>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              <Button variant={showBrowser ? "secondary" : "outline"} size="sm" onClick={() => setShowBrowser(!showBrowser)} className="h-7 text-xs">
                <HugeiconsIcon icon={BrowserIcon} size={13} data-icon="inline-start" />Browser
              </Button>
              <Button variant={showTerminal ? "secondary" : "outline"} size="sm" onClick={() => setShowTerminal(!showTerminal)} className="h-7 text-xs">
                <HugeiconsIcon icon={CommandLineIcon} size={13} data-icon="inline-start" />Terminal
              </Button>
              <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors ml-1">
                <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
              </button>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-5 py-4">
            <div className="max-w-3xl mx-auto flex flex-col gap-1">
              {activeConversationMessages.map((message) => (
                <MessageBubble key={message.id} message={message} />
              ))}
            </div>
          </div>

          <div className="shrink-0 border-t border-border p-3">
            <div className="max-w-3xl mx-auto relative">
              <Textarea placeholder="Message the agent..." className="min-h-[64px] max-h-32 pr-12" />
              <button className="absolute bottom-2 right-2 h-8 w-8 rounded-xl bg-primary text-primary-foreground hover:bg-primary/80 flex items-center justify-center transition-colors">
                <HugeiconsIcon icon={SentIcon} size={14} />
              </button>
            </div>
          </div>
        </div>

        <AnimatePresence>
          {panelsOpen && (
            <motion.div
              initial={{ width: 0, opacity: 0 }}
              animate={{ width: 400, opacity: 1 }}
              exit={{ width: 0, opacity: 0 }}
              transition={{ type: "spring", stiffness: 400, damping: 35 }}
              className="flex flex-col shrink-0 border-l border-border overflow-hidden"
            >
              {showBrowser && <div className={showTerminal ? "h-[60%]" : "h-full"}><BrowserPanel /></div>}
              {showTerminal && <div className={showBrowser ? "h-[40%]" : "h-full"}><TerminalPanel /></div>}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}
