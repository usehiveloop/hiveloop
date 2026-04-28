"use client"

import * as React from "react"
import Masonry from "react-masonry-css"
import { ScrollArea } from "@/components/ui/scroll-area"

const HEIGHTS = [320, 460, 380, 540, 360, 500, 420, 600, 340, 480, 400, 560]

const PROMPTS = [
  "isometric studio",
  "neon retro grid",
  "soft gradient mesh",
  "ink wash botanical",
  "wireframe terrain",
  "duotone portrait",
]

const BREAKPOINTS = {
  default: 4,
  1280: 3,
  900: 2,
  600: 1,
}

export function GalleryView() {
  const images = React.useMemo(
    () =>
      HEIGHTS.map((height, index) => ({
        id: `img-${index}`,
        url: `https://picsum.photos/seed/agent-${index}/600/${height}`,
        title: `Generation ${String(index + 1).padStart(2, "0")}`,
        prompt: PROMPTS[index % PROMPTS.length],
      })),
    [],
  )

  return (
    <ScrollArea className="h-full">
      <div className="px-6 pt-6 pb-28">
        <Masonry
          breakpointCols={BREAKPOINTS}
          className="-ml-3 flex w-auto"
          columnClassName="space-y-3 pl-3"
        >
          {images.map((image) => (
            <figure
              key={image.id}
              className="group overflow-hidden rounded-lg border border-border/60 bg-muted/30"
            >
              <img
                src={image.url}
                alt={image.title}
                loading="lazy"
                className="block w-full transition-transform duration-300 group-hover:scale-[1.02]"
              />
              <figcaption className="flex items-center justify-between gap-2 px-2.5 py-1.5 text-[11px]">
                <span className="truncate text-foreground">{image.title}</span>
                <span className="truncate font-mono text-muted-foreground/70">{image.prompt}</span>
              </figcaption>
            </figure>
          ))}
        </Masonry>
      </div>
    </ScrollArea>
  )
}
