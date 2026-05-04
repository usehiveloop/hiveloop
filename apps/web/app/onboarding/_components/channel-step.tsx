"use client"

import { useState } from "react"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { integrationLogoURL } from "@/components/integration-logo"
import { SlackSetupDialog } from "./slack-setup-dialog"
import { StepHeader } from "./step-header"
import { useOnboarding, type Channel } from "./context"

const CHANNELS: Array<{
  id: Channel
  title: string
  description: string
  logoUrl: string
  logoSize: number
}> = [
  {
    id: "slack",
    title: "Slack",
    description: "Let your AI employee live in your workspace channels and DMs.",
    logoUrl: integrationLogoURL("slack"),
    logoSize: 36,
  },
  {
    id: "whatsapp",
    title: "WhatsApp",
    description: "Reach customers and teammates on the messaging app they already use.",
    logoUrl: "/images/whatsapp.svg",
    logoSize: 32,
  },
]

export function ChannelStep() {
  const { selectChannel } = useOnboarding()
  const [slackOpen, setSlackOpen] = useState(false)

  function handleClick(channelId: Channel) {
    if (channelId === "slack") {
      setSlackOpen(true)
      return
    }
    selectChannel(channelId)
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-col items-center gap-8 text-center">
      <StepHeader
        title="Where should your AI employee work?"
        description="Pick the channel they'll use to talk to your team and customers. You can add more later."
      />

      <div className="flex w-full flex-col gap-3">
        {CHANNELS.map((channel) => (
          <ChoiceCard
            key={channel.id}
            logoUrl={channel.logoUrl}
            logoSize={channel.logoSize}
            title={channel.title}
            description={channel.description}
            onClick={() => handleClick(channel.id)}
          />
        ))}
      </div>

      <SlackSetupDialog
        open={slackOpen}
        onOpenChange={setSlackOpen}
        onContinue={() => {
          setSlackOpen(false)
          selectChannel("slack")
        }}
      />
    </div>
  )
}
