"use client"

import * as React from "react"
import Image from "next/image"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Image01Icon,
  Loading03Icon,
  PencilIcon,
} from "@hugeicons/core-free-icons"
import { useUpload, type UploadAssetType } from "@/hooks/use-upload"

interface ImagePickerProps {
  /** Current image URL (CDN public URL once uploaded). */
  value?: string
  /** Fires with the public CDN URL after a successful upload. */
  onChange?: (url: string | undefined) => void
  /** Drives the upload policy (size cap, allowed content types, key prefix). */
  assetType: UploadAssetType
  /** Required when `assetType === "org_logo"`. Ignored otherwise. */
  orgId?: string
  /** Square size in pixels. */
  size?: number
  /** Single-character glyph shown when there's no image yet (sr-only). */
  fallback?: string
  /** Custom aria-label for the upload button. */
  ariaLabel?: string
}

export function ImagePicker({
  value,
  onChange,
  assetType,
  orgId,
  size = 72,
  fallback = "?",
  ariaLabel,
}: ImagePickerProps) {
  const inputRef = React.useRef<HTMLInputElement>(null)
  const { upload, isUploading } = useUpload(assetType)

  // Optimistic blob-URL preview shown while the PUT is in flight.
  const [pendingPreview, setPendingPreview] = React.useState<string | null>(null)
  const previousBlobUrl = React.useRef<string | null>(null)

  React.useEffect(() => {
    return () => {
      if (previousBlobUrl.current) URL.revokeObjectURL(previousBlobUrl.current)
    }
  }, [])

  async function handleFile(file: File | null) {
    if (!file) return

    if (previousBlobUrl.current) URL.revokeObjectURL(previousBlobUrl.current)
    const blobUrl = URL.createObjectURL(file)
    previousBlobUrl.current = blobUrl
    setPendingPreview(blobUrl)

    try {
      const result = await upload(file, orgId ? { orgId } : undefined)
      onChange?.(result.publicUrl)
    } catch (err) {
      const message = err instanceof Error ? err.message : "Upload failed"
      toast.error(`Couldn't upload image — ${message}`, {
        action: {
          label: "Retry",
          onClick: () => {
            void handleFile(file)
          },
        },
      })
    } finally {
      setPendingPreview(null)
      if (previousBlobUrl.current) {
        URL.revokeObjectURL(previousBlobUrl.current)
        previousBlobUrl.current = null
      }
    }
  }

  function open() {
    if (isUploading) return
    inputRef.current?.click()
  }

  const displayed = pendingPreview ?? value
  const buttonLabel = ariaLabel ?? (displayed ? "Replace image" : "Upload image")

  return (
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      <button
        type="button"
        onClick={open}
        aria-label={buttonLabel}
        aria-busy={isUploading}
        disabled={isUploading}
        className="group relative flex h-full w-full items-center justify-center overflow-hidden rounded-2xl border border-border bg-muted/40 transition-colors hover:border-primary/40 hover:bg-muted/70 disabled:cursor-progress"
      >
        {displayed ? (
          <Image
            src={displayed}
            alt=""
            fill
            sizes={`${size}px`}
            className="object-cover"
            unoptimized
          />
        ) : (
          <>
            <HugeiconsIcon
              icon={Image01Icon}
              strokeWidth={1.5}
              className="size-6 text-muted-foreground/60 transition-colors group-hover:text-muted-foreground"
              aria-hidden
            />
            <span className="sr-only">{fallback}</span>
          </>
        )}

        {isUploading ? (
          <span className="absolute inset-0 flex items-center justify-center bg-background/60 backdrop-blur-[1px]">
            <HugeiconsIcon
              icon={Loading03Icon}
              strokeWidth={2}
              className="size-5 animate-spin text-foreground"
            />
          </span>
        ) : null}
      </button>

      <button
        type="button"
        onClick={open}
        disabled={isUploading}
        aria-label="Change image"
        className="absolute -right-1 -bottom-1 flex size-6 items-center justify-center rounded-full border border-border bg-background text-muted-foreground shadow-sm transition-colors hover:border-primary/40 hover:text-foreground disabled:cursor-progress"
      >
        <HugeiconsIcon icon={PencilIcon} strokeWidth={2} className="size-3" />
      </button>

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(event) => handleFile(event.target.files?.[0] ?? null)}
      />
    </div>
  )
}
