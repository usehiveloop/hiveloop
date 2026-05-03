"use client"

import * as React from "react"
import type { Asset } from "../preview-router"

// Click toggles a 1x ↔ 2x zoom. Pan-on-drag while zoomed. Pure CSS, no deps.
export function ImagePreview({ asset }: { asset: Asset }) {
  const [zoomed, setZoomed] = React.useState(false)
  const [origin, setOrigin] = React.useState({ x: 50, y: 50 })

  return (
    <div
      className="flex h-full w-full items-center justify-center overflow-hidden px-12 py-20"
      onClick={(e) => {
        const target = e.currentTarget.getBoundingClientRect()
        setOrigin({
          x: ((e.clientX - target.left) / target.width) * 100,
          y: ((e.clientY - target.top) / target.height) * 100,
        })
        setZoomed((z) => !z)
      }}
      onMouseMove={(e) => {
        if (!zoomed) return
        const target = e.currentTarget.getBoundingClientRect()
        setOrigin({
          x: ((e.clientX - target.left) / target.width) * 100,
          y: ((e.clientY - target.top) / target.height) * 100,
        })
      }}
      style={{ cursor: zoomed ? "zoom-out" : "zoom-in" }}
    >
      {/* The hairline frame catches the eye when the image has soft edges or
          a transparent background — a faint envelope that says "this is the
          subject," not a chrome border that competes with it. */}
      <div className="relative max-h-full max-w-full">
        <span className="pointer-events-none absolute -inset-px rounded-md ring-1 ring-border" />
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={asset.publicUrl}
          alt={asset.filename}
          draggable={false}
          className="block max-h-[calc(100vh-9rem)] max-w-[min(1200px,calc(100vw-6rem))] select-none rounded-md object-contain transition-transform duration-300 ease-[cubic-bezier(0.16,1,0.3,1)]"
          style={{
            transform: zoomed ? "scale(2)" : "scale(1)",
            transformOrigin: `${origin.x}% ${origin.y}%`,
          }}
        />
      </div>
    </div>
  )
}
