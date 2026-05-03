"use client"

import * as React from "react"
import {
  MediaCommunitySkin,
  MediaOutlet,
  MediaPlayer,
} from "@vidstack/react"

import "vidstack/styles/defaults.css"
import "vidstack/styles/community-skin/audio.css"

import type { Asset } from "../preview-router"

// Audio gets its own composition: the player itself is a thin slab, but
// stacked above is a hero card that gives the asset presence on a giant
// canvas. The card's gradient is generated from the filename hash so each
// track has a recognisable colour signature without us hand-curating one.
export function AudioPreview({ asset }: { asset: Asset }) {
  const [hueA, hueB] = React.useMemo(() => hashHues(asset.filename), [asset.filename])

  return (
    <div className="flex h-full w-full items-center justify-center px-6 py-24">
      <div className="flex w-full max-w-[680px] flex-col gap-8">
        <HeroCard filename={asset.filename} hueA={hueA} hueB={hueB} />

        <div
          className="overflow-hidden rounded-xl ring-1 ring-foreground/10"
          style={{
            ["--media-brand" as never]: "oklch(var(--primary))",
            ["--media-focus-ring" as never]: "0 0 0 3px oklch(var(--ring) / 0.35)",
          }}
        >
          <MediaPlayer
            title={asset.filename}
            src={asset.publicUrl}
            crossOrigin
            autoplay
            load="eager"
            view="audio"
          >
            <MediaOutlet />
            <MediaCommunitySkin />
          </MediaPlayer>
        </div>
      </div>
    </div>
  )
}

function HeroCard({ filename, hueA, hueB }: { filename: string; hueA: number; hueB: number }) {
  return (
    <div className="relative aspect-[16/7] overflow-hidden rounded-2xl ring-1 ring-foreground/10">
      <div
        aria-hidden
        className="absolute inset-0"
        style={{
          background: `radial-gradient(120% 100% at 18% 28%, oklch(0.62 0.16 ${hueA}) 0%, oklch(0.38 0.12 ${hueA}) 38%, oklch(0.18 0.06 ${hueB}) 78%, oklch(0.12 0.04 ${hueB}) 100%)`,
        }}
      />
      {/* Subtle noise to fight banding without adding a real texture file. */}
      <div
        aria-hidden
        className="absolute inset-0 opacity-[0.06] mix-blend-overlay"
        style={{
          backgroundImage:
            "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='120' height='120'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='2'/></filter><rect width='100%' height='100%' filter='url(%23n)' opacity='0.65'/></svg>\")",
        }}
      />
      <div className="relative flex h-full flex-col justify-end gap-1 p-7">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-white/55">
          Audio track
        </span>
        <h3 className="font-heading text-[28px] font-medium leading-[1.05] tracking-[-0.01em] text-white">
          {stripExtension(filename)}
        </h3>
      </div>
    </div>
  )
}

function stripExtension(name: string): string {
  const dot = name.lastIndexOf(".")
  return dot > 0 ? name.slice(0, dot) : name
}

// Two stable hues derived from the filename — enough variety for a session,
// deterministic so the same file renders the same way across reloads.
function hashHues(input: string): [number, number] {
  let h = 0
  for (let i = 0; i < input.length; i += 1) {
    h = (h * 31 + input.charCodeAt(i)) | 0
  }
  const a = Math.abs(h) % 360
  const b = (a + 70 + (Math.abs(h >> 8) % 80)) % 360
  return [a, b]
}
