"use client"

import { useRouter } from "next/navigation"
import { CreateAgentProvider } from "@/app/w/agents/_components/create-agent/context"
import { AgentForm } from "@/app/w/agents/_components/create-agent/agent-form"

export default function NewAgentPage() {
  const router = useRouter()
  return (
    <CreateAgentProvider onClose={() => router.push("/w/agents")}>
      <AgentForm />
    </CreateAgentProvider>
  )
}
