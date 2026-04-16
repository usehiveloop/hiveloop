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
  DragDropVerticalIcon,
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
   Variant 3 — Full Canvas + Bottom Drawer

   Left:   280px sidebar with date-grouped conversations
   Center: Full conversation canvas
   Bottom: Resizable drawer that slides up with tabs for
           Browser + Terminal (like VS Code bottom panel)
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
        isActive ? "bg-primary/10 border border-primary/20" : "hover:bg-muted border border-transparent"
      }`}
    >
      <div className="flex items-center gap-2">
        {conversation.status === "active" && (
          <span className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse shrink-0" />
        )}
        {conversation.status === "error" && (
          <span className="h-1.5 w-1.5 rounded-full bg-destructive shrink-0" />
        )}
        {conversation.status === "ended" && (
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30 shrink-0" />
        )}
        <span className="text-[13px] font-medium text-foreground truncate flex-1">{conversation.title}</span>
      </div>
      <p className="text-xs text-muted-foreground truncate pl-3.5">{conversation.preview}</p>
      <div className="flex items-center justify-between pl-3.5">
        <span className="text-[11px] text-muted-foreground/50 font-mono">{conversation.date}</span>
        <span className="text-[11px] text-muted-foreground/50 font-mono">{(conversation.tokenCount / 1000).toFixed(1)}k</span>
      </div>
    </button>
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
        className="flex items-center gap-3 w-full px-4 py-2.5 text-left hover:bg-muted/50 transition-colors cursor-pointer"
      >
        <HugeiconsIcon
          icon={Wrench01Icon}
          size={13}
          className={`shrink-0 ${isRunning ? "text-primary animate-spin" : isSuccess ? "text-green-500" : "text-destructive"}`}
        />
        <span className="font-mono text-[11px] font-medium text-foreground flex-1 min-w-0 truncate">{message.toolName}</span>
        <div className="flex items-center gap-2 shrink-0">
          {isRunning ? (
            <span className="font-mono text-[11px] text-primary">Running...</span>
          ) : (
            <span className="font-mono text-[11px] text-muted-foreground">{message.toolDuration}</span>
          )}
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            size={12}
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
    default:
      return null
  }
}

export default function ConversationV3() {
  const [activeConversation, setActiveConversation] = useState("conv_001")
  const [drawerOpen, setDrawerOpen] = useState(true)
  const [activeDrawerTab, setActiveDrawerTab] = useState<"browser" | "terminal">("browser")

  const drawerTabs = [
    { id: "browser" as const, label: "Browser", icon: BrowserIcon },
    { id: "terminal" as const, label: "Terminal", icon: CommandLineIcon },
  ]

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden">
      {/* Left sidebar */}
      <aside className="flex flex-col w-[280px] shrink-0 border-r border-border bg-sidebar h-full">
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
                    isActive={conversation.id === activeConversation}
                    onClick={() => setActiveConversation(conversation.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      </aside>

      {/* Main area with conversation + bottom drawer */}
      <div className="flex flex-col flex-1 min-w-0">
        {/* Conversation header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-border shrink-0">
          <div className="flex items-center gap-3 min-w-0">
            <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse shrink-0" />
            <div className="min-w-0">
              <h2 className="text-sm font-semibold text-foreground truncate">Debug Safari login regression</h2>
              <p className="text-[11px] text-muted-foreground font-mono mt-0.5">conv_001 &middot; 10:42 AM &middot; Active</p>
            </div>
          </div>
          <div className="flex items-center gap-1 shrink-0">
            <Button
              variant={drawerOpen ? "secondary" : "outline"}
              size="sm"
              onClick={() => setDrawerOpen(!drawerOpen)}
              className="h-7 text-xs"
            >
              <HugeiconsIcon icon={CommandLineIcon} size={13} data-icon="inline-start" />
              {drawerOpen ? "Hide tools" : "Show tools"}
            </Button>
            <button className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors">
              <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
            </button>
          </div>
        </div>

        {/* Conversation messages — takes remaining vertical space */}
        <div className={`overflow-y-auto px-5 py-4 ${drawerOpen ? "flex-1" : "flex-1"}`} style={{ flex: drawerOpen ? "1 1 0%" : "1 1 auto" }}>
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
          <div className="px-4 pb-3">
            <div className="max-w-3xl mx-auto">
              <Textarea placeholder="Send a message..." className="min-h-[64px] max-h-32" />
            </div>
          </div>
        </div>

        {/* Bottom drawer */}
        {drawerOpen && (
          <div className="shrink-0 border-t border-border" style={{ height: "38%" }}>
            {/* Drawer header with drag handle and tabs */}
            <div className="flex items-center gap-1 px-3 py-0 border-b border-border bg-muted/20">
              {/* Drag handle */}
              <div className="flex items-center justify-center w-full py-1 cursor-row-resize">
                <div className="w-8 h-1 rounded-full bg-border" />
              </div>
            </div>
            <div className="flex items-center gap-1 px-3 py-1.5 border-b border-border bg-muted/20">
              {drawerTabs.map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => setActiveDrawerTab(tab.id)}
                  className={`flex items-center gap-1.5 px-3 py-1 rounded-lg text-[11px] font-medium transition-colors cursor-pointer ${
                    activeDrawerTab === tab.id
                      ? "bg-background text-foreground border border-border shadow-sm"
                      : "text-muted-foreground hover:text-foreground hover:bg-muted/50"
                  }`}
                >
                  <HugeiconsIcon icon={tab.icon} size={12} />
                  {tab.label}
                </button>
              ))}
              <div className="flex-1" />
              <button
                onClick={() => setDrawerOpen(false)}
                className="flex items-center justify-center h-6 w-6 rounded-md hover:bg-muted transition-colors"
              >
                <HugeiconsIcon icon={ArrowDown01Icon} size={12} className="text-muted-foreground" />
              </button>
            </div>

            {/* Drawer content */}
            <div className="flex-1 h-[calc(100%-68px)] overflow-hidden">
              {activeDrawerTab === "browser" && (
                <div className="h-full flex flex-col">
                  {/* Browser URL bar */}
                  <div className="flex items-center gap-2 px-3 py-2 bg-muted/10 border-b border-border shrink-0">
                    <div className="flex items-center gap-1.5">
                      <span className="h-2 w-2 rounded-full bg-destructive/60" />
                      <span className="h-2 w-2 rounded-full bg-yellow-500/60" />
                      <span className="h-2 w-2 rounded-full bg-green-500/60" />
                    </div>
                    <div className="flex-1 flex items-center rounded-md bg-background border border-border px-2.5 py-0.5">
                      <span className="text-[10px] text-green-600 font-mono">https://</span>
                      <span className="text-[10px] text-foreground font-mono">{browserContent.url.replace("https://", "")}</span>
                    </div>
                  </div>
                  {/* Content area as two columns: viewport + console */}
                  <div className="flex flex-1 overflow-hidden">
                    <div className="flex-1 p-4 overflow-y-auto">
                      <div className="flex flex-col items-center justify-center gap-3 py-4">
                        <div className="w-56 h-9 rounded-lg bg-muted animate-pulse" />
                        <div className="w-44 h-7 rounded-lg bg-muted" />
                        <div className="w-48 h-7 rounded-lg bg-muted" />
                        <div className="w-36 h-8 rounded-lg bg-primary/20" />
                      </div>
                    </div>
                    <div className="w-[360px] shrink-0 border-l border-border p-3 overflow-y-auto bg-muted/10">
                      <span className="font-mono text-[9px] font-medium uppercase tracking-[1px] text-muted-foreground">Console output</span>
                      <div className="mt-2 flex flex-col gap-1">
                        {browserContent.consoleErrors.map((error, index) => (
                          <div key={index} className={`rounded px-2 py-1.5 font-mono text-[10px] leading-relaxed ${
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
              )}

              {activeDrawerTab === "terminal" && (
                <div className="h-full overflow-y-auto bg-[oklch(0.16_0.01_250)] p-4">
                  <pre className="font-mono text-[11px] leading-relaxed text-green-400 whitespace-pre-wrap">{terminalOutput}</pre>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
