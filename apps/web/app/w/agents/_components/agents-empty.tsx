"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import { PencilEdit02Icon, Store01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"

interface AgentsEmptyProps {
  onCreateFromScratch: () => void
  onCreateFromMarketplace: () => void
}

export function AgentsEmpty({ onCreateFromScratch, onCreateFromMarketplace }: AgentsEmptyProps) {
  return (
    <div className="flex flex-col items-center pt-[20vh] pb-24 px-4">
      <div className="text-center mb-8">
        <h2 className="font-heading text-2xl font-semibold text-foreground">
          Create your first agent
        </h2>
        <p className="text-sm text-muted-foreground mt-2 max-w-sm">
          Agents are autonomous workers that use your integrations and LLM keys to get things done.
        </p>
      </div>

      <div className="w-full max-w-sm flex flex-col gap-2">
        <button
          type="button"
          onClick={onCreateFromScratch}
          className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
        >
          <HugeiconsIcon icon={PencilEdit02Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-foreground">Create from scratch</p>
            <p className="text-sm-alt text-muted-foreground mt-0.5 leading-relaxed">
              Write your own system prompt and configure every detail manually.
            </p>
          </div>
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
        </button>

        <button
          type="button"
          onClick={onCreateFromMarketplace}
          className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
        >
          <HugeiconsIcon icon={Store01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-semibold text-foreground">Install from marketplace</p>
            <p className="text-sm-alt text-muted-foreground mt-0.5 leading-relaxed">
              Browse community-built agents and install one in seconds.
            </p>
          </div>
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
        </button>
      </div>
    </div>
  )
}
