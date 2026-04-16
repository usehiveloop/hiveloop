"use client"

import { useState } from "react"
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
  Message01Icon,
  GridViewIcon,
  StopIcon,
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
   Variant 4 — IDE-Style Tabbed Layout

   Left:   240px sidebar
   Main:   Tab bar at top: Chat | Browser | Terminal | Split
           Split view = conversation top + terminal bottom
           Very IDE-like, productive feel
   ──────────────────────────────────────────────────────────── */

type ActiveTab = "chat" | "browser" | "terminal" | "split"

function ConversationListItem({ conversation, isActive, onClick }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors cursor-pointer w-full ${
        isActive ? "bg-primary/10 text-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground"
      }`}
    >
      {conversation.status === "active" ? (
        <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse shrink-0" />
      ) : conversation.status === "error" ? (
        <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" />
      ) : (
        <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/20 shrink-0" />
      )}
      <div className="flex-1 min-w-0">
        <span className="text-[13px] font-medium truncate block">{conversation.title}</span>
        <span className="text-[10px] font-mono text-muted-foreground/50">{conversation.date}</span>
      </div>
    </button>
  )
}

function ToolCallInline({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

  return (
    <div className={`rounded-xl border overflow-hidden ${isRunning ? "border-primary/30 bg-primary/[0.02]" : "border-border"}`}>
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-3 w-full px-4 py-2.5 text-left hover:bg-muted/50 transition-colors cursor-pointer"
      >
        <HugeiconsIcon
          icon={Wrench01Icon}
          size={13}
          className={`shrink-0 ${isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"}`}
        />
        <span className="font-mono text-[11px] font-medium text-foreground flex-1 truncate">{message.toolName}</span>
        {isRunning ? (
          <span className="font-mono text-[11px] text-primary shrink-0">Running...</span>
        ) : (
          <span className="font-mono text-[11px] text-muted-foreground shrink-0">{message.toolDuration}</span>
        )}
        <HugeiconsIcon icon={ArrowDown01Icon} size={12} className={`text-muted-foreground transition-transform ${expanded ? "rotate-180" : ""}`} />
      </button>
      {expanded && (
        <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
          {message.toolParams && Object.keys(message.toolParams).length > 0 && (
            <div>
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Arguments</span>
              <div className="mt-1.5 rounded-lg bg-muted p-3">
                {Object.entries(message.toolParams).map(([key, value]) => (
                  <div key={key} className="flex gap-2 font-mono text-[11px]">
                    <span className="text-muted-foreground">{key}:</span>
                    <span className="text-foreground break-all">{value}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
          {message.toolResponse && (
            <div>
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Response</span>
              <div className="mt-1.5 rounded-lg bg-muted p-3 overflow-x-auto">
                <pre className="font-mono text-[11px] text-foreground whitespace-pre-wrap break-all leading-relaxed">{message.toolResponse}</pre>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function MessageBubble({ message }: { message: MessageItem }) {
  switch (message.role) {
    case "system":
      return (
        <div className="flex justify-center py-2">
          <div className="rounded-lg bg-muted/50 px-4 py-2 max-w-lg">
            <p className="text-xs text-muted-foreground text-center leading-relaxed">{message.content}</p>
          </div>
        </div>
      )
    case "user":
      return (
        <div className="rounded-xl bg-primary/10 p-4 ml-12">
          <div className="flex items-center justify-between mb-1.5">
            <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-foreground/60">You</span>
            <span className="font-mono text-[10px] text-muted-foreground/50">{message.timestamp}</span>
          </div>
          <p className="text-sm text-foreground leading-relaxed">{message.content}</p>
        </div>
      )
    case "agent":
      return (
        <div className="rounded-xl border border-border p-4 mr-12">
          <div className="flex items-center justify-between mb-1.5">
            <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-primary">Agent</span>
            <span className="font-mono text-[10px] text-muted-foreground/50">{message.timestamp}</span>
          </div>
          <div className="text-sm text-foreground leading-relaxed whitespace-pre-wrap">{message.content}</div>
        </div>
      )
    case "tool_call":
      return <ToolCallInline message={message} />
    default:
      return null
  }
}

function ChatView() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 overflow-y-auto px-5 py-4">
        <div className="max-w-3xl mx-auto flex flex-col gap-3">
          {activeConversationMessages.map((message) => (
            <MessageBubble key={message.id} message={message} />
          ))}
        </div>
      </div>
      <div className="shrink-0 border-t border-border">
        <div className="flex items-center justify-between px-5 py-1 text-xs text-muted-foreground">
          <span className="flex items-center gap-3 tabular-nums font-mono">
            <span className="flex items-center gap-1">
              <HugeiconsIcon icon={ArrowDown01Icon} size={12} />
              12.4k
            </span>
            <span className="flex items-center gap-1">
              <HugeiconsIcon icon={ArrowUp01Icon} size={12} />
              4.8k
            </span>
          </span>
          <span className="font-mono text-[10px] text-muted-foreground/50">claude-sonnet-4-20250514</span>
        </div>
        <div className="px-4 pb-4">
          <div className="max-w-3xl mx-auto">
            <Textarea placeholder="Send a message..." className="min-h-[72px] max-h-40" />
          </div>
        </div>
      </div>
    </div>
  )
}

function BrowserView() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-2.5 bg-muted/20 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-full bg-destructive/60" />
          <span className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
          <span className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
        </div>
        <div className="flex-1 flex items-center rounded-lg bg-background border border-border px-3 py-1">
          <span className="text-[11px] text-green-600 font-mono">https://</span>
          <span className="text-[11px] text-foreground font-mono">{browserContent.url.replace("https://", "")}</span>
        </div>
      </div>
      <div className="flex flex-1 overflow-hidden">
        {/* Viewport */}
        <div className="flex-1 overflow-y-auto p-6">
          <div className="flex flex-col items-center justify-center gap-4 py-12">
            <div className="w-72 h-12 rounded-xl bg-muted animate-pulse" />
            <div className="w-56 h-9 rounded-lg bg-muted" />
            <div className="w-60 h-9 rounded-lg bg-muted" />
            <div className="w-44 h-10 rounded-xl bg-primary/20" />
          </div>
        </div>
        {/* Console panel */}
        <div className="w-[380px] shrink-0 border-l border-border overflow-y-auto p-4 bg-muted/10">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Console</span>
          <div className="mt-3 flex flex-col gap-1.5">
            {browserContent.consoleErrors.map((error, index) => (
              <div key={index} className={`rounded-lg px-3 py-2 font-mono text-[11px] leading-relaxed ${
                error.level === "error" ? "bg-destructive/5 text-destructive" : "bg-yellow-500/5 text-yellow-600"
              }`}>
                <span className="text-muted-foreground mr-2">{error.timestamp}</span>
                {error.message}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function TerminalView() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-4 py-2.5 bg-muted/20 border-b border-border shrink-0">
        <HugeiconsIcon icon={CommandLineIcon} size={13} className="text-muted-foreground" />
        <span className="text-[11px] font-mono text-muted-foreground">bash &mdash; acme/webapp</span>
        <div className="flex-1" />
        <span className="text-[10px] font-mono text-muted-foreground/50">192 lines</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-[oklch(0.16_0.01_250)] p-4">
        <pre className="font-mono text-[11px] leading-relaxed text-green-400 whitespace-pre-wrap">{terminalOutput}</pre>
      </div>
    </div>
  )
}

function SplitView() {
  return (
    <div className="flex flex-col h-full">
      {/* Top: Chat (60%) */}
      <div className="flex flex-col" style={{ height: "60%" }}>
        <div className="flex-1 overflow-y-auto px-5 py-4">
          <div className="max-w-3xl mx-auto flex flex-col gap-3">
            {activeConversationMessages.map((message) => (
              <MessageBubble key={message.id} message={message} />
            ))}
          </div>
        </div>
        <div className="shrink-0 px-4 pb-3">
          <div className="max-w-3xl mx-auto">
            <Textarea placeholder="Send a message..." className="min-h-[56px] max-h-28" />
          </div>
        </div>
      </div>
      {/* Divider */}
      <div className="h-px bg-border relative">
        <div className="absolute inset-x-0 -top-1 -bottom-1 flex items-center justify-center cursor-row-resize">
          <div className="w-8 h-1 rounded-full bg-border" />
        </div>
      </div>
      {/* Bottom: Terminal (40%) */}
      <div className="flex flex-col" style={{ height: "40%" }}>
        <div className="flex items-center gap-2 px-4 py-2 bg-muted/20 border-b border-border shrink-0">
          <HugeiconsIcon icon={CommandLineIcon} size={12} className="text-muted-foreground" />
          <span className="text-[10px] font-mono text-muted-foreground">Terminal</span>
        </div>
        <div className="flex-1 overflow-y-auto bg-[oklch(0.16_0.01_250)] p-4">
          <pre className="font-mono text-[11px] leading-relaxed text-green-400 whitespace-pre-wrap">{terminalOutput}</pre>
        </div>
      </div>
    </div>
  )
}

export default function ConversationV4() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [activeTab, setActiveTab] = useState<ActiveTab>("split")

  const tabs: { id: ActiveTab; label: string; icon: typeof Message01Icon }[] = [
    { id: "chat", label: "Chat", icon: Message01Icon },
    { id: "browser", label: "Browser", icon: BrowserIcon },
    { id: "terminal", label: "Terminal", icon: CommandLineIcon },
    { id: "split", label: "Split", icon: GridViewIcon },
  ]

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden">
      {/* Left sidebar */}
      <aside className="flex flex-col w-[240px] shrink-0 border-r border-border bg-sidebar h-full">
        {/* Sidebar header */}
        <div className="flex items-center justify-between px-3 py-3 border-b border-border">
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />
            <h2 className="text-[13px] font-semibold text-foreground">Issue Triage Agent</h2>
          </div>
          <button className="flex items-center justify-center h-6 w-6 rounded-md hover:bg-primary/10 transition-colors">
            <HugeiconsIcon icon={Add01Icon} size={13} className="text-primary" />
          </button>
        </div>

        {/* Search */}
        <div className="px-3 py-2 border-b border-border">
          <div className="flex items-center gap-2 rounded-lg bg-muted/50 px-2.5 py-1.5">
            <HugeiconsIcon icon={Search01Icon} size={13} className="text-muted-foreground" />
            <span className="text-xs text-muted-foreground">Search conversations...</span>
          </div>
        </div>

        {/* List */}
        <div className="flex-1 overflow-y-auto px-2 py-2">
          {Object.entries(sidebarConversations).map(([dateGroup, conversations]) => (
            <div key={dateGroup} className="mb-3">
              <span className="font-mono text-[9px] font-medium uppercase tracking-[1.5px] text-muted-foreground/50 px-3 mb-1 block">
                {dateGroup}
              </span>
              <div className="flex flex-col">
                {conversations.map((conversation) => (
                  <ConversationListItem
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

        {/* Sidebar footer */}
        <div className="px-3 py-2 border-t border-border">
          <div className="flex items-center justify-between text-[11px] text-muted-foreground font-mono">
            <span>11 conversations</span>
            <span>$0.42 today</span>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex flex-col flex-1 min-w-0">
        {/* Tab bar */}
        <div className="flex items-center justify-between px-2 py-1.5 border-b border-border shrink-0 bg-muted/20">
          <div className="flex items-center gap-0.5">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors cursor-pointer ${
                  activeTab === tab.id
                    ? "bg-background text-foreground border border-border shadow-sm"
                    : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                }`}
              >
                <HugeiconsIcon icon={tab.icon} size={13} />
                {tab.label}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-1">
            <Button variant="destructive" size="sm" className="h-7 text-xs">
              <HugeiconsIcon icon={StopIcon} size={12} data-icon="inline-start" />
              End run
            </Button>
            <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors">
              <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
            </button>
          </div>
        </div>

        {/* Active tab content */}
        <div className="flex-1 overflow-hidden">
          {activeTab === "chat" && <ChatView />}
          {activeTab === "browser" && <BrowserView />}
          {activeTab === "terminal" && <TerminalView />}
          {activeTab === "split" && <SplitView />}
        </div>
      </div>
    </div>
  )
}
