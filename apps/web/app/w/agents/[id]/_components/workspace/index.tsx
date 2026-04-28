"use client"

import * as React from "react"
import { AnimatePresence, motion } from "motion/react"
import { BottomBar, type View } from "./bottom-bar"
import { BrowserView } from "./browser-view"
import { FilesView } from "./files-view"
import { GalleryView } from "./gallery-view"

export function Workspace() {
  const [view, setView] = React.useState<View>("browser")

  return (
    <div className="relative h-full w-full overflow-hidden">
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={view}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -8 }}
          transition={{ duration: 0.18, ease: [0.32, 0.72, 0, 1] }}
          className="absolute inset-0"
        >
          {view === "browser" ? <BrowserView /> : null}
          {view === "gallery" ? <GalleryView /> : null}
          {view === "files" ? <FilesView /> : null}
        </motion.div>
      </AnimatePresence>

      <BottomBar value={view} onChange={setView} />
    </div>
  )
}
