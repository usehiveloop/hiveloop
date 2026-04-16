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
  ArrowLeft01Icon,
  ArrowRight01Icon,
  ArrowReloadHorizontalIcon,
  Message01Icon,
  Settings01Icon,
  Activity01Icon,
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
   Variant 8 — Icon Rail + Expandable List Sidebar

   Layout: Original spec (sidebar left, canvas right, panels top-right)
   Sidebar style: 52px icon rail + 220px expandable conversation list
                  Icon rail toggles list visibility
   Messages: Clean avatar-based with subtle alternating backgrounds
   Animations: Sidebar expand/collapse, layoutId tab pills
   ──────────────────────────────────────────────────────────── */

function ConversationItem({ conversation, isActive, onClick }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`flex flex-col gap-0.5 rounded-lg px-3 py-2 text-left transition-all cursor-pointer w-full ${
        isActive ? "bg-primary/8 text-foreground" : "text-muted-foreground hover:text-foreground hover:bg-muted/40"
      }`}
    >
      <div className="flex items-center gap-2">
        <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${
          conversation.status === "active" ? "bg-green-500 animate-pulse" : conversation.status === "error" ? "bg-destructive" : "bg-muted-foreground/20"
        }`} />
        <span className="text-[13px] font-medium truncate flex-1">{conversation.title}</span>
      </div>
      <div className="flex items-center justify-between pl-3.5 mt-0.5">
        <span className="text-[10px] text-muted-foreground/40 font-mono">{conversation.date}</span>
        <span className="text-[10px] text-muted-foreground/40 font-mono">{(conversation.tokenCount / 1000).toFixed(1)}k</span>
      </div>
    </button>
  )
}

function ToolCallMessage({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

  return (
    <div className={`rounded-xl border overflow-hidden ${isRunning ? "border-primary/20 bg-primary/[0.02]" : "border-border"}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-2.5 w-full px-4 py-2.5 text-left hover:bg-muted/30 transition-colors cursor-pointer"
      >
        <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${isRunning ? "bg-primary animate-pulse" : isSuccess ? "bg-green-500" : "bg-destructive"}`} />
        <span className="font-mono text-[11px] font-medium text-foreground flex-1 truncate">{message.toolName}</span>
        <span className="font-mono text-[10px] text-muted-foreground">{isRunning ? "running..." : message.toolDuration}</span>
        <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.15 }}>
          <HugeiconsIcon icon={ArrowDown01Icon} size={11} className="text-muted-foreground/50" />
        </motion.div>
      </button>
      <AnimatePresence>
        {expanded && (
          <motion.div initial={{ height: 0 }} animate={{ height: "auto" }} exit={{ height: 0 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} className="overflow-hidden">
            <div className="border-t border-border px-4 py-3 flex flex-col gap-2">
              {message.toolParams && (
                <div className="rounded-lg bg-muted p-2.5">
                  {Object.entries(message.toolParams).map(([key, value]) => (
                    <div key={key} className="flex gap-2 font-mono text-[10px]"><span className="text-muted-foreground">{key}:</span><span className="text-foreground break-all">{value}</span></div>
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
  )
}

function MessageBubble({ message, index }: { message: MessageItem; index: number }) {
  switch (message.role) {
    case "system":
      return (
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: index * 0.02 }} className="px-4 py-2 border-b border-dashed border-border/40">
          <p className="text-[11px] text-muted-foreground/50 leading-relaxed">{message.content}</p>
        </motion.div>
      )
    case "user":
      return (
        <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.02, type: "spring", stiffness: 500, damping: 35 }} className="px-4 py-3">
          <div className="flex items-start gap-3">
            <div className="h-6 w-6 rounded-full bg-primary/15 flex items-center justify-center shrink-0 mt-0.5">
              <span className="text-[10px] font-bold text-primary">Y</span>
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-[12px] font-semibold text-foreground">You</span>
                <span className="text-[10px] text-muted-foreground/40 font-mono">{message.timestamp}</span>
              </div>
              <p className="text-[13px] text-foreground leading-relaxed">{message.content}</p>
            </div>
          </div>
        </motion.div>
      )
    case "agent":
      return (
        <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.02, type: "spring", stiffness: 500, damping: 35 }} className="px-4 py-3 bg-muted/15">
          <div className="flex items-start gap-3">
            <div className="h-6 w-6 rounded-full bg-primary/20 flex items-center justify-center shrink-0 mt-0.5">
              <HugeiconsIcon icon={Robot01Icon} size={11} className="text-primary" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 mb-1">
                <span className="text-[12px] font-semibold text-primary">Agent</span>
                <span className="text-[10px] text-muted-foreground/40 font-mono">{message.timestamp}</span>
              </div>
              <div className="text-[13px] text-foreground leading-relaxed whitespace-pre-wrap">{message.content}</div>
            </div>
          </div>
        </motion.div>
      )
    case "tool_call":
      return (
        <motion.div initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.02 }} className="px-4 py-1.5">
          <div className="ml-9"><ToolCallMessage message={message} /></div>
        </motion.div>
      )
    default:
      return null
  }
}

function BrowserPanel() {
  return (
    <div className="flex flex-col h-full border-b border-border">
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/20 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-full bg-destructive/40" /><span className="h-2.5 w-2.5 rounded-full bg-chart-1/40" /><span className="h-2.5 w-2.5 rounded-full bg-green-500/40" /></div>
        <div className="flex-1 flex items-center rounded-lg bg-background border border-border px-2.5 py-1">
          <HugeiconsIcon icon={LinkSquare02Icon} size={10} className="text-green-600 mr-1.5" />
          <span className="text-[10px] text-muted-foreground font-mono truncate">{browserContent.url}</span>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <div className="flex flex-col items-center justify-center gap-3 py-8">
          <div className="w-52 h-8 rounded-lg bg-muted animate-pulse" /><div className="w-40 h-7 rounded-lg bg-muted/60" /><div className="w-44 h-7 rounded-lg bg-muted/60" /><div className="w-32 h-8 rounded-lg bg-primary/15" />
        </div>
        <div className="border-t border-border pt-2 mt-2">
          <span className="font-mono text-[9px] font-medium uppercase tracking-[1px] text-muted-foreground mb-1.5 block">Console</span>
          {browserContent.consoleErrors.map((error, errorIndex) => (
            <div key={errorIndex} className={`rounded px-2 py-1 mb-1 font-mono text-[10px] ${error.level === "error" ? "bg-destructive/5 text-destructive" : "bg-chart-1/5 text-chart-1"}`}>{error.message}</div>
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
        <HugeiconsIcon icon={CommandLineIcon} size={11} className="text-muted-foreground" /><span className="text-[10px] font-mono text-muted-foreground">Terminal</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-foreground p-3"><pre className="font-mono text-[10px] leading-[1.7] text-background whitespace-pre-wrap">{terminalOutput}</pre></div>
    </div>
  )
}

export default function ConversationV8() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [sidebarExpanded, setSidebarExpanded] = useState(true)
  const [showBrowser, setShowBrowser] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const panelsOpen = showBrowser || showTerminal

  const iconRailItems = [
    { icon: Message01Icon, label: "Conversations", active: true, onClick: () => setSidebarExpanded(!sidebarExpanded) },
    { icon: Activity01Icon, label: "Logs", active: false, onClick: () => {} },
    { icon: Settings01Icon, label: "Settings", active: false, onClick: () => {} },
  ]

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden bg-background">
      {/* Icon rail */}
      <div className="flex flex-col items-center w-[52px] shrink-0 border-r border-border bg-sidebar py-2 gap-1">
        {iconRailItems.map((item) => (
          <button key={item.label} onClick={item.onClick} className={`flex items-center justify-center h-9 w-9 rounded-xl transition-colors cursor-pointer ${item.active ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground"}`} title={item.label}>
            <HugeiconsIcon icon={item.icon} size={16} />
          </button>
        ))}
        <div className="flex-1" />
        <div className="h-8 w-8 rounded-xl bg-primary/15 flex items-center justify-center">
          <HugeiconsIcon icon={Robot01Icon} size={14} className="text-primary" />
        </div>
      </div>

      {/* Expandable conversation list */}
      <AnimatePresence>
        {sidebarExpanded && (
          <motion.aside
            initial={{ width: 0, opacity: 0 }}
            animate={{ width: 220, opacity: 1 }}
            exit={{ width: 0, opacity: 0 }}
            transition={{ type: "spring", stiffness: 400, damping: 35 }}
            className="flex flex-col shrink-0 border-r border-border bg-sidebar overflow-hidden"
          >
            <div className="flex items-center justify-between px-3 py-3 border-b border-border shrink-0">
              <span className="text-[13px] font-semibold text-foreground">Conversations</span>
              <div className="flex items-center gap-0.5">
                <button className="h-6 w-6 rounded-md hover:bg-muted flex items-center justify-center transition-colors"><HugeiconsIcon icon={Search01Icon} size={13} className="text-muted-foreground" /></button>
                <button className="h-6 w-6 rounded-md hover:bg-primary/10 flex items-center justify-center transition-colors"><HugeiconsIcon icon={Add01Icon} size={13} className="text-primary" /></button>
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-1.5 py-2">
              {Object.entries(sidebarConversations).map(([dateGroup, conversations]) => (
                <div key={dateGroup} className="mb-3">
                  <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/40 px-3 mb-1 block">{dateGroup}</span>
                  {conversations.map((conversation) => (
                    <ConversationItem key={conversation.id} conversation={conversation} isActive={conversation.id === activeConversation} onClick={() => setActiveConversation(conversation.id)} />
                  ))}
                </div>
              ))}
            </div>
          </motion.aside>
        )}
      </AnimatePresence>

      {/* Canvas + panels */}
      <div className="flex flex-1 min-w-0">
        <div className="flex flex-col flex-1 min-w-0">
          <div className="flex items-center justify-between px-5 py-3 border-b border-border shrink-0">
            <div className="flex items-center gap-3 min-w-0">
              <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-foreground truncate">Debug Safari login regression</h2>
                <p className="text-[11px] text-muted-foreground font-mono mt-0.5">conv_001 &middot; 10:42 AM</p>
              </div>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              <Button variant={showBrowser ? "secondary" : "outline"} size="sm" onClick={() => setShowBrowser(!showBrowser)} className="h-7 text-xs"><HugeiconsIcon icon={BrowserIcon} size={13} data-icon="inline-start" />Browser</Button>
              <Button variant={showTerminal ? "secondary" : "outline"} size="sm" onClick={() => setShowTerminal(!showTerminal)} className="h-7 text-xs"><HugeiconsIcon icon={CommandLineIcon} size={13} data-icon="inline-start" />Terminal</Button>
              <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors ml-1"><HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" /></button>
            </div>
          </div>
          <div className="flex-1 overflow-y-auto">
            {activeConversationMessages.map((message, index) => (
              <MessageBubble key={message.id} message={message} index={index} />
            ))}
          </div>
          <div className="shrink-0 border-t border-border p-3">
            <div className="flex items-center gap-2 mb-1.5 px-1">
              <span className="font-mono text-[10px] text-muted-foreground/30 tabular-nums flex items-center gap-2">
                <span className="flex items-center gap-0.5"><HugeiconsIcon icon={ArrowDown01Icon} size={10} />12.4k</span>
                <span className="flex items-center gap-0.5"><HugeiconsIcon icon={ArrowUp01Icon} size={10} />4.8k</span>
              </span>
              <span className="font-mono text-[9px] text-muted-foreground/20">claude-sonnet-4-20250514</span>
            </div>
            <div className="relative">
              <Textarea placeholder="Message..." className="min-h-[52px] max-h-28 pr-11" />
              <button className="absolute bottom-2 right-2 h-7 w-7 rounded-lg bg-primary text-primary-foreground hover:bg-primary/80 flex items-center justify-center transition-colors"><HugeiconsIcon icon={SentIcon} size={12} /></button>
            </div>
          </div>
        </div>

        <AnimatePresence>
          {panelsOpen && (
            <motion.div initial={{ width: 0, opacity: 0 }} animate={{ width: 400, opacity: 1 }} exit={{ width: 0, opacity: 0 }} transition={{ type: "spring", stiffness: 400, damping: 35 }} className="flex flex-col shrink-0 border-l border-border overflow-hidden">
              {showBrowser && <div className={showTerminal ? "h-[60%]" : "h-full"}><BrowserPanel /></div>}
              {showTerminal && <div className={showBrowser ? "h-[40%]" : "h-full"}><TerminalPanel /></div>}
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}
