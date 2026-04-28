"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { Clock01Icon, GlobeIcon } from "@hugeicons/core-free-icons"
import { IntegrationLogo } from "@/components/integration-logo"
import type { TriggerConfig } from "../create-agent/types"

export function TriggerTypeAvatar({
  trigger,
  size = 28,
}: {
  trigger: TriggerConfig
  size?: number
}) {
  if (trigger.triggerType === "webhook") {
    return (
      <IntegrationLogo
        provider={trigger.provider}
        size={size}
        className="shrink-0 mt-0.5"
      />
    )
  }
  const sizeClass = size <= 24 ? "size-6" : size <= 28 ? "size-7" : "size-8"
  const iconSize = size <= 24 ? 12 : 14
  if (trigger.triggerType === "cron") {
    return (
      <div
        className={`flex items-center justify-center rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5 ${sizeClass}`}
      >
        <HugeiconsIcon icon={Clock01Icon} strokeWidth={2} size={iconSize} />
      </div>
    )
  }
  return (
    <div
      className={`flex items-center justify-center rounded-md bg-blue-500/10 text-blue-600 dark:text-blue-400 shrink-0 mt-0.5 ${sizeClass}`}
    >
      <HugeiconsIcon icon={GlobeIcon} strokeWidth={2} size={iconSize} />
    </div>
  )
}

export function triggerDisplayName(trigger: TriggerConfig): string {
  if (trigger.triggerType === "webhook") return trigger.connectionName
  if (trigger.triggerType === "cron") return "Cron schedule"
  return "HTTP trigger"
}

/**
 * Read-only pill that shows the HTTP endpoint URL for a trigger, or a
 * placeholder while the agent (and therefore the URL) doesn't exist yet.
 */
export function HttpEndpointPill({ url }: { url?: string }) {
  return (
    <div className="flex h-7 items-center rounded-full border border-dashed border-border/70 bg-muted/30 px-2.5 font-mono text-[11px] text-muted-foreground">
      {url ?? "HTTP endpoint will be generated after the agent is created."}
    </div>
  )
}
