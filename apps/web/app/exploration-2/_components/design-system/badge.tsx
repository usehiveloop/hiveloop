"use client"

import React from "react"
import { Theme } from "./theme"

interface BadgeProps {
  theme: Theme
  children: React.ReactNode
  variant?: "default" | "dot" | "outline"
  className?: string
}

export function Badge({ theme, children, variant = "default", className = "" }: BadgeProps) {
  if (variant === "dot") {
    return (
      <div
        className={`inline-flex items-center gap-2 rounded-full border px-4 py-1.5 text-xs font-medium backdrop-blur-sm ${className}`}
        style={{
          borderColor: theme.secondaryBorder,
          backgroundColor: theme.navBg,
          color: theme.muted,
        }}
      >
        <span className="relative flex h-2 w-2">
          <span
            className="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
            style={{ backgroundColor: theme.pillFrom }}
          />
          <span
            className="relative inline-flex h-2 w-2 rounded-full"
            style={{ backgroundColor: theme.pillFrom }}
          />
        </span>
        {children}
      </div>
    )
  }

  if (variant === "outline") {
    return (
      <span
        className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium ${className}`}
        style={{
          borderColor: theme.secondaryBorder,
          color: theme.muted,
        }}
      >
        {children}
      </span>
    )
  }

  return (
    <span
      className={`inline-flex items-center rounded-full px-3 py-1 text-xs font-medium ${className}`}
      style={{
        backgroundColor: theme.pillFrom + "15",
        color: theme.primary,
      }}
    >
      {children}
    </span>
  )
}
