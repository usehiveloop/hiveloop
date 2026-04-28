"use client"

import { useParams, useRouter } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { PageLoader } from "@/components/page-loader"
import { CreateAgentProvider } from "@/app/w/agents/_components/create-agent/context"
import { AgentForm } from "@/app/w/agents/_components/create-agent/agent-form"

export default function EditAgentPage() {
  const router = useRouter()
  const { id } = useParams<{ id: string }>()

  const { data: agent, isLoading } = $api.useQuery("get", "/v1/agents/{id}", {
    params: { path: { id } },
  })

  if (isLoading || !agent) {
    return <PageLoader description="Loading agent" />
  }

  return (
    <CreateAgentProvider agent={agent} onClose={() => router.push(`/w/agents/${id}`)}>
      <AgentForm />
    </CreateAgentProvider>
  )
}
