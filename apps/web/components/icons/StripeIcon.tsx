"use client"

import { cn } from "@/lib/utils"

interface StripeIconProps {
  className?: string
  size?: number
}

export function StripeIcon({ className, size = 20 }: StripeIconProps) {
  return (
    <div
      className={cn("flex items-center justify-center", className)}
      style={{ width: size, height: size }}
    >
      <svg
        className="h-full w-full"
        aria-hidden="true"
        focusable="false"
        fill="none"
        viewBox="100 100 312 312"
      >
        <path
          fill="#533afd"
          fillRule="evenodd"
          d="m120 392 272-57.683V120l-272 58.357z"
          clipRule="evenodd"
        />
      </svg>
    </div>
  )
}
