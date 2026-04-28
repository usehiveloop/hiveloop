"use client"

import * as React from "react"
import { $api } from "@/lib/api/hooks"

export type UploadAssetType = "avatar" | "org_logo" | "generic"

export interface UploadResult {
  /** Durable, CDN-served URL — store this. */
  publicUrl: string
  /** Storage key, useful for debugging. */
  key: string
  /** When the signed PUT URL expires. */
  expiresAt: string
  /** Max bytes allowed by policy at sign time. */
  maxSizeBytes: number
}

export interface UploadOptions {
  /** Forwarded to /v1/uploads/sign — required for org_logo, ignored otherwise. */
  orgId?: string
  /** Filename hint passed to the signer (influences the leaf in the storage key). */
  filename?: string
}

interface UseUploadResult {
  upload: (file: File, options?: UploadOptions) => Promise<UploadResult>
  isUploading: boolean
  error: Error | null
  reset: () => void
}

/**
 * Two-step pre-signed PUT upload to the public-assets bucket.
 *
 * 1. POST /v1/uploads/sign with content-type + size → presigned URL
 * 2. PUT the file directly to that URL with the required headers
 * 3. Resolves with the durable public CDN URL
 *
 * The hook is policy-aware via `assetType` — pick "avatar" (5MB images),
 * "org_logo" (5MB images, requires orgId), or "generic" (25MB images + pdf/txt).
 *
 * Files never traverse the API server.
 */
export function useUpload(assetType: UploadAssetType): UseUploadResult {
  const [isUploading, setIsUploading] = React.useState(false)
  const [error, setError] = React.useState<Error | null>(null)
  const sign = $api.useMutation("post", "/v1/uploads/sign")

  const upload = React.useCallback(
    async (file: File, options: UploadOptions = {}): Promise<UploadResult> => {
      setIsUploading(true)
      setError(null)
      try {
        const signedRaw = await sign.mutateAsync({
          body: {
            asset_type: assetType,
            content_type: file.type,
            size_bytes: file.size,
            filename: options.filename ?? file.name,
            ...(options.orgId ? { org_id: options.orgId } : {}),
          } as never,
        })
        const signed = signedRaw as {
          upload_url: string
          upload_method: string
          required_headers: Record<string, string>
          key: string
          public_url: string
          expires_at: string
          max_size_bytes: number
        }

        const putResponse = await fetch(signed.upload_url, {
          method: signed.upload_method,
          headers: signed.required_headers,
          body: file,
        })
        if (!putResponse.ok) {
          throw new Error(
            `Upload failed: ${putResponse.status} ${putResponse.statusText}`
          )
        }

        return {
          publicUrl: signed.public_url,
          key: signed.key,
          expiresAt: signed.expires_at,
          maxSizeBytes: signed.max_size_bytes,
        }
      } catch (err) {
        const wrapped = err instanceof Error ? err : new Error(String(err))
        setError(wrapped)
        throw wrapped
      } finally {
        setIsUploading(false)
      }
    },
    [assetType, sign]
  )

  const reset = React.useCallback(() => {
    setError(null)
  }, [])

  return { upload, isUploading, error, reset }
}
