"use client"

import { motion } from "motion/react"

import { cn } from "@/lib/utils"

interface AuthGhostLogoProps {
  className?: string
  logoClassName?: string
  title?: string
  description?: string
}

export function AuthGhostLogo({
  className,
  logoClassName,
  title,
  description,
}: AuthGhostLogoProps) {
  return (
    <div className={cn("flex flex-col items-center gap-4", className)}>
      <motion.div
        animate={{ scale: [1, 1.18, 1] }}
        transition={{ duration: 1.2, repeat: Infinity, ease: "easeInOut" }}
      >
        <svg
          viewBox="0 0 640 640"
          width="48"
          height="48"
          className={cn(
            "mx-auto text-muted-foreground drop-shadow-[0_0_16px_rgba(139,140,246,0.35)]",
            logoClassName
          )}
          fill="currentColor"
        >
          <path
            d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z"
            fill="currentColor"
          />
          <ellipse
            cx="318.5"
            cy="282"
            rx="45.5"
            ry="101"
            fill="var(--background)"
          />
          <ellipse
            cx="457.5"
            cy="282"
            rx="45.5"
            ry="101"
            fill="var(--background)"
          />
        </svg>
      </motion.div>

      {(title || description) && (
        <div className="space-y-1 text-center">
          {title ? (
            <p className="text-sm font-medium text-foreground">{title}</p>
          ) : null}
          {description ? (
            <p className="text-sm text-muted-foreground">{description}</p>
          ) : null}
        </div>
      )}
    </div>
  )
}
