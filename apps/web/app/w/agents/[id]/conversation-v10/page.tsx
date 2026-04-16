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
  StopIcon,
  FilterIcon,
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
   Variant 10 — Tabbed Panel View (Cursor-inspired)

   Layout: Original spec (sidebar left, canvas right, panels top-right)
   Sidebar style: Standard date-grouped list with filter chips
                  (All / Active / Ended) and count badges
   Panels: Top-right with tab bar to switch Browser | Terminal
           instead of separate toggle buttons
   Messages: Clean inline style, tool calls as compact rows
   Animations: Tab indicator layoutId, panel tab switch, message fade
   ──────────────────────────────────────────────────────────── */

type SidebarFilter = "all" | "active" | "ended"
type PanelTab = "browser" | "terminal"

function ConversationItem({ conversation, isActive, onClick }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors cursor-pointer w-full ${
        isActive ? "bg-primary/8 border border-primary/15" : "border border-transparent hover:bg-muted/50"
      }`}
    >
      <span className={`h-2 w-2 rounded-full shrink-0 ${
        conversation.status === "active" ? "bg-green-500" : conversation.status === "error" ? "bg-destructive" : "bg-muted-foreground/15"
      }`} />
      <div className="flex-1 min-w-0">
        <span className="text-[13px] font-medium text-foreground truncate block">{conversation.title}</span>
        <span className="text-[11px] text-muted-foreground/50 truncate block mt-0.5">{conversation.preview}</span>
      </div>
      <span className="text-[10px] text-muted-foreground/30 font-mono shrink-0">{conversation.date}</span>
    </button>
  )
}

function ToolCallMessage({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

  return (
    <div className={`rounded-xl border overflow-hidden ${isRunning ? "border-primary/20 bg-primary/[0.02]" : "border-border"}`}>
      <button onClick={() => setExpanded(!expanded)} className="flex items-center gap-3 w-full px-4 py-3 text-left hover:bg-muted/30 transition-colors cursor-pointer">
        <HugeiconsIcon icon={Wrench01Icon} size={14} className={`shrink-0 ${isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"}`} />
        <span className="font-mono text-xs font-medium text-foreground flex-1 truncate">{message.toolName}</span>
        {isRunning ? <span className="font-mono text-[11px] text-primary">Running...</span> : <span className="font-mono text-[11px] text-muted-foreground">{message.toolDuration}</span>}
        <motion.div animate={{ rotate: expanded ? 180 : 0 }} transition={{ duration: 0.2 }}><HugeiconsIcon icon={ArrowDown01Icon} size={14} className="text-muted-foreground" /></motion.div>
      </button>
      <AnimatePresence>
        {expanded && (
          <motion.div initial={{ height: 0 }} animate={{ height: "auto" }} exit={{ height: 0 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} className="overflow-hidden">
            <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
              {message.toolParams && Object.keys(message.toolParams).length > 0 && (
                <div>
                  <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Arguments</span>
                  <div className="mt-1.5 rounded-lg bg-muted p-3">
                    {Object.entries(message.toolParams).map(([key, value]) => (
                      <div key={key} className="flex gap-2 font-mono text-[11px]"><span className="text-muted-foreground shrink-0">{key}:</span><span className="text-foreground break-all">{value}</span></div>
                    ))}
                  </div>
                </div>
              )}
              {message.toolResponse && (
                <div>
                  <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Response</span>
                  <div className="mt-1.5 rounded-lg bg-muted p-3 overflow-x-auto"><pre className="font-mono text-[11px] text-foreground whitespace-pre-wrap break-all leading-relaxed">{message.toolResponse}</pre></div>
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
        <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ delay: index * 0.02 }} className="flex justify-center py-2">
          <div className="rounded-lg bg-muted/50 px-4 py-2 max-w-lg"><p className="text-xs text-muted-foreground text-center leading-relaxed">{message.content}</p></div>
        </motion.div>
      )
    case "user":
      return (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.02, type: "spring", stiffness: 400, damping: 30 }}>
          <div className="rounded-xl bg-primary/10 p-4 ml-12">
            <div className="flex items-center justify-between mb-1.5">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-foreground/60">You</span>
              <span className="font-mono text-[10px] text-muted-foreground/50">{message.timestamp}</span>
            </div>
            <p className="text-sm text-foreground leading-relaxed">{message.content}</p>
          </div>
        </motion.div>
      )
    case "agent":
      return (
        <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: index * 0.02, type: "spring", stiffness: 400, damping: 30 }}>
          <div className="rounded-xl border border-border p-4 mr-12">
            <div className="flex items-center justify-between mb-1.5">
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-primary">Agent</span>
              <span className="font-mono text-[10px] text-muted-foreground/50">{message.timestamp}</span>
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

export default function ConversationV10() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [sidebarFilter, setSidebarFilter] = useState<SidebarFilter>("all")
  const [panelOpen, setPanelOpen] = useState(false)
  const [panelTab, setPanelTab] = useState<PanelTab>("browser")

  const allConversations = Object.values(sidebarConversations).flat()
  const activeCount = allConversations.filter((conversation) => conversation.status === "active").length
  const endedCount = allConversations.filter((conversation) => conversation.status !== "active").length

  const filters: { id: SidebarFilter; label: string; count?: number }[] = [
    { id: "all", label: "All" },
    { id: "active", label: "Active", count: activeCount },
    { id: "ended", label: "Ended", count: endedCount },
  ]

  const panelTabs: { id: PanelTab; label: string; icon: typeof BrowserIcon }[] = [
    { id: "browser", label: "Browser", icon: BrowserIcon },
    { id: "terminal", label: "Terminal", icon: CommandLineIcon },
  ]

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden bg-background">
      {/* ── Left: Sidebar with filters ────────────────────── */}
      <aside className="flex flex-col w-[300px] shrink-0 border-r border-border bg-sidebar h-full">
        <div className="flex items-center justify-between px-4 py-3 border-b border-border">
          <h2 className="text-sm font-semibold text-foreground">Conversations</h2>
          <div className="flex items-center gap-1">
            <button className="h-7 w-7 rounded-lg hover:bg-muted flex items-center justify-center transition-colors"><HugeiconsIcon icon={Search01Icon} size={14} className="text-muted-foreground" /></button>
            <button className="h-7 w-7 rounded-lg hover:bg-primary/10 flex items-center justify-center transition-colors"><HugeiconsIcon icon={Add01Icon} size={14} className="text-primary" /></button>
          </div>
        </div>

        {/* Filter chips */}
        <div className="flex items-center gap-1 px-3 py-2 border-b border-border">
          {filters.map((filter) => (
            <button
              key={filter.id}
              onClick={() => setSidebarFilter(filter.id)}
              className={`relative flex items-center gap-1.5 px-3 py-1 rounded-full text-[11px] font-medium transition-colors cursor-pointer ${
                sidebarFilter === filter.id ? "text-foreground" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {sidebarFilter === filter.id && (
                <motion.div layoutId="v10-filter" className="absolute inset-0 rounded-full bg-muted border border-border" style={{ zIndex: -1 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} />
              )}
              {filter.label}
              {filter.count !== undefined && (
                <span className="text-[9px] font-mono text-muted-foreground/50">{filter.count}</span>
              )}
            </button>
          ))}
        </div>

        <div className="flex-1 overflow-y-auto px-2 py-2">
          {Object.entries(sidebarConversations).map(([dateGroup, conversations]) => {
            const filtered = sidebarFilter === "all"
              ? conversations
              : sidebarFilter === "active"
              ? conversations.filter((conversation) => conversation.status === "active")
              : conversations.filter((conversation) => conversation.status !== "active")

            if (filtered.length === 0) return null

            return (
              <div key={dateGroup} className="mb-4">
                <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground/60 px-3 mb-1.5 block">{dateGroup}</span>
                <div className="flex flex-col gap-0.5">
                  {filtered.map((conversation) => (
                    <ConversationItem key={conversation.id} conversation={conversation} isActive={conversation.id === activeConversation} onClick={() => setActiveConversation(conversation.id)} />
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </aside>

      {/* ── Right: Canvas + tabbed panel ──────────────────── */}
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
            <div className="flex items-center gap-1.5 shrink-0">
              <Button
                variant={panelOpen ? "secondary" : "outline"}
                size="sm"
                onClick={() => setPanelOpen(!panelOpen)}
                className="h-7 text-xs"
              >
                <HugeiconsIcon icon={BrowserIcon} size={13} data-icon="inline-start" />
                {panelOpen ? "Hide panels" : "Show panels"}
              </Button>
              <Button variant="destructive" size="sm" className="h-7 text-xs">
                <HugeiconsIcon icon={StopIcon} size={12} data-icon="inline-start" />
                End
              </Button>
              <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors"><HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" /></button>
            </div>
          </div>

          <div className="flex-1 overflow-y-auto px-5 py-4">
            <div className="max-w-3xl mx-auto flex flex-col gap-3">
              {activeConversationMessages.map((message, index) => (
                <MessageBubble key={message.id} message={message} index={index} />
              ))}
            </div>
          </div>

          <div className="shrink-0 border-t border-border">
            <div className="flex items-center justify-between px-5 py-1 text-xs text-muted-foreground">
              <span className="flex items-center gap-3 tabular-nums font-mono">
                <span className="flex items-center gap-1"><HugeiconsIcon icon={ArrowDown01Icon} size={12} />12.4k</span>
                <span className="flex items-center gap-1"><HugeiconsIcon icon={ArrowUp01Icon} size={12} />4.8k</span>
              </span>
              <span className="font-mono text-[10px] text-muted-foreground/50">claude-sonnet-4-20250514</span>
            </div>
            <div className="px-4 pb-4">
              <div className="max-w-3xl mx-auto"><Textarea placeholder="Send a message..." className="min-h-[72px] max-h-40" /></div>
            </div>
          </div>
        </div>

        {/* Tabbed panel */}
        <AnimatePresence>
          {panelOpen && (
            <motion.div
              initial={{ width: 0, opacity: 0 }}
              animate={{ width: 420, opacity: 1 }}
              exit={{ width: 0, opacity: 0 }}
              transition={{ type: "spring", stiffness: 400, damping: 35 }}
              className="flex flex-col shrink-0 border-l border-border overflow-hidden"
            >
              {/* Panel tab bar */}
              <div className="flex items-center px-2 py-1.5 border-b border-border bg-muted/20 shrink-0">
                {panelTabs.map((tab) => (
                  <button
                    key={tab.id}
                    onClick={() => setPanelTab(tab.id)}
                    className={`relative flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-[11px] font-medium transition-colors cursor-pointer ${
                      panelTab === tab.id ? "text-foreground" : "text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    <HugeiconsIcon icon={tab.icon} size={12} />
                    {tab.label}
                    {panelTab === tab.id && (
                      <motion.div layoutId="v10-panel-tab" className="absolute inset-0 rounded-lg bg-background border border-border shadow-sm" style={{ zIndex: -1 }} transition={{ type: "spring", stiffness: 500, damping: 35 }} />
                    )}
                  </button>
                ))}
                <div className="flex-1" />
                <button onClick={() => setPanelOpen(false)} className="h-6 w-6 rounded-md hover:bg-muted flex items-center justify-center transition-colors">
                  <HugeiconsIcon icon={Cancel01Icon} size={12} className="text-muted-foreground" />
                </button>
              </div>

              {/* Panel content */}
              <AnimatePresence mode="wait">
                {panelTab === "browser" && (
                  <motion.div key="browser" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="flex flex-col flex-1">
                    <div className="flex items-center gap-2 px-3 py-2 bg-muted/10 border-b border-border shrink-0">
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
                  </motion.div>
                )}
                {panelTab === "terminal" && (
                  <motion.div key="terminal" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="flex flex-col flex-1">
                    <div className="flex items-center gap-2 px-3 py-2 bg-muted/10 border-b border-border shrink-0">
                      <HugeiconsIcon icon={CommandLineIcon} size={11} className="text-muted-foreground" /><span className="text-[10px] font-mono text-muted-foreground">Terminal</span>
                    </div>
                    <div className="flex-1 overflow-y-auto bg-foreground p-3"><pre className="font-mono text-[10px] leading-[1.7] text-background whitespace-pre-wrap">{terminalOutput}</pre></div>
                  </motion.div>
                )}
              </AnimatePresence>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  )
}
