"use client"

import * as React from "react"
import { Document, Page, pdfjs } from "react-pdf"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowLeft01Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"

import "react-pdf/dist/Page/TextLayer.css"
import "react-pdf/dist/Page/AnnotationLayer.css"

import type { Asset } from "../preview-router"

// pdf.js workers ship as a separate file; point at the unpkg CDN matching
// the version we resolved at install time. Doing this once at module load
// is fine — the import is already inside a dynamic-imported chunk.
if (typeof window !== "undefined" && !pdfjs.GlobalWorkerOptions.workerSrc) {
  pdfjs.GlobalWorkerOptions.workerSrc = `https://unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`
}

const PDF_OPTIONS = {
  cMapUrl: `https://unpkg.com/pdfjs-dist@${pdfjs.version}/cmaps/`,
  standardFontDataUrl: `https://unpkg.com/pdfjs-dist@${pdfjs.version}/standard_fonts/`,
}

export function PdfPreview({ asset }: { asset: Asset }) {
  const [numPages, setNumPages] = React.useState<number | null>(null)
  const [page, setPage] = React.useState(1)
  const [pageWidth, setPageWidth] = React.useState(720)
  const containerRef = React.useRef<HTMLDivElement | null>(null)

  React.useEffect(() => {
    if (!containerRef.current) return
    const obs = new ResizeObserver((entries) => {
      for (const entry of entries) {
        // Cap at 880 so the layout stays comfortable on ultrawide displays.
        const w = Math.min(880, Math.max(320, entry.contentRect.width - 32))
        setPageWidth(w)
      }
    })
    obs.observe(containerRef.current)
    return () => obs.disconnect()
  }, [])

  React.useEffect(() => {
    setPage(1)
    setNumPages(null)
  }, [asset.id])

  const goPrev = () => setPage((p) => Math.max(1, p - 1))
  const goNext = () => setPage((p) => Math.min(numPages ?? p, p + 1))

  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-5 px-6 pb-24 pt-24">
      <div
        ref={containerRef}
        className="flex w-full max-w-[920px] flex-1 items-start justify-center overflow-y-auto rounded-md bg-foreground/[0.02] py-6 ring-1 ring-foreground/10"
      >
        <Document
          file={asset.publicUrl}
          options={PDF_OPTIONS}
          onLoadSuccess={({ numPages: n }) => setNumPages(n)}
          loading={<DocumentLoading />}
          error={<DocumentError />}
          className="flex flex-col items-center gap-6"
        >
          <Page
            pageNumber={page}
            width={pageWidth}
            renderAnnotationLayer
            renderTextLayer
            loading={<DocumentLoading />}
            className="overflow-hidden rounded-sm shadow-lg shadow-black/40"
          />
        </Document>
      </div>

      {numPages && numPages > 1 ? (
        <div className="flex items-center gap-2 rounded-full bg-foreground/[0.05] px-2 py-1.5 ring-1 ring-foreground/10">
          <button
            type="button"
            onClick={goPrev}
            disabled={page === 1}
            aria-label="Previous page"
            className="flex size-7 items-center justify-center rounded-full text-foreground/80 transition-colors hover:bg-foreground/[0.08] disabled:pointer-events-none disabled:opacity-40"
          >
            <HugeiconsIcon icon={ArrowLeft01Icon} size={13} />
          </button>
          <span className="px-2 font-mono text-[11px] tabular-nums tracking-[0.04em] text-foreground/80">
            {page} <span className="text-foreground/30">/</span> {numPages}
          </span>
          <button
            type="button"
            onClick={goNext}
            disabled={page === numPages}
            aria-label="Next page"
            className="flex size-7 items-center justify-center rounded-full text-foreground/80 transition-colors hover:bg-foreground/[0.08] disabled:pointer-events-none disabled:opacity-40"
          >
            <HugeiconsIcon icon={ArrowRight01Icon} size={13} />
          </button>
        </div>
      ) : null}
    </div>
  )
}

function DocumentLoading() {
  return (
    <div className="flex h-72 w-[480px] animate-pulse items-center justify-center rounded-md bg-foreground/[0.04] font-mono text-[10px] uppercase tracking-[0.16em] text-foreground/40">
      Rendering page
    </div>
  )
}

function DocumentError() {
  return (
    <div className="flex max-w-md flex-col items-center gap-2 rounded-md bg-foreground/[0.04] px-6 py-8 text-center ring-1 ring-foreground/10">
      <span className="font-mono text-[10px] uppercase tracking-[0.16em] text-foreground/40">
        Couldn&apos;t render PDF
      </span>
      <p className="text-[12px] text-foreground/60">
        Try downloading the file and opening it locally.
      </p>
    </div>
  )
}
