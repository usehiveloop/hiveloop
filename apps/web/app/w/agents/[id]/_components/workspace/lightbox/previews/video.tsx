"use client"

import * as React from "react"
import {
  MediaCommunitySkin,
  MediaOutlet,
  MediaPlayer,
} from "@vidstack/react"

// Vidstack ships its own stylesheet for the community skin. Importing here
// keeps the CSS in this lazy chunk so it never lands in the initial bundle.
import "vidstack/styles/defaults.css"
import "vidstack/styles/community-skin/video.css"

import type { Asset } from "../preview-router"

export function VideoPreview({ asset }: { asset: Asset }) {
  return (
    <div className="flex h-full w-full items-center justify-center px-12 py-20">
      <div
        className="relative w-full max-w-[min(1200px,calc(100vw-6rem))] overflow-hidden rounded-md ring-1 ring-foreground/10"
        // The vidstack community skin reads colour from `--media-brand` for
        // the progress fill, time-slider thumb and active button states. We
        // route it through our app's primary so the player feels native.
        style={{
          ["--media-brand" as never]: "oklch(var(--primary))",
          ["--media-focus-ring" as never]: "0 0 0 3px oklch(var(--ring) / 0.35)",
        }}
      >
        <MediaPlayer
          title={asset.filename}
          src={asset.publicUrl}
          aspectRatio={16 / 9}
          playsInline
          autoplay
          load="eager"
        >
          <MediaOutlet />
          <MediaCommunitySkin />
        </MediaPlayer>
      </div>
    </div>
  )
}
