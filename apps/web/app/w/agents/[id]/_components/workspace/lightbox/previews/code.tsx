"use client"

import * as React from "react"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism"
import type { Asset } from "../preview-router"

type LoadState =
  | { status: "loading" }
  | { status: "ready"; text: string }
  | { status: "error"; message: string }

const MAX_BYTES = 2 * 1024 * 1024

export function CodePreview({ asset, language }: { asset: Asset; language: string }) {
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
        if (cancelled) return
        // Auto-pretty JSON when it looks parseable; leave other languages untouched.
        if (language === "json") {
          try {
            const parsed = JSON.parse(text)
            setState({ status: "ready", text: JSON.stringify(parsed, null, 2) })
            return
          } catch {
            /* malformed JSON; render verbatim */
          }
        }
        setState({ status: "ready", text })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        const message = err instanceof Error ? err.message : "Failed to load"
        setState({ status: "error", message })
      })
    return () => {
      cancelled = true
    }
  }, [asset.publicUrl, language])

  return (
    <div className="flex h-full w-full items-start justify-center overflow-hidden px-6 pb-24 pt-24">
      <div className="flex w-full max-w-[960px] flex-col overflow-hidden rounded-md bg-card ring-1 ring-border">
        {state.status === "loading" ? <CodeSkeleton /> : null}
        {state.status === "error" ? <CodeError message={state.message} /> : null}
        {state.status === "ready" ? (
          <SyntaxHighlighter
            language={language}
            style={oneDark}
            showLineNumbers
            customStyle={{
              margin: 0,
              padding: "1.25rem 1.25rem",
              background: "transparent",
              fontSize: "12px",
              lineHeight: 1.55,
              maxHeight: "calc(100vh - 12rem)",
              overflow: "auto",
            }}
            codeTagProps={{
              style: { fontFamily: "var(--font-mono), ui-monospace, monospace" },
            }}
            lineNumberStyle={{
              minWidth: "2.4em",
              paddingRight: "1em",
              color: "oklch(var(--foreground) / 0.25)",
              userSelect: "none",
              fontSize: "11px",
            }}
          >
            {state.text}
          </SyntaxHighlighter>
        ) : null}
      </div>
    </div>
  )
}

function CodeSkeleton() {
  return (
    <div className="flex flex-col gap-2 p-6">
      {Array.from({ length: 14 }).map((_, i) => (
        <span
          key={i}
          className="h-3 animate-pulse rounded bg-muted"
          style={{
            width: `${30 + Math.round(Math.sin(i) * 30 + 40)}%`,
            animationDelay: `${i * 35}ms`,
          }}
        />
      ))}
    </div>
  )
}

function CodeError({ message }: { message: string }) {
  return (
    <div className="px-6 py-12 text-center">
      <p className="font-mono text-[10.5px] uppercase tracking-[0.16em] text-muted-foreground">
        Couldn&apos;t load file
      </p>
      <p className="mt-2 text-[12px] text-muted-foreground">{message}</p>
    </div>
  )
}
