"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { ConversationIcon } from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { useAgentSessions } from "@/hooks/use-agent-sessions"
import type { components } from "@/lib/api/schema"
import { ChoiceCard } from "./create-agent/choice-card"

type Agent = components["schemas"]["agentResponse"]

interface AgentSessionsDialogProps {
  agent: Agent | null
  onOpenChange: (open: boolean) => void
}

export function AgentSessionsDialog({ agent, onOpenChange }: AgentSessionsDialogProps) {
  const open = agent !== null
  const { sessions, isLoading } = useAgentSessions(agent?.id ?? null)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(780px,85vh)] flex-col">
        <DialogHeader>
          <DialogTitle>Sessions - {agent?.name ?? "Agent"}</DialogTitle>
          <DialogDescription>
            Open a previous session or start a new one.
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className="-mx-6 min-h-0 flex-1 px-6">
          {isLoading ? (
            <div className="flex flex-col gap-2">
              <Skeleton className="h-16 w-full rounded-xl" />
              <Skeleton className="h-16 w-full rounded-xl" />
              <Skeleton className="h-16 w-full rounded-xl" />
            </div>
          ) : sessions.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-3 py-12">
              <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                <HugeiconsIcon
                  icon={ConversationIcon}
                  size={20}
                  className="text-muted-foreground"
                />
              </div>
              <div className="text-center">
                <p className="text-sm font-medium text-foreground">No sessions yet</p>
                <p className="mt-1 max-w-[240px] text-xs text-muted-foreground">
                  Start a new session with this agent and it'll show up here.
                </p>
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              {sessions.map((session) => {
                if (!session.id || !agent?.id) return null
                const title = session.name ?? session.id
                const description = [
                  session.status,
                  session.created_at ? new Date(session.created_at).toLocaleString() : null,
                ]
                  .filter(Boolean)
                  .join(" · ")
                return (
                  <ChoiceCard
                    key={session.id}
                    icon={ConversationIcon}
                    title={title}
                    description={description || "Session"}
                    href={`/w/agents/${agent.id}/sessions/${session.id}`}
                    onClick={() => onOpenChange(false)}
                  />
                )
              })}
            </div>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
