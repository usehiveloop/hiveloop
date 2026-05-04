"use client"

import * as React from "react"
import { AnimatePresence, motion } from "motion/react"
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Cancel01Icon,
} from "@hugeicons/core-free-icons"

const ARROW_BTN =
  "flex size-9 items-center justify-center rounded-full bg-card text-muted-foreground ring-1 ring-border shadow-sm transition-all duration-150 hover:bg-accent hover:text-foreground hover:ring-foreground/20 disabled:pointer-events-none disabled:opacity-30"

export type LightboxMedia = {
  title: string
  description?: string
  /** File URL for image / video. Ignored when kind === "youtube". */
  src?: string
  kind?: "image" | "video" | "youtube"
  /** Required when kind === "youtube" — the bare video id (e.g. "vcOG0fRxdoc"). */
  videoId?: string
}

export function InstructionLightbox({
  items,
  index,
  onIndexChange,
  onClose,
}: {
  items: LightboxMedia[]
  index: number | null
  onIndexChange: (next: number) => void
  onClose: () => void
}) {
  const open = index !== null
  const current = open ? items[index] : null
  const total = items.length

  React.useEffect(() => {
    if (!open) return
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault()
        onClose()
      } else if (event.key === "ArrowLeft" && index !== null && index > 0) {
        onIndexChange(index - 1)
      } else if (event.key === "ArrowRight" && index !== null && index < total - 1) {
        onIndexChange(index + 1)
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [open, index, total, onClose, onIndexChange])

  const goPrev = () => {
    if (index !== null && index > 0) onIndexChange(index - 1)
  }
  const goNext = () => {
    if (index !== null && index < total - 1) onIndexChange(index + 1)
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop className="fixed inset-0 z-[60] bg-background/95 supports-backdrop-filter:bg-background/70 supports-backdrop-filter:backdrop-blur-xl data-open:animate-in data-open:fade-in-0 data-open:duration-200 data-closed:animate-out data-closed:fade-out-0" />
        <DialogPrimitive.Popup className="fixed inset-0 z-[60] flex flex-col text-foreground outline-none data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0">
          <DialogPrimitive.Title className="sr-only">
            {current?.title ?? "Instruction preview"}
          </DialogPrimitive.Title>

          {current && index !== null ? (
            <>
              <header className="absolute inset-x-0 top-0 z-20 flex items-center gap-4 border-b border-border bg-background/85 px-5 py-3 supports-backdrop-filter:bg-background/70 supports-backdrop-filter:backdrop-blur-md">
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <h2 className="truncate text-[15px] font-medium leading-tight">
                    {current.title}
                  </h2>
                  {current.description ? (
                    <p className="truncate text-[12px] text-muted-foreground">
                      {current.description}
                    </p>
                  ) : null}
                </div>

                {total > 1 ? (
                  <span className="hidden font-mono text-[10.5px] tabular-nums tracking-[0.04em] text-muted-foreground sm:inline">
                    {index + 1} <span className="text-muted-foreground/50">/</span> {total}
                  </span>
                ) : null}

                <button
                  type="button"
                  onClick={onClose}
                  className="ml-1 flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  aria-label="Close preview"
                >
                  <HugeiconsIcon icon={Cancel01Icon} size={14} />
                </button>
              </header>

              <div className="relative flex-1 overflow-hidden">
                {total > 1 ? (
                  <>
                    <button
                      type="button"
                      onClick={goPrev}
                      disabled={index === 0}
                      aria-label="Previous"
                      className={`${ARROW_BTN} absolute top-1/2 left-4 z-10 -translate-y-1/2`}
                    >
                      <HugeiconsIcon icon={ArrowLeft01Icon} size={16} />
                    </button>
                    <button
                      type="button"
                      onClick={goNext}
                      disabled={index === total - 1}
                      aria-label="Next"
                      className={`${ARROW_BTN} absolute top-1/2 right-4 z-10 -translate-y-1/2`}
                    >
                      <HugeiconsIcon icon={ArrowRight01Icon} size={16} />
                    </button>
                  </>
                ) : null}

                <AnimatePresence mode="wait" initial={false}>
                  <motion.div
                    key={index}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -4 }}
                    transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
                    className="absolute inset-0 flex items-center justify-center px-6 pt-20 pb-10 sm:px-16"
                  >
                    {current.kind === "youtube" && current.videoId ? (
                      <iframe
                        src={`https://www.youtube.com/embed/${current.videoId}?autoplay=1&rel=0&modestbranding=1`}
                        title={current.title}
                        allow="autoplay; encrypted-media; picture-in-picture"
                        allowFullScreen
                        className="aspect-video w-full max-w-4xl rounded-lg shadow-2xl ring-1 ring-border"
                      />
                    ) : current.kind === "video" ? (
                      <video
                        src={current.src}
                        controls
                        autoPlay
                        playsInline
                        className="max-h-full max-w-full rounded-lg shadow-2xl ring-1 ring-border"
                      />
                    ) : (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={current.src}
                        alt={current.title}
                        className="max-h-full max-w-full rounded-lg object-contain shadow-2xl ring-1 ring-border"
                      />
                    )}
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
