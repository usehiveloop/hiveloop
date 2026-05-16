"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Copy01Icon, GlobeIcon, Tick02Icon } from "@hugeicons/core-free-icons"
import { IntegrationLogo } from "@/components/integration-logo"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { cn } from "@/lib/utils"
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

export function HttpEndpointField({
  url,
  className,
}: {
  url?: string
  className?: string
}) {
  const [copied, setCopied] = React.useState(false)
  const value = url ?? "HTTP endpoint will be generated after the agent is saved."

  async function copyValue() {
    if (!url) return
    await navigator.clipboard.writeText(url)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  return (
    <InputGroup className={cn("h-8 w-full bg-input/40", className)}>
      <InputGroupInput
        value={value}
        readOnly
        disabled={!url}
        className="font-mono text-[11px]"
        onFocus={(event) => event.currentTarget.select()}
      />
      <InputGroupAddon align="inline-end">
        <button
          type="button"
          disabled={!url}
          onClick={copyValue}
          className="inline-flex shrink-0 items-center justify-center text-muted-foreground transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40"
          aria-label="Copy HTTP trigger endpoint"
        >
          <HugeiconsIcon
            icon={copied ? Tick02Icon : Copy01Icon}
            className="size-3.5"
            strokeWidth={2.5}
          />
        </button>
      </InputGroupAddon>
    </InputGroup>
  )
}
