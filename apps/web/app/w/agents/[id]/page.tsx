"use client"

import { useState } from "react"
import { AgentHeader } from "./_components/agent-header"
import { StatCards } from "./_components/stat-cards"
import { ActiveRuns } from "./_components/active-runs"
import { RecentRuns } from "./_components/recent-runs"
import { RunPanel } from "./_components/run-panel"
import { EditAgentPanel } from "./_components/edit-agent-panel"
import { agent, activeRuns, recentRuns, type Run } from "./_data/agent-detail"

export default function AgentDetailPage() {
  const [selectedRun, setSelectedRun] = useState<Run | null>(null)
  const [editOpen, setEditOpen] = useState(false)

  return (
    <>
      <div className="max-w-464 mx-auto w-full px-4 py-8">
        <AgentHeader
          name={agent.name}
          provider={agent.provider}
          model={agent.model}
          sandboxType={agent.sandboxType}
          memoryEnabled={agent.memoryEnabled}
          status={agent.status}
          onEdit={() => setEditOpen(true)}
        />

        <StatCards stats={agent.stats} />

        <div className="flex flex-col gap-8">
          <ActiveRuns runs={activeRuns} onSelectRun={setSelectedRun} />
          <RecentRuns runs={recentRuns} onSelectRun={setSelectedRun} />
        </div>
      </div>

      {selectedRun && (
        <RunPanel run={selectedRun} onClose={() => setSelectedRun(null)} />
      )}

      <EditAgentPanel open={editOpen} onOpenChange={setEditOpen} />
    </>
  )
}
