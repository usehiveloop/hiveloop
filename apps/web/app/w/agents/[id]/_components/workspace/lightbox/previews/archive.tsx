"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Download01Icon, FileZipIcon } from "@hugeicons/core-free-icons"
import type { Asset } from "../preview-router"
import type { AssetKind } from "../preview-router"

// No safe in-browser preview for arbitrary archives. Instead of pretending,
// we lean into the uncertainty: a single confident card with the only two
// pieces of information the user actually has — what it is, how big it is —
// and the action they came to take.
export function ArchivePreview({ asset, kind }: { asset: Asset; kind: AssetKind }) {
  const label = kind === "archive" ? "Archive" : "Binary file"
  const tagline =
    kind === "archive"
      ? "Archives can't be previewed in the browser. Download to inspect the contents."
      : "This file type can't be previewed inline. Download to open it locally."

  return (
    <div className="flex h-full w-full items-center justify-center px-6 py-24">
      <div className="relative flex w-full max-w-[520px] flex-col items-center gap-6 overflow-hidden rounded-2xl bg-card px-10 py-12 text-center ring-1 ring-border shadow-sm">
        {/* Diagonal hairline pattern hints at "wrapped contents" without a
            literal stack-of-folders icon cliché. Uses --foreground via the
            CSS var so it adapts to both themes. */}
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 opacity-[0.05]"
          style={{
            backgroundImage:
              "repeating-linear-gradient(135deg, var(--foreground) 0 1px, transparent 1px 12px)",
          }}
        />

        <div className="relative flex size-14 items-center justify-center rounded-xl bg-muted text-foreground ring-1 ring-border">
          <HugeiconsIcon icon={FileZipIcon} size={22} />
        </div>

        <div className="relative flex flex-col gap-1.5">
          <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
            {label}
          </span>
          <h3 className="font-heading text-[20px] font-medium leading-tight text-foreground">
            {asset.filename}
          </h3>
          <p className="text-[12.5px] leading-relaxed text-muted-foreground">{tagline}</p>
        </div>

        <a
          href={asset.publicUrl}
          download={asset.filename}
          className="relative inline-flex items-center gap-2 rounded-full bg-foreground px-5 py-2 text-[12.5px] font-medium text-background transition-colors hover:bg-foreground/90"
        >
          <HugeiconsIcon icon={Download01Icon} size={13} />
          Download {formatBytes(asset.bytes)}
        </a>
      </div>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return ""
  const units = ["B", "KB", "MB", "GB", "TB"]
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  return `${value.toFixed(value >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}
