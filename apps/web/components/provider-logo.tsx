"use client"

import { useState } from "react"
import Image from "next/image"
import { cn } from "@/lib/utils"

const sizeClasses: Record<number, string> = {
  16: "size-4",
  20: "size-5",
  24: "size-6",
  28: "size-7",
  32: "size-8",
  40: "size-10",
  48: "size-12",
}

interface ProviderLogoProps {
  provider: string
  size?: number
  className?: string
}

export function ProviderLogo({ provider, size = 20, className }: ProviderLogoProps) {
  const sizeClass = sizeClasses[size] ?? "size-5"
  const [failed, setFailed] = useState(false)

  if (failed || !provider) {
    return (
      <div className={cn("shrink-0 rounded-md bg-muted flex items-center justify-center text-[9px] font-bold text-muted-foreground", sizeClass, className)}>
        {(provider || "?").charAt(0).toUpperCase()}
      </div>
    )
  }

  return (
    <div className={cn("shrink-0 rounded-md bg-white p-0.5", sizeClass, className)}>
      <Image
        src={`/logos/${provider}.svg`}
        alt={provider}
        width={size}
        height={size}
        className="size-full object-contain"
        onError={() => setFailed(true)}
      />
    </div>
  )
}
