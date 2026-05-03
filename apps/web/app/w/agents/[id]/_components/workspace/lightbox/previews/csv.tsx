"use client"

import * as React from "react"
import Papa from "papaparse"
import type { Asset } from "../preview-router"

const MAX_ROWS = 500

type ParseState =
  | { status: "loading" }
  | { status: "ready"; headers: string[]; rows: string[][]; total: number; truncated: boolean }
  | { status: "error"; message: string }

export function CsvPreview({ asset }: { asset: Asset }) {
  const [state, setState] = React.useState<ParseState>({ status: "loading" })

  React.useEffect(() => {
    let cancelled = false
    setState({ status: "loading" })
    fetch(asset.publicUrl)
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.text()
      })
      .then((text) => {
        if (cancelled) return
        const parsed = Papa.parse<string[]>(text, {
          skipEmptyLines: true,
        })
        if (parsed.errors.length > 0 && parsed.data.length === 0) {
          setState({ status: "error", message: parsed.errors[0]!.message })
          return
        }
        const all = parsed.data
        const [headerRow, ...bodyRows] = all
        const headers = headerRow ?? []
        const rows = bodyRows.slice(0, MAX_ROWS)
        setState({
          status: "ready",
          headers,
          rows,
          total: bodyRows.length,
          truncated: bodyRows.length > MAX_ROWS,
        })
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
      <div className="flex w-full max-w-[1100px] flex-col gap-3">
        <Meta state={state} />
        <div className="overflow-auto rounded-md bg-foreground/[0.02] ring-1 ring-foreground/10">
          {state.status === "loading" ? <TableSkeleton /> : null}
          {state.status === "error" ? <TableError message={state.message} /> : null}
          {state.status === "ready" ? <Table headers={state.headers} rows={state.rows} /> : null}
        </div>
      </div>
    </div>
  )
}

function Meta({ state }: { state: ParseState }) {
  if (state.status !== "ready") return null
  const cols = state.headers.length
  return (
    <div className="flex items-center gap-3 px-1 font-mono text-[10.5px] uppercase tracking-[0.14em] text-foreground/45">
      <span>
        {state.total.toLocaleString()} {state.total === 1 ? "row" : "rows"}
      </span>
      <span className="text-foreground/20">·</span>
      <span>
        {cols} {cols === 1 ? "col" : "cols"}
      </span>
      {state.truncated ? (
        <>
          <span className="text-foreground/20">·</span>
          <span className="text-foreground/65">
            showing first {MAX_ROWS.toLocaleString()}
          </span>
        </>
      ) : null}
    </div>
  )
}

function Table({ headers, rows }: { headers: string[]; rows: string[][] }) {
  return (
    <table className="min-w-full border-collapse text-[12px]">
      <thead className="sticky top-0 z-10 bg-[oklch(0.07_0.005_30/0.95)] backdrop-blur-md">
        <tr>
          <th className="border-b border-foreground/10 py-2 pl-4 pr-3 text-left font-mono text-[10px] font-normal uppercase tracking-[0.14em] text-foreground/45">
            #
          </th>
          {headers.map((h, i) => (
            <th
              key={i}
              className="border-b border-foreground/10 px-3 py-2 text-left font-mono text-[10px] font-normal uppercase tracking-[0.14em] text-foreground/55"
            >
              {h || `col_${i + 1}`}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row, ri) => (
          <tr key={ri} className="border-b border-foreground/[0.06] last:border-b-0 hover:bg-foreground/[0.03]">
            <td className="py-1.5 pl-4 pr-3 font-mono text-[10.5px] tabular-nums text-foreground/35">
              {ri + 1}
            </td>
            {row.map((cell, ci) => (
              <td key={ci} className="px-3 py-1.5 text-foreground/85">
                {cell}
              </td>
            ))}
            {/* Pad any short rows so the columns stay aligned. */}
            {row.length < headers.length
              ? Array.from({ length: headers.length - row.length }).map((_, k) => (
                  <td key={`pad-${k}`} className="px-3 py-1.5 text-foreground/30">
                    —
                  </td>
                ))
              : null}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function TableSkeleton() {
  return (
    <div className="flex flex-col gap-2 p-6">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="flex gap-2">
          {Array.from({ length: 5 }).map((_, j) => (
            <span
              key={j}
              className="h-3 flex-1 animate-pulse rounded bg-foreground/[0.06]"
              style={{ animationDelay: `${(i * 5 + j) * 30}ms` }}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

function TableError({ message }: { message: string }) {
  return (
    <div className="px-6 py-12 text-center">
      <p className="font-mono text-[10.5px] uppercase tracking-[0.16em] text-foreground/40">
        Couldn&apos;t parse CSV
      </p>
      <p className="mt-2 text-[12px] text-foreground/60">{message}</p>
    </div>
  )
}
