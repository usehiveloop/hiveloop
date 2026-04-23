"use client"

import { useParams, useRouter, usePathname } from "next/navigation"
import { motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Search01Icon,
  Add01Icon,
  Robot01Icon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import type { ConversationSummary } from "../_data/conversation-mock"

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
          layoutId="active-pill"
          className="absolute inset-0 rounded-xl bg-primary/8 border border-primary/12 -z-[1]"
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

export default function ConversationsLayout({ children }: { children: React.ReactNode }) {
  const params = useParams()
  const router = useRouter()
  const pathname = usePathname()
  const agentId = params.id as string
  const activeConversationId = params.conversationId as string | undefined

  const { data: agent } = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id: agentId } },
  })

  const { data: conversationsData } = $api.useQuery("get", "/v1/agents/{agentID}/conversations", {
    params: { path: { agentID: agentId } },
  })

  const conversations = conversationsData?.data ?? []

  const grouped = conversations.reduce<Record<string, typeof conversations>>((groups, conv) => {
    const date = conv.created_at ? new Date(conv.created_at) : new Date()
    const now = new Date()
    const isToday = date.toDateString() === now.toDateString()
    const isYesterday = date.toDateString() === new Date(now.getTime() - 86400000).toDateString()
    const label = isToday ? "Today" : isYesterday ? "Yesterday" : date.toLocaleDateString()
    if (!groups[label]) groups[label] = []
    groups[label].push(conv)
    return groups
  }, {})

  let conversationIndex = 0

  return (
    <div className="flex h-[calc(100vh-54px)] overflow-hidden bg-background">
      <aside className="flex flex-col w-[300px] shrink-0 border-r border-border bg-sidebar h-full">
        <div className="flex items-center justify-between px-4 py-3.5 border-b border-border">
          <div className="flex items-center gap-2.5">
            <div className="h-7 w-7 rounded-xl bg-primary/15 flex items-center justify-center">
              <HugeiconsIcon icon={Robot01Icon} size={14} className="text-primary" />
            </div>
            <div>
              <h2 className="text-[13px] font-semibold text-foreground leading-tight">{agent?.name ?? "Agent"}</h2>
              <span className="text-[10px] text-muted-foreground/40 font-mono">{agent?.model ?? ""}</span>
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
          {Object.entries(grouped).map(([group, items]) => (
            <div key={group} className="mb-3">
              <div className="flex items-center gap-2 px-3 py-1.5">
                <span className="text-[10px] font-semibold uppercase tracking-[1.5px] text-muted-foreground/25">{group}</span>
                <div className="flex-1 h-px bg-border/40" />
              </div>
              {items.map((conv) => {
                const currentIndex = conversationIndex++
                const convId = conv.id ?? ""
                const isActive = convId === activeConversationId
                const summary: ConversationSummary = {
                  id: convId,
                  title: conv.name && conv.name.length > 0 ? conv.name : convId.slice(0, 8),
                  preview: "",
                  status: (conv.status ?? "active") as ConversationSummary["status"],
                  date: conv.created_at ? new Date(conv.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "",
                  tokenCount: 0,
                }
                return (
                  <ConversationItem
                    key={convId}
                    conversation={summary}
                    isActive={isActive}
                    onClick={() => router.push(`/w/agents/${agentId}/conversations/${convId}`)}
                    index={currentIndex}
                  />
                )
              })}
            </div>
          ))}
          {conversations.length === 0 && (
            <div className="flex items-center justify-center py-10 text-[12px] text-muted-foreground/40">
              No conversations yet
            </div>
          )}
        </div>
      </aside>

      <div className="flex flex-1 min-w-0">
        {children}
      </div>
    </div>
  )
}
