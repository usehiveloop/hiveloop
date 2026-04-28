"use client"

import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BrowserIcon,
  Folder01Icon,
  Image01Icon,
} from "@hugeicons/core-free-icons"

export type View = "browser" | "gallery" | "files"

const ITEMS: { id: View; icon: typeof BrowserIcon; label: string }[] = [
  { id: "browser", icon: BrowserIcon, label: "Preview" },
  { id: "gallery", icon: Image01Icon, label: "Gallery" },
  { id: "files", icon: Folder01Icon, label: "Files" },
]

export function BottomBar({
  value,
  onChange,
}: {
  value: View
  onChange: (next: View) => void
}) {
  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-6 z-10 flex justify-center">
      <div className="pointer-events-auto flex items-center gap-1 rounded-full border border-border/60 bg-background/85 p-1 shadow-lg backdrop-blur-md">
        {ITEMS.map((item) => {
          const active = value === item.id
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => onChange(item.id)}
              aria-pressed={active}
              className={
                "flex h-9 items-center gap-1.5 rounded-full px-3 text-[12px] font-medium transition-colors " +
                (active
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:text-foreground")
              }
            >
              <HugeiconsIcon icon={item.icon} size={14} />
              <AnimatePresence initial={false}>
                {active ? (
                  <motion.span
                    initial={{ width: 0, opacity: 0 }}
                    animate={{ width: "auto", opacity: 1 }}
                    exit={{ width: 0, opacity: 0 }}
                    transition={{ duration: 0.18, ease: [0.32, 0.72, 0, 1] }}
                    className="overflow-hidden whitespace-nowrap"
                  >
                    {item.label}
                  </motion.span>
                ) : null}
              </AnimatePresence>
            </button>
          )
        })}
      </div>
    </div>
  )
}
