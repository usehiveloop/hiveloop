"use client"

import Link from "next/link"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  PencilEdit02Icon,
  Store01Icon,
} from "@hugeicons/core-free-icons"

export function AgentsEmpty() {
  return (
    <div className="flex flex-col items-center px-4 pt-[20vh] pb-24">
      <div className="mb-8 text-center">
        <h2 className="font-heading text-2xl font-semibold text-foreground">
          Create your first agent
        </h2>
        <p className="mt-2 max-w-sm text-sm text-muted-foreground">
          Agents are autonomous workers that use your integrations and LLM keys to get things done.
        </p>
      </div>

      <div className="flex w-full max-w-sm flex-col gap-2">
        <Link
          href="/w/agents/new"
          className="group flex w-full items-start gap-4 rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted"
        >
          <HugeiconsIcon
            icon={PencilEdit02Icon}
            size={20}
            className="mt-0.5 shrink-0 text-muted-foreground"
          />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-foreground">Create from scratch</p>
            <p className="mt-0.5 text-[13px] leading-relaxed text-muted-foreground">
              Write your own system prompt and configure every detail manually.
            </p>
          </div>
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            size={16}
            className="mt-0.5 shrink-0 text-muted-foreground/30"
          />
        </Link>

        <button
          type="button"
          className="group flex w-full items-start gap-4 rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
        >
          <HugeiconsIcon
            icon={Store01Icon}
            size={20}
            className="mt-0.5 shrink-0 text-muted-foreground"
          />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold text-foreground">Install from marketplace</p>
            <p className="mt-0.5 text-[13px] leading-relaxed text-muted-foreground">
              Browse community-built agents and install one in seconds.
            </p>
          </div>
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            size={16}
            className="mt-0.5 shrink-0 text-muted-foreground/30"
          />
        </button>
      </div>
    </div>
  )
}
