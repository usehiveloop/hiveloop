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
  MoreHorizontalIcon,
  Robot01Icon,
  SentIcon,
  LinkSquare02Icon,
  SparklesIcon,
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
   Variant 9 — Linear-Clean Sidebar with Pill Indicators

   Layout: Original spec (sidebar left, canvas right, panels top-right)
   Sidebar style: Ultra-refined with sliding pill active state (layoutId),
                  horizontal rule date dividers, generous spacing
   Messages: Avatar-based with gradient rings, large text, wide leading
   Animations: Spring pill slide, staggered list items, message stagger
   ──────────────────────────────────────────────────────────── */

function ConversationItem({ conversation, isActive, onClick, index }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
  index: number
}) {
  return (
    <motion.button
      initial={{ opacity: 0, x: -6 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ delay: index * 0.03, type: "spring", stiffness: 400, damping: 30 }}
      onClick={onClick}
      className={`relative flex items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-all cursor-pointer w-full ${
        isActive ? "text-foreground" : "text-muted-foreground hover:text-foreground"
      }`}
    >
      {isActive && (
        <motion.div
          layoutId="v9-active-pill"
          className="absolute inset-0 rounded-xl bg-primary/8 border border-primary/12"
          style={{ zIndex: -1 }}
          transition={{ type: "spring", stiffness: 500, damping: 35 }}
        />
      )}
      <span className="relative flex items-center gap-3 flex-1 min-w-0">
        <span className={`h-2 w-2 rounded-full shrink-0 ${
          conversation.status === "active" ? "bg-green-500" : conversation.status === "error" ? "bg-destructive" : "bg-muted-foreground/15"
        }`} />
        <span className="flex-1 min-w-0">
          <span className="text-[13px] font-medium truncate block leading-tight">{conversation.title}</span>
          <span className="text-[11px] text-muted-foreground/40 font-mono mt-0.5 block">{conversation.date}</span>
        </span>
      </span>
    </motion.button>
  )
}

function ToolCallMessage({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

  return (
    <div className="ml-10">
      <div className={`rounded-xl border overflow-hidden ${isRunning ? "border-primary/20" : "border-border/50"}`}>
        <button onClick={() => setExpanded(!expanded)} className="flex items-center gap-2.5 w-full px-3.5 py-2.5 text-left hover:bg-muted/20 transition-colors cursor-pointer">
          <div className={`h-5 w-5 rounded-lg flex items-center justify-center ${isRunning ? "bg-primary/10" : isSuccess ? "bg-green-500/8" : "bg-destructive/8"}`}>
            <HugeiconsIcon icon={Wrench01Icon} size={10} className={isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"} />
          </div>
          <span className="font-mono text-[11px] font-medium text-foreground/70 flex-1 truncate">{message.toolName}</span>
          {isRunning ? (
            <div className="flex items-center gap-1">
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_infinite]" />
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.15s_infinite]" />
              <span className="h-1 w-1 rounded-full bg-primary animate-[bounce_1s_ease-in-out_0.3s_infinite]" />
            </div>
          ) : (
            <span className="font-mono text-[10px] text-muted-foreground/30">{message.toolDuration}</span>
          )}
          <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.15 }}>
            <HugeiconsIcon icon={ArrowDown01Icon} size={10} className="text-muted-foreground/25" />
          </motion.div>
        </button>
        <AnimatePresence>
          {expanded && (
            <motion.div initial={{ height: 0 }} animate={{ height: "auto" }} exit={{ height: 0 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} className="overflow-hidden">
              <div className="border-t border-border/30 px-3.5 py-2.5 flex flex-col gap-2">
                {message.toolParams && (
                  <div className="rounded-lg bg-muted/40 p-2.5">
                    {Object.entries(message.toolParams).map(([key, value]) => (
                      <div key={key} className="font-mono text-[10px]"><span className="text-muted-foreground/50">{key}: </span><span className="text-foreground/70">{value}</span></div>
                    ))}
                  </div>
                )}
                {message.toolResponse && (
                  <div className="rounded-lg bg-muted/40 p-2.5 overflow-x-auto">
                    <pre className="font-mono text-[10px] text-foreground/70 whitespace-pre-wrap break-all leading-relaxed">{message.toolResponse}</pre>
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

function MessageBubble({ message, index }: { message: MessageItem; index: number }) {
  switch (message.role) {
    case "system":
      return (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: index * 0.03 }} className="flex justify-center py-4">
          <div className="flex items-center gap-2 rounded-full bg-muted/40 px-5 py-2">
            <HugeiconsIcon icon={SparklesIcon} size={11} className="text-muted-foreground/40" />
            <p className="text-[11px] text-muted-foreground/50">{message.content.slice(0, 80)}...</p>
          </div>
        </motion.div>
      )
    case "user":
      return (
        <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.03, type: "spring", stiffness: 400, damping: 28 }} className="flex gap-3 py-3">
          <div className="h-7 w-7 rounded-full bg-primary/15 border border-primary/10 flex items-center justify-center shrink-0 mt-0.5">
            <span className="text-[11px] font-bold text-primary">Y</span>
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2 mb-1.5">
              <span className="text-[13px] font-semibold text-foreground">You</span>
              <span className="text-[10px] text-muted-foreground/30 font-mono">{message.timestamp}</span>
            </div>
            <p className="text-[14px] text-foreground leading-[1.65]">{message.content}</p>
          </div>
        </motion.div>
      )
    case "agent":
      return (
        <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.03, type: "spring", stiffness: 400, damping: 28 }} className="flex gap-3 py-3">
          <div className="h-7 w-7 rounded-full bg-primary/20 border border-primary/10 flex items-center justify-center shrink-0 mt-0.5">
            <HugeiconsIcon icon={Robot01Icon} size={13} className="text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2 mb-1.5">
              <span className="text-[13px] font-semibold text-primary">Agent</span>
              <span className="text-[10px] text-muted-foreground/30 font-mono">{message.timestamp}</span>
            </div>
            <div className="text-[14px] text-foreground leading-[1.65] whitespace-pre-wrap">{message.content}</div>
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
      <div className="flex items-center gap-2.5 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-full bg-destructive/40" /><span className="h-2.5 w-2.5 rounded-full bg-chart-1/40" /><span className="h-2.5 w-2.5 rounded-full bg-green-500/40" /></div>
        <div className="flex-1 flex items-center rounded-xl bg-background border border-border px-3 py-1.5">
          <HugeiconsIcon icon={LinkSquare02Icon} size={11} className="text-green-600 mr-2" />
          <span className="text-[11px] text-muted-foreground font-mono truncate">{browserContent.url}</span>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <div className="flex flex-col items-center justify-center gap-4 py-10">
          <div className="w-56 h-9 rounded-xl bg-muted animate-pulse" /><div className="w-44 h-7 rounded-lg bg-muted/60" /><div className="w-48 h-7 rounded-lg bg-muted/60" /><div className="w-36 h-8 rounded-xl bg-primary/15 border border-primary/15" />
        </div>
        <div className="border-t border-border pt-3 mt-3">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground mb-2 block">Console</span>
          {browserContent.consoleErrors.map((error, errorIndex) => (
            <div key={errorIndex} className={`rounded-lg px-3 py-1.5 mb-1.5 font-mono text-[10px] leading-relaxed ${error.level === "error" ? "bg-destructive/5 text-destructive" : "bg-chart-1/5 text-chart-1"}`}>
              <span className="text-muted-foreground mr-2">{error.timestamp}</span>{error.message}
            </div>
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
        <HugeiconsIcon icon={CommandLineIcon} size={12} className="text-muted-foreground" /><span className="text-[11px] font-mono text-muted-foreground">Terminal</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-foreground p-4"><pre className="font-mono text-[11px] leading-[1.7] text-background whitespace-pre-wrap">{terminalOutput}</pre></div>
    </div>
  )
}

export default function ConversationV9() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [showBrowser, setShowBrowser] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const panelsOpen = showBrowser || showTerminal

  let conversationIndex = 0

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden bg-background">
      {/* ── Left: Sidebar ─────────────────────────────────── */}
      <aside className="flex flex-col w-[300px] shrink-0 border-r border-border bg-sidebar h-full">
        <div className="flex items-center justify-between px-4 py-3.5 border-b border-border">
          <div className="flex items-center gap-2.5">
            <div className="h-7 w-7 rounded-xl bg-primary/15 flex items-center justify-center">
              <HugeiconsIcon icon={Robot01Icon} size={14} className="text-primary" />
            </div>
            <div>
              <h2 className="text-[13px] font-semibold text-foreground leading-tight">Issue Triage</h2>
              <span className="text-[10px] text-muted-foreground/40 font-mono">claude-sonnet-4</span>
            </div>
          </div>
          <button className="h-7 w-7 rounded-lg hover:bg-primary/8 flex items-center justify-center transition-colors">
            <HugeiconsIcon icon={Add01Icon} size={14} className="text-primary/60" />
          </button>
        </div>

        <div className="px-3 py-2">
          <div className="flex items-center gap-2 rounded-xl bg-muted/30 px-3 py-2 cursor-text hover:bg-muted/50 transition-colors">
            <HugeiconsIcon icon={Search01Icon} size={13} className="text-muted-foreground/30" />
            <span className="text-[12px] text-muted-foreground/30">Search conversations...</span>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-2 py-1">
          {Object.entries(sidebarConversations).map(([group, items]) => (
            <div key={group} className="mb-3">
              <div className="flex items-center gap-2 px-3 py-1.5">
                <span className="text-[10px] font-semibold uppercase tracking-[1.5px] text-muted-foreground/25">{group}</span>
                <div className="flex-1 h-px bg-border/40" />
              </div>
              {items.map((conversation) => {
                const currentIndex = conversationIndex++
                return (
                  <ConversationItem key={conversation.id} conversation={conversation} isActive={conversation.id === activeConversation} onClick={() => setActiveConversation(conversation.id)} index={currentIndex} />
                )
              })}
            </div>
          ))}
        </div>
      </aside>

      {/* ── Right: Canvas + panels ────────────────────────── */}
      <div className="flex flex-1 min-w-0">
        {/* Chat area — fixed max width 728px */}
        <div className="flex flex-col w-full max-w-[728px] shrink-0 border-r border-border">
          <div className="flex items-center justify-between px-8 py-3.5 border-b border-border shrink-0">
            <div className="flex items-center gap-3">
              <motion.div animate={{ scale: [1, 1.15, 1] }} transition={{ repeat: Infinity, duration: 2.5, ease: "easeInOut" }} className="h-2 w-2 rounded-full bg-green-500" />
              <h2 className="text-[15px] font-semibold text-foreground">Debug Safari login regression</h2>
            </div>
            <div className="flex items-center gap-1.5 shrink-0">
              <Button variant={showBrowser ? "secondary" : "outline"} size="sm" onClick={() => setShowBrowser(!showBrowser)} className="h-7 text-xs"><HugeiconsIcon icon={BrowserIcon} size={13} data-icon="inline-start" />Browser</Button>
              <Button variant={showTerminal ? "secondary" : "outline"} size="sm" onClick={() => setShowTerminal(!showTerminal)} className="h-7 text-xs"><HugeiconsIcon icon={CommandLineIcon} size={13} data-icon="inline-start" />Terminal</Button>
              <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors ml-1"><HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" /></button>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-8 py-6">
            <div className="flex flex-col gap-1">
              {activeConversationMessages.map((message, index) => (
                <MessageBubble key={message.id} message={message} index={index} />
              ))}
            </div>
          </div>

          <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2, type: "spring", stiffness: 400, damping: 30 }} className="shrink-0 px-8 pb-6">
            <div className="rounded-2xl border border-border bg-muted/10 p-1.5 transition-colors focus-within:border-primary/30">
              <Textarea placeholder="Ask the agent anything..." className="border-0 bg-transparent min-h-[60px] max-h-32 focus-visible:ring-0 focus-visible:border-transparent text-[14px]" />
              <div className="flex items-center justify-between px-2 pt-1">
                <span className="font-mono text-[9px] text-muted-foreground/20">claude-sonnet-4-20250514 &middot; 12.4k in / 4.8k out</span>
                <button className="h-7 w-7 rounded-lg bg-primary text-primary-foreground hover:bg-primary/80 flex items-center justify-center transition-colors"><HugeiconsIcon icon={SentIcon} size={13} /></button>
              </div>
            </div>
          </motion.div>
        </div>

        {/* Panels — fill remaining space */}
        <AnimatePresence>
          {panelsOpen && (
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ type: "spring", stiffness: 400, damping: 35 }}
              className="flex flex-col flex-1 min-w-0 overflow-hidden"
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
