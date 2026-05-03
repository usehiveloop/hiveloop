"use client"

import * as React from "react"
import type { Asset } from "../preview-router"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; text: string }
  | { status: "error"; message: string }

const MAX_BYTES = 1 * 1024 * 1024

export function TextPreview({ asset }: { asset: Asset }) {
  const [state, setState] = React.useState<LoadState>({ status: "loading" })

  React.useEffect(() => {
    let cancelled = false
    setState({ status: "loading" })
    fetch(asset.publicUrl)
      .then(async (res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const blob = await res.blob()
        if (blob.size > MAX_BYTES) {
          throw new Error("File is too large to render inline. Try downloading it.")
        }
        return blob.text()
      })
      .then((text) => {
        if (!cancelled) setState({ status: "ready", text })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        const message = err instanceof Error ? err.message : "Failed to load"
        setState({ status: "error", message })
      })
    return () => {
      cancelled = true
    }
  }, [asset.publicUrl])

  return (
    <div className="flex h-full w-full items-start justify-center overflow-hidden px-6 pb-24 pt-24">
      <div className="flex w-full max-w-[820px] overflow-hidden rounded-md bg-card ring-1 ring-border">
        {state.status === "loading" ? <TextSkeleton /> : null}
        {state.status === "error" ? (
          <p className="w-full px-8 py-12 text-center text-[12px] text-muted-foreground">
            {state.message}
          </p>
        ) : null}
        {state.status === "ready" ? (
          <pre className="m-0 max-h-[calc(100vh-12rem)] w-full overflow-auto whitespace-pre-wrap break-words px-8 py-7 font-mono text-[12px] leading-[1.7] text-foreground">
            {state.text}
          </pre>
        ) : null}
      </div>
    </div>
  )
}

function TextSkeleton() {
  return (
    <div className="flex w-full flex-col gap-2 p-7">
      {Array.from({ length: 12 }).map((_, i) => (
        <span
          key={i}
          className="h-3 animate-pulse rounded bg-muted"
          style={{
            width: `${40 + Math.round(Math.cos(i * 0.7) * 30 + 30)}%`,
            animationDelay: `${i * 30}ms`,
          }}
        />
      ))}
    </div>
  )
}
