"use client"

import React from "react"
import { Theme } from "./theme"

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "ghost" | "outline"
  size?: "sm" | "md" | "lg"
  theme: Theme
  children: React.ReactNode
  href?: string
}

export function Button({
  variant = "primary",
  size = "md",
  theme,
  children,
  href,
  className = "",
  ...props
}: ButtonProps) {
  const baseStyles =
    "inline-flex items-center justify-center rounded-md cursor-pointer font-semibold transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"

  const sizeStyles = {
    sm: "h-9 px-4 text-xs",
    md: "h-11 px-5 text-sm",
    lg: "h-12 px-8 text-sm",
  }

  const variantStyles = {
    primary: {
      backgroundColor: theme.primary,
      color: theme.primaryText,
    },
    secondary: {
      backgroundColor: theme.secondary,
      border: `1px solid ${theme.secondaryBorder}`,
      color: theme.secondaryText,
    },
    ghost: {
      backgroundColor: "transparent",
      color: theme.muted,
    },
    outline: {
      backgroundColor: "transparent",
      border: `1px solid ${theme.secondaryBorder}`,
      color: theme.text,
    },
  }

  const hoverStyles = {
    primary: "hover:scale-105",
    secondary: "hover:scale-105",
    ghost: "hover:underline hover:underline-offset-4",
    outline: "hover:bg-black/[0.03]",
  }

  const isDisabled = props.disabled
  const activeHoverStyles = isDisabled ? "" : hoverStyles[variant]
  const combinedClassName = `${baseStyles} ${sizeStyles[size]} ${activeHoverStyles} ${className}`

  const style = variantStyles[variant]

  if (href) {
    return (
      <a
        href={href}
        className={combinedClassName}
        style={style}
      >
        {children}
      </a>
    )
  }

  return (
    <button
      type="button"
      className={combinedClassName}
      style={style}
      {...props}
    >
      {children}
    </button>
  )
}
