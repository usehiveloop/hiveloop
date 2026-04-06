"use client"

import { useMemo, useState } from "react"
import { useParams, useRouter, useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { PageLoader } from "@/components/page-loader"
import { AgentHeader } from "./_components/agent-header"
import { StatCards } from "./_components/stat-cards"
import { ActiveRuns } from "./_components/active-runs"
import { RecentRuns } from "./_components/recent-runs"
import { RunsEmpty } from "./_components/runs-empty"
import { RunPanel } from "./_components/run-panel"
import { EditAgentPanel } from "./_components/edit-agent-panel"
import type { Run, RunStatus } from "./_data/agent-detail"
import type { components } from "@/lib/api/schema"

type Conversation = components["schemas"]["conversationResponse"]

const staticStats = {
  totalRuns: 0,
  totalRunsTrend: 0,
  activeNow: 0,
  spendThisMonth: 0,
  spendTrend: 0,
  tokensThisMonth: 0,
  tokensTrend: 0,
  avgCostPerRun: 0,
  avgCostTrend: 0,
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000)
  if (seconds < 60) return "Just now"
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function mapStatus(status: string | undefined): RunStatus {
  switch (status) {
    case "active": return "running"
    case "error": return "error"
    case "ended": return "completed"
    default: return "completed"
  }
}

function conversationToRun(conv: Conversation): Run {
  return {
    id: conv.id ?? "",
    identity: "conversation",
    subject: `Run ${conv.id?.slice(0, 8) ?? ""}`,
    status: mapStatus(conv.status),
    duration: "\u2014",
    tokensIn: 0,
    tokensOut: 0,
    cost: 0,
    startedAt: conv.created_at ? timeAgo(conv.created_at) : "\u2014",
    events: [],
  }
}

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const searchParams = useSearchParams()
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)

  const activeRunId = searchParams.get("run")

  const { data: agent, isLoading: agentLoading } = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id } },
  })

  const { data: conversationsData } = $api.useQuery("get", "/v1/agents/{agentID}/conversations", {
    params: { path: { agentID: id } },
  })

  const conversations = conversationsData?.data ?? []

  const { activeRuns, recentRuns } = useMemo(() => {
    const active: Run[] = []
    const recent: Run[] = []
    for (const conv of conversations) {
      const run = conversationToRun(conv)
      if (conv.status === "active") {
        active.push(run)
      } else {
        recent.push(run)
      }
    }
    return { activeRuns: active, recentRuns: recent }
  }, [conversations])

  const hasRuns = activeRuns.length > 0 || recentRuns.length > 0

  const stats = useMemo(() => ({
    ...staticStats,
    totalRuns: conversations.length,
    activeNow: activeRuns.length,
  }), [conversations.length, activeRuns.length])

  const createConversation = $api.useMutation("post", "/v1/agents/{agentID}/conversations")

  function handleStartRun() {
    createConversation.mutate(
      { params: { path: { agentID: id } } },
      {
        onSuccess: (data) => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents/{agentID}/conversations"] })
          if (data.id) {
            router.push(`/w/agents/${id}?run=${data.id}`)
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to start run"))
        },
      },
    )
  }

  function handleSelectRun(run: Run) {
    router.push(`/w/agents/${id}?run=${run.id}`)
  }

  function handleCloseRun() {
    router.push(`/w/agents/${id}`)
  }

  if (agentLoading || !agent) {
    return <PageLoader description="Loading agent details" />
  }

  return (
    <>
      <div className="max-w-464 mx-auto w-full px-4 py-8">
        <AgentHeader
          name={agent.name ?? "Untitled Agent"}
          provider={agent.provider_id ?? ""}
          model={agent.model ?? ""}
          sandboxType={agent.sandbox_type ?? "shared"}
          memoryEnabled={agent.shared_memory ?? false}
          status={agent.status ?? "active"}
          onStartConversation={handleStartRun}
          startingConversation={createConversation.isPending}
          onEdit={() => setEditOpen(true)}
        />

        {hasRuns && <StatCards stats={stats} />}

        {hasRuns ? (
          <div className="flex flex-col gap-8">
            <ActiveRuns runs={activeRuns} onSelectRun={handleSelectRun} />
            <RecentRuns runs={recentRuns} onSelectRun={handleSelectRun} />
          </div>
        ) : (
          <RunsEmpty
            onStartRun={handleStartRun}
            startingRun={createConversation.isPending}
            onForgeAgent={() => {/* TODO: start forge run */}}
            onEditAgent={() => setEditOpen(true)}
          />
        )}
      </div>

      {activeRunId && (
        <RunPanel conversationId={activeRunId} onClose={handleCloseRun} />
      )}

      <EditAgentPanel
        open={editOpen}
        onOpenChange={setEditOpen}
        agent={agent}
      />
    </>
  )
}
