"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  BrowserIcon,
  ReloadIcon,
} from "@hugeicons/core-free-icons"

const PREVIEW_URL: string | null = "https://usehiveloop.com"

export function BrowserView() {
  if (!PREVIEW_URL) return <BrowserEmpty />
  return <BrowserPreview url={PREVIEW_URL} />
}

function BrowserPreview({ url }: { url: string }) {
  return (
    <div className="flex h-full flex-col p-6 pb-24">
      <div className="flex h-full flex-col overflow-hidden rounded-xl border border-border/60 bg-background shadow-sm">
        <BrowserChrome url={url} />
        <iframe
          title="App preview"
          src={url}
          sandbox="allow-same-origin allow-scripts allow-popups allow-forms"
          className="size-full flex-1 bg-background"
        />
      </div>
    </div>
  )
}

function BrowserChrome({ url }: { url: string }) {
  return (
    <div className="flex shrink-0 items-center gap-3 border-b border-border/60 bg-muted/30 px-3 py-2">
      <div className="flex gap-1.5">
        <span className="size-2.5 rounded-full bg-destructive/40" />
        <span className="size-2.5 rounded-full bg-amber-500/50" />
        <span className="size-2.5 rounded-full bg-emerald-500/50" />
      </div>
      <div className="flex items-center gap-0.5 text-muted-foreground/60">
        <button type="button" className="rounded p-1 hover:bg-muted hover:text-foreground">
          <HugeiconsIcon icon={ArrowLeft01Icon} size={13} />
        </button>
        <button type="button" className="rounded p-1 hover:bg-muted hover:text-foreground">
          <HugeiconsIcon icon={ArrowRight01Icon} size={13} />
        </button>
        <button type="button" className="rounded p-1 hover:bg-muted hover:text-foreground">
          <HugeiconsIcon icon={ReloadIcon} size={13} />
        </button>
      </div>
      <div className="flex flex-1 items-center gap-2 truncate rounded-md border border-border/40 bg-background px-3 py-1 font-mono text-[11px] text-muted-foreground">
        {url}
      </div>
    </div>
  )
}

function BrowserEmpty() {
  return (
    <div className="flex h-full items-center justify-center px-10">
      <div className="flex max-w-md flex-col items-center gap-4 text-center">
        <div className="flex size-12 items-center justify-center rounded-2xl bg-muted/40 text-muted-foreground/60">
          <HugeiconsIcon icon={BrowserIcon} size={20} />
        </div>
        <div className="flex flex-col gap-1.5">
          <h2 className="text-[15px] font-medium text-foreground">No preview yet</h2>
          <p className="text-[13px] leading-relaxed text-muted-foreground">
            When the agent deploys an app or starts a preview server, it will appear here.
          </p>
        </div>
      </div>
    </div>
  )
}
