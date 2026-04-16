"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  Search01Icon,
  Add01Icon,
  Wrench01Icon,
  ArrowDown01Icon,
  BrowserIcon,
  CommandLineIcon,
  Settings01Icon,
  ArrowUp01Icon,
  MoreHorizontalIcon,
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
   Variant 1 — Classic Split

   Left:  280px sidebar with date-grouped conversations
   Right: Full conversation canvas
   Top-right: Toggle buttons for Browser + Terminal panels
              that stack 60/40 when open
   ──────────────────────────────────────────────────────────── */

function ConversationListItem({ conversation, isActive, onClick }: {
  conversation: ConversationSummary
  isActive: boolean
  onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`flex flex-col gap-1 rounded-xl px-3 py-2.5 text-left transition-colors cursor-pointer w-full ${
        isActive
          ? "bg-primary/10 border border-primary/20"
          : "hover:bg-muted border border-transparent"
      }`}
    >
      <div className="flex items-center gap-2">
        {conversation.status === "active" && (
          <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse shrink-0" />
        )}
        {conversation.status === "error" && (
          <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" />
        )}
        <span className="text-sm font-medium text-foreground truncate flex-1">{conversation.title}</span>
      </div>
      <p className="text-xs text-muted-foreground truncate pl-3.5">{conversation.preview}</p>
      <div className="flex items-center justify-between pl-3.5">
        <span className="text-[11px] text-muted-foreground/60 font-mono">{conversation.date}</span>
        <span className="text-[11px] text-muted-foreground/60 font-mono">{(conversation.tokenCount / 1000).toFixed(1)}k tokens</span>
      </div>
    </button>
  )
}

function Sidebar({ activeId, onSelect }: {
  activeId: string
  onSelect: (id: string) => void
}) {
  return (
    <aside className="flex flex-col w-[280px] shrink-0 border-r border-border bg-sidebar h-full">
      {/* Sidebar header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h2 className="text-sm font-semibold text-foreground">Conversations</h2>
        <div className="flex items-center gap-1">
          <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors">
            <HugeiconsIcon icon={Search01Icon} size={14} className="text-muted-foreground" />
          </button>
          <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-primary/10 transition-colors">
            <HugeiconsIcon icon={Add01Icon} size={14} className="text-primary" />
          </button>
        </div>
      </div>

      {/* Conversation list */}
      <div className="flex-1 overflow-y-auto px-2 py-2">
        {Object.entries(sidebarConversations).map(([dateGroup, conversations]) => (
          <div key={dateGroup} className="mb-4">
            <span className="font-mono text-[10px] font-medium uppercase tracking-[1.5px] text-muted-foreground/60 px-3 mb-1.5 block">
              {dateGroup}
            </span>
            <div className="flex flex-col gap-0.5">
              {conversations.map((conversation) => (
                <ConversationListItem
                  key={conversation.id}
                  conversation={conversation}
                  isActive={conversation.id === activeId}
                  onClick={() => onSelect(conversation.id)}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  )
}

function ToolCallMessage({ message }: { message: MessageItem }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = message.toolStatus === "running"
  const isSuccess = message.toolStatus === "success"

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
        <span className="font-mono text-xs font-medium text-foreground flex-1 min-w-0 truncate">{message.toolName}</span>
        <div className="flex items-center gap-2 shrink-0">
          {isRunning ? (
            <span className="font-mono text-[11px] text-primary">Running...</span>
          ) : (
            <span className="font-mono text-[11px] text-muted-foreground">{message.toolDuration}</span>
          )}
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            size={14}
            className={`text-muted-foreground transition-transform duration-200 ${expanded ? "rotate-180" : ""}`}
          />
        </div>
      </button>

      {expanded && (
        <div className="border-t border-border px-4 py-3 flex flex-col gap-3">
          {message.toolParams && Object.keys(message.toolParams).length > 0 && (
            <div>
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Arguments</span>
              <div className="mt-1.5 rounded-lg bg-muted p-3">
                <div className="flex flex-col gap-1">
                  {Object.entries(message.toolParams).map(([key, value]) => (
                    <div key={key} className="flex gap-2 font-mono text-[11px]">
                      <span className="text-muted-foreground shrink-0">{key}:</span>
                      <span className="text-foreground break-all">{value}</span>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
          {message.toolResponse && (
            <div>
              <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Response</span>
              <div className="mt-1.5 rounded-lg bg-muted p-3 overflow-x-auto">
                <pre className="font-mono text-[11px] text-foreground whitespace-pre-wrap break-all leading-relaxed">
                  {message.toolResponse}
                </pre>
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
      return <ToolCallMessage message={message} />
    case "error":
      return (
        <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-destructive">Error</span>
          <p className="text-sm text-destructive mt-1.5">{message.content}</p>
        </div>
      )
    default:
      return null
  }
}

function BrowserPanel() {
  return (
    <div className="flex flex-col h-full border-b border-border">
      {/* Browser chrome */}
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/30 border-b border-border shrink-0">
        <div className="flex items-center gap-1.5">
          <span className="h-2.5 w-2.5 rounded-full bg-destructive/60" />
          <span className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
          <span className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
        </div>
        <div className="flex-1 flex items-center gap-2 rounded-lg bg-background border border-border px-3 py-1">
          <span className="text-[11px] text-green-600 font-mono">https://</span>
          <span className="text-[11px] text-foreground font-mono">{browserContent.url.replace("https://", "")}</span>
        </div>
      </div>
      {/* Browser content */}
      <div className="flex-1 overflow-y-auto p-4 bg-background">
        <div className="flex flex-col items-center justify-center gap-4 py-8">
          <div className="w-64 h-10 rounded-lg bg-muted animate-pulse" />
          <div className="w-48 h-8 rounded-lg bg-muted" />
          <div className="w-56 h-8 rounded-lg bg-muted" />
          <div className="w-40 h-9 rounded-lg bg-primary/20" />
        </div>
        {/* Console errors */}
        <div className="mt-4 border-t border-border pt-3">
          <span className="font-mono text-[10px] font-medium uppercase tracking-[1px] text-muted-foreground">Console</span>
          <div className="mt-2 flex flex-col gap-1">
            {browserContent.consoleErrors.map((error, index) => (
              <div key={index} className={`flex items-start gap-2 rounded-lg px-3 py-1.5 font-mono text-[11px] ${
                error.level === "error" ? "bg-destructive/5 text-destructive" : "bg-yellow-500/5 text-yellow-600"
              }`}>
                <span className="shrink-0 text-muted-foreground">{error.timestamp}</span>
                <span className="break-all">{error.message}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function TerminalPanel() {
  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-2 bg-muted/30 border-b border-border shrink-0">
        <HugeiconsIcon icon={CommandLineIcon} size={12} className="text-muted-foreground" />
        <span className="text-[11px] font-mono text-muted-foreground">Terminal</span>
      </div>
      <div className="flex-1 overflow-y-auto bg-[oklch(0.16_0.01_250)] p-4">
        <pre className="font-mono text-[11px] leading-relaxed text-green-400 whitespace-pre-wrap">{terminalOutput}</pre>
      </div>
    </div>
  )
}

export default function ConversationV1() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [showBrowser, setShowBrowser] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)

  const panelsOpen = showBrowser || showTerminal

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden">
      {/* Left sidebar */}
      <Sidebar activeId={activeConversation} onSelect={setActiveConversation} />

      {/* Main content area */}
      <div className="flex flex-1 min-w-0">
        {/* Conversation canvas */}
        <div className="flex flex-col flex-1 min-w-0">
          {/* Canvas header */}
          <div className="flex items-center justify-between px-5 py-3 border-b border-border shrink-0">
            <div className="flex items-center gap-3 min-w-0">
              <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-foreground truncate">Debug Safari login regression</h2>
                <p className="text-[11px] text-muted-foreground font-mono mt-0.5">conv_001 &middot; Started 10:42 AM</p>
              </div>
            </div>
            <div className="flex items-center gap-1 shrink-0">
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

          {/* Messages */}
          <div className="flex-1 overflow-y-auto px-5 py-4">
            <div className="max-w-3xl mx-auto flex flex-col gap-3">
              {activeConversationMessages.map((message) => (
                <MessageBubble key={message.id} message={message} />
              ))}
            </div>
          </div>

          {/* Input area */}
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
                <Textarea
                  placeholder="Send a message..."
                  className="min-h-[72px] max-h-40"
                />
              </div>
            </div>
          </div>
        </div>

        {/* Right panels (Browser + Terminal) */}
        {panelsOpen && (
          <div className="flex flex-col w-[420px] shrink-0 border-l border-border">
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
          </div>
        )}
      </div>
    </div>
  )
}
