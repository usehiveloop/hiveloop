"use client"

import React from "react"
import { Theme } from "./theme"

interface CardProps {
  theme: Theme
  children: React.ReactNode
  className?: string
  padding?: "none" | "sm" | "md" | "lg"
}

export function Card({
  theme,
  children,
  className = "",
  padding = "md",
}: CardProps) {
  const paddingStyles = {
    none: "",
    sm: "p-4",
    md: "p-6",
    lg: "p-8",
  }

  return (
    <div
      className={`rounded-2xl border ${paddingStyles[padding]} ${className}`}
      style={{
        backgroundColor: theme.secondary,
        borderColor: theme.secondaryBorder,
      }}
    >
      {children}
    </div>
  )
}
