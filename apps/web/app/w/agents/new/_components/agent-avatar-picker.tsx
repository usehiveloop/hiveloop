"use client"

import * as React from "react"
import Image from "next/image"
import { HugeiconsIcon } from "@hugeicons/react"
import { Image01Icon, PencilIcon } from "@hugeicons/core-free-icons"

interface AgentAvatarPickerProps {
  size?: number
  fallback?: string
}

export function AgentAvatarPicker({ size = 72, fallback = "A" }: AgentAvatarPickerProps) {
  const inputRef = React.useRef<HTMLInputElement>(null)
  const [preview, setPreview] = React.useState<string | null>(null)
  const previousUrl = React.useRef<string | null>(null)

  React.useEffect(() => {
    return () => {
      if (previousUrl.current) URL.revokeObjectURL(previousUrl.current)
    }
  }, [])

  function handleFile(file: File | null) {
    if (!file) return
    if (previousUrl.current) URL.revokeObjectURL(previousUrl.current)
    const url = URL.createObjectURL(file)
    previousUrl.current = url
    setPreview(url)
  }

  function open() {
    inputRef.current?.click()
  }

  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <button
        type="button"
        onClick={open}
        aria-label={preview ? "Replace avatar" : "Upload avatar"}
        className="group relative flex h-full w-full items-center justify-center overflow-hidden rounded-2xl border border-border bg-muted/40 transition-colors hover:border-primary/40 hover:bg-muted/70"
      >
        {preview ? (
          <Image
            src={preview}
            alt="Agent avatar"
            fill
            sizes={`${size}px`}
            className="object-cover"
            unoptimized
          />
        ) : (
          <HugeiconsIcon
            icon={Image01Icon}
            strokeWidth={1.5}
            className="size-6 text-muted-foreground/60 transition-colors group-hover:text-muted-foreground"
            aria-hidden
          />
        )}
        {!preview ? (
          <span className="sr-only">{fallback}</span>
        ) : null}
      </button>

      <button
        type="button"
        onClick={open}
        aria-label="Change avatar"
        className="absolute -right-1 -bottom-1 flex size-6 items-center justify-center rounded-full border border-border bg-background text-muted-foreground shadow-sm transition-colors hover:border-primary/40 hover:text-foreground"
      >
        <HugeiconsIcon icon={PencilIcon} strokeWidth={2} className="size-3" />
      </button>

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(e) => handleFile(e.target.files?.[0] ?? null)}
      />
    </div>
  )
}
