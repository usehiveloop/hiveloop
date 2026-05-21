"use client"

import React from "react"
import { Theme } from "./theme"

interface SwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  theme: Theme
  disabled?: boolean
  label?: string
}

export function Switch({ checked, onChange, theme, disabled = false, label }: SwitchProps) {
  return (
    <div className="flex items-center justify-between gap-3">
      {label && (
        <span
          className="text-sm font-medium"
          style={{ color: checked ? theme.text : theme.muted }}
        >
          {label}
        </span>
      )}
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className="relative h-5 w-9 rounded-full transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed shrink-0"
        style={{ backgroundColor: checked ? theme.pillFrom : theme.secondaryBorder }}
      >
        <span
          className="absolute top-0.5 h-4 w-4 rounded-full bg-white shadow-sm transition-all duration-200"
          style={{ left: checked ? "calc(100% - 18px)" : "2px" }}
        />
      </button>
    </div>
  )
}
