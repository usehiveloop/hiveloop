"use client"

import * as React from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Cancel01Icon,
  Copy01Icon,
  Download01Icon,
  Tick01Icon,
} from "@hugeicons/core-free-icons"
import type { components } from "@/lib/api/schema"
import { PreviewRouter, type Asset } from "./preview-router"

type ApiAsset = components["schemas"]["assetListItem"]

const ARROW_BTN =
  "flex size-9 items-center justify-center rounded-full bg-foreground/[0.04] text-foreground/70 ring-1 ring-foreground/10 transition-all duration-150 hover:bg-foreground/[0.08] hover:text-foreground hover:ring-foreground/20 disabled:pointer-events-none disabled:opacity-30"

export function Lightbox({
  assets,
  index,
  onIndexChange,
  onClose,
}: {
  assets: ApiAsset[]
  index: number | null
  onIndexChange: (next: number | null) => void
  onClose: () => void
}) {
  const open = index !== null
  const current = open ? assets[index] : null

  // Idle-tracking for the auto-hiding control rail. Mouse motion or any input
  // resets the timer; after 2s of stillness during media playback, the chrome
  // fades out so the asset gets the full stage.
  const [chromeVisible, setChromeVisible] = React.useState(true)
  const idleTimer = React.useRef<number | null>(null)
  const wakeChrome = React.useCallback(() => {
    setChromeVisible(true)
    if (idleTimer.current) window.clearTimeout(idleTimer.current)
    idleTimer.current = window.setTimeout(() => setChromeVisible(false), 2400)
  }, [])
  React.useEffect(() => {
    if (!open) return
    wakeChrome()
    return () => {
      if (idleTimer.current) window.clearTimeout(idleTimer.current)
    }
  }, [open, index, wakeChrome])

  // Keyboard nav. Esc → close, ←/→ → prev/next.
  React.useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault()
        onClose()
      } else if (e.key === "ArrowLeft" && index !== null && index > 0) {
        onIndexChange(index - 1)
      } else if (e.key === "ArrowRight" && index !== null && index < assets.length - 1) {
        onIndexChange(index + 1)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [open, index, assets.length, onClose, onIndexChange])

  const goPrev = () => {
    if (index !== null && index > 0) onIndexChange(index - 1)
  }
  const goNext = () => {
    if (index !== null && index < assets.length - 1) onIndexChange(index + 1)
  }

  const total = assets.length

  return (
    <DialogPrimitive.Root open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          className="fixed inset-0 z-[60] bg-[oklch(0.07_0.005_30/0.92)] supports-backdrop-filter:backdrop-blur-xl data-open:animate-in data-open:fade-in-0 data-open:duration-200 data-closed:animate-out data-closed:fade-out-0"
        />
        <DialogPrimitive.Popup
          onMouseMove={wakeChrome}
          onPointerDown={wakeChrome}
          className="fixed inset-0 z-[60] flex flex-col text-foreground outline-none data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0"
        >
          <DialogPrimitive.Title className="sr-only">Asset preview</DialogPrimitive.Title>

          {current ? (
            <>
              <Header
                asset={current}
                index={index!}
                total={total}
                visible={chromeVisible}
                onClose={onClose}
              />

              <div className="relative flex-1 overflow-hidden">
                {/* Prev / next arrows live above the asset, vertically centered. */}
                {total > 1 ? (
                  <AnimatePresence>
                    {chromeVisible ? (
                      <>
                        <motion.button
                          key="prev"
                          type="button"
                          onClick={goPrev}
                          disabled={index === 0}
                          aria-label="Previous asset"
                          initial={{ opacity: 0, x: -6 }}
                          animate={{ opacity: 1, x: 0 }}
                          exit={{ opacity: 0, x: -6 }}
                          transition={{ duration: 0.16, ease: [0.16, 1, 0.3, 1] }}
                          className={`${ARROW_BTN} absolute left-4 top-1/2 z-10 -translate-y-1/2`}
                        >
                          <HugeiconsIcon icon={ArrowLeft01Icon} size={16} />
                        </motion.button>
                        <motion.button
                          key="next"
                          type="button"
                          onClick={goNext}
                          disabled={index === total - 1}
                          aria-label="Next asset"
                          initial={{ opacity: 0, x: 6 }}
                          animate={{ opacity: 1, x: 0 }}
                          exit={{ opacity: 0, x: 6 }}
                          transition={{ duration: 0.16, ease: [0.16, 1, 0.3, 1] }}
                          className={`${ARROW_BTN} absolute right-4 top-1/2 z-10 -translate-y-1/2`}
                        >
                          <HugeiconsIcon icon={ArrowRight01Icon} size={16} />
                        </motion.button>
                      </>
                    ) : null}
                  </AnimatePresence>
                ) : null}

                <AnimatePresence mode="wait" initial={false}>
                  <motion.div
                    key={current.id}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -4 }}
                    transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
                    className="absolute inset-0 flex items-center justify-center"
                  >
                    <PreviewRouter asset={toPreviewAsset(current)} />
                  </motion.div>
                </AnimatePresence>
              </div>
            </>
          ) : null}
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function toPreviewAsset(a: ApiAsset): Asset {
  return {
    id: a.id ?? "",
    filename: a.filename ?? "",
    path: a.path ?? "",
    publicUrl: a.public_url ?? "",
    contentType: a.content_type ?? "application/octet-stream",
    bytes: a.bytes ?? 0,
    createdAt: a.created_at ?? "",
  }
}

/* ────────────────────────────────────────────────────────────────── */

function Header({
  asset,
  index,
  total,
  visible,
  onClose,
}: {
  asset: ApiAsset
  index: number
  total: number
  visible: boolean
  onClose: () => void
}) {
  const filename = asset.filename ?? ""
  const path = asset.path ?? ""
  const publicUrl = asset.public_url ?? ""

  const [copied, setCopied] = React.useState(false)
  const handleCopy = async () => {
    if (!publicUrl) return
    try {
      await navigator.clipboard.writeText(publicUrl)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1400)
    } catch {
      /* clipboard blocked, ignore */
    }
  }

  const segments = path ? path.split("/") : []

  return (
    <AnimatePresence>
      {visible ? (
        <motion.header
          key="header"
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -6 }}
          transition={{ duration: 0.16, ease: [0.16, 1, 0.3, 1] }}
          className="absolute inset-x-0 top-0 z-20 flex items-center gap-4 border-b border-foreground/10 bg-[oklch(0.06_0.005_30/0.6)] px-5 py-3 supports-backdrop-filter:backdrop-blur-md"
        >
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <h2 className="truncate font-heading text-[15px] font-medium leading-tight text-foreground/95">
              {filename}
            </h2>
            <div className="flex items-center gap-1.5 font-mono text-[10.5px] tracking-[0.04em] text-foreground/45">
              <span>Home</span>
              {segments.length > 0 ? (
                <>
                  {segments.map((seg, i) => (
                    <React.Fragment key={`${seg}-${i}`}>
                      <span className="text-foreground/25">/</span>
                      <span>{seg}</span>
                    </React.Fragment>
                  ))}
                </>
              ) : null}
              <span className="text-foreground/25">/</span>
              <span className="text-foreground/70">{filename}</span>
            </div>
          </div>

          <span className="hidden font-mono text-[10.5px] tabular-nums tracking-[0.04em] text-foreground/40 sm:inline">
            {index + 1} <span className="text-foreground/20">/</span> {total}
          </span>

          <div className="flex items-center gap-1">
            <a
              href={publicUrl || "#"}
              download={filename}
              className="flex size-8 items-center justify-center rounded-md text-foreground/70 transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
              aria-label="Download asset"
              title="Download"
            >
              <HugeiconsIcon icon={Download01Icon} size={14} />
            </a>
            <button
              type="button"
              onClick={handleCopy}
              className="flex size-8 items-center justify-center rounded-md text-foreground/70 transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
              aria-label="Copy public URL"
              title={copied ? "Copied" : "Copy URL"}
            >
              <HugeiconsIcon icon={copied ? Tick01Icon : Copy01Icon} size={14} />
            </button>
            <button
              type="button"
              onClick={onClose}
              className="ml-1 flex size-8 items-center justify-center rounded-md text-foreground/70 transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
              aria-label="Close preview"
              title="Close"
            >
              <HugeiconsIcon icon={Cancel01Icon} size={14} />
            </button>
          </div>
        </motion.header>
      ) : null}
    </AnimatePresence>
  )
}
