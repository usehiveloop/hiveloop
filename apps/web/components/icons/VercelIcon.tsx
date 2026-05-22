"use client"

import { cn } from "@/lib/utils"

interface VercelIconProps {
  className?: string
  size?: number
}

export function VercelIcon({ className, size = 20 }: VercelIconProps) {
  return (
    <div
      className={cn("flex items-center justify-center", className)}
      style={{ width: size, height: size }}
    >
      <svg
        className="h-full w-full"
        aria-hidden="true"
        focusable="false"
        viewBox="0 0 256 222"
        preserveAspectRatio="xMidYMid"
      >
        <path d="m128 0 128 221.705H0z" />
      </svg>
    </div>
  )
}
