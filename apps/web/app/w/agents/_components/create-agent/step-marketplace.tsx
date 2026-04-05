"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Search01Icon,
  Tick02Icon,
  Download04Icon,
  CheckmarkBadge01Icon,
} from "@hugeicons/core-free-icons"
import { DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { marketplaceAgents, integrations, connectedIntegrations } from "./data"

function formatInstalls(count: number) {
  if (count >= 1000) return `${(count / 1000).toFixed(count % 1000 === 0 ? 0 : 1)}k`
  return count.toString()
}

interface StepMarketplaceBrowseProps {
  onBack: () => void
  onSelect: (slug: string) => void
}

export function StepMarketplaceBrowse({ onBack, onSelect }: StepMarketplaceBrowseProps) {
  const [search, setSearch] = useState("")

  const filtered = marketplaceAgents.filter(
    (agent) =>
      agent.name.toLowerCase().includes(search.toLowerCase()) ||
      agent.description.toLowerCase().includes(search.toLowerCase()) ||
      agent.integrations.some((integration) => integration.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Marketplace</DialogTitle>
        </div>
        <DialogDescription className="mt-2">
          Browse community-built agents. Install one to get started instantly.
        </DialogDescription>
      </DialogHeader>

      <div className="relative mt-4">
        <HugeiconsIcon icon={Search01Icon} size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search agents..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="pl-9 h-9"
        />
      </div>

      <div className="flex flex-col gap-2 mt-4 flex-1 overflow-y-auto">
        {filtered.map((agent) => (
          <button
            key={agent.slug}
            onClick={() => onSelect(agent.slug)}
            className="group flex items-start gap-3 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
          >
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <p className="text-sm font-semibold text-foreground truncate">{agent.name}</p>
                {agent.verified && (
                  <HugeiconsIcon icon={CheckmarkBadge01Icon} size={13} className="text-green-500 shrink-0" />
                )}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2 leading-relaxed">{agent.description}</p>
              <div className="flex items-center gap-3 mt-2">
                <span className="flex items-center gap-1.5 text-[10px] text-muted-foreground/40">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img src={agent.publisher.avatar} alt="" className="h-3.5 w-3.5 rounded-full" />
                  {agent.publisher.name}
                </span>
                <span className="text-[10px] text-muted-foreground/30">·</span>
                <span className="font-mono text-[10px] text-muted-foreground/40">
                  {formatInstalls(agent.installs)} installs
                </span>
                <span className="flex items-center -space-x-1.5 ml-auto">
                  {agent.integrations.map((name) => {
                    const integ = integrations.find((item) => item.name === name)
                    return integ ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img key={name} src={integ.logo} alt={name} className="h-4 w-4 rounded-full border-2 border-muted/50 dark:invert" />
                    ) : (
                      <span key={name} className="flex h-4 w-4 items-center justify-center rounded-full border-2 border-muted/50 bg-muted text-[7px] font-bold text-muted-foreground">
                        {name[0]}
                      </span>
                    )
                  })}
                </span>
              </div>
            </div>
            <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
          </button>
        ))}
        {filtered.length === 0 && (
          <div className="flex items-center justify-center py-12">
            <p className="text-sm text-muted-foreground">No agents found</p>
          </div>
        )}
      </div>
    </div>
  )
}

interface StepMarketplaceDetailProps {
  slug: string
  onBack: () => void
  onInstall: () => void
}

export function StepMarketplaceDetail({ slug, onBack, onInstall }: StepMarketplaceDetailProps) {
  const agent = marketplaceAgents.find((item) => item.slug === slug)
  if (!agent) return null

  const missing = agent.integrations.filter((name) => !connectedIntegrations.has(name))
  const canInstall = missing.length === 0

  return (
    <div className="flex flex-col h-full">
      <DialogHeader>
        <div className="flex items-center gap-2">
          <button type="button" onClick={onBack} className="flex items-center justify-center h-7 w-7 rounded-lg hover:bg-muted transition-colors -ml-1">
            <HugeiconsIcon icon={ArrowLeft01Icon} size={16} className="text-muted-foreground" />
          </button>
          <DialogTitle>Agent details</DialogTitle>
        </div>
      </DialogHeader>

      <div className="flex flex-col gap-6 mt-6 flex-1 overflow-y-auto">
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold text-foreground">{agent.name}</h3>
            {agent.verified && (
              <HugeiconsIcon icon={CheckmarkBadge01Icon} size={14} className="text-green-500 shrink-0" />
            )}
          </div>
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground/50">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={agent.publisher.avatar} alt="" className="h-4 w-4 rounded-full" />
            {agent.publisher.name}
          </span>
        </div>

        <p className="text-sm text-muted-foreground leading-relaxed">{agent.description}</p>

        <div className="flex gap-6">
          <div className="flex flex-col">
            <span className="font-mono text-xs text-muted-foreground/60 uppercase tracking-wide">Installs</span>
            <span className="text-sm font-semibold text-foreground mt-0.5">{formatInstalls(agent.installs)}</span>
          </div>
          <div className="flex flex-col">
            <span className="font-mono text-xs text-muted-foreground/60 uppercase tracking-wide">Integrations</span>
            <span className="text-sm font-semibold text-foreground mt-0.5">{agent.integrations.length}</span>
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <span className="font-mono text-[10px] text-muted-foreground/60 uppercase tracking-[1px]">Required integrations</span>
          <div className="flex flex-wrap gap-2">
            {agent.integrations.map((name) => {
              const connected = connectedIntegrations.has(name)
              return (
                <span
                  key={name}
                  className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium ${
                    connected
                      ? "bg-muted text-foreground"
                      : "bg-destructive/5 text-destructive border border-destructive/10"
                  }`}
                >
                  {name}
                  {connected && (
                    <HugeiconsIcon icon={Tick02Icon} size={12} className="text-green-500" />
                  )}
                </span>
              )
            })}
          </div>
        </div>

        {missing.length > 0 && (
          <div className="rounded-xl border border-amber-500/20 bg-amber-500/5 px-4 py-3">
            <p className="text-sm text-amber-500/90 leading-snug">
              You are missing {missing.length} {missing.length === 1 ? "integration" : "integrations"} to install this agent. Please connect {missing.join(", ")} and try again.
            </p>
          </div>
        )}
      </div>

      <div className="pt-4 shrink-0">
        <Button onClick={onInstall} disabled={!canInstall} className="w-full">
          <HugeiconsIcon icon={Download04Icon} size={16} data-icon="inline-start" />
          Install agent
        </Button>
      </div>
    </div>
  )
}
