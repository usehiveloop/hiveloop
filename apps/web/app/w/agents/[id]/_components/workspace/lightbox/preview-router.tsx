"use client"

import * as React from "react"
import dynamic from "next/dynamic"
import { ImagePreview } from "./previews/image"
import { TextPreview } from "./previews/text"
import { ArchivePreview } from "./previews/archive"

// Heavy previews are lazy-loaded so the initial Files view bundle stays small.
// react-pdf and vidstack pull pdfjs and a media stack; papaparse handles
// CSV parsing. None of these need to be in the bundle until a user actually
// opens a matching asset.
const VideoPreview = dynamic(() => import("./previews/video").then((m) => m.VideoPreview), {
  ssr: false,
  loading: PreviewSkeleton,
})
const AudioPreview = dynamic(() => import("./previews/audio").then((m) => m.AudioPreview), {
  ssr: false,
  loading: PreviewSkeleton,
})
const PdfPreview = dynamic(() => import("./previews/pdf").then((m) => m.PdfPreview), {
  ssr: false,
  loading: PreviewSkeleton,
})
const CsvPreview = dynamic(() => import("./previews/csv").then((m) => m.CsvPreview), {
  ssr: false,
  loading: PreviewSkeleton,
})
const CodePreview = dynamic(() => import("./previews/code").then((m) => m.CodePreview), {
  ssr: false,
  loading: PreviewSkeleton,
})

export type Asset = {
  id: string
  filename: string
  path: string
  publicUrl: string
  contentType: string
  bytes: number
  createdAt: string
}

export function PreviewRouter({ asset }: { asset: Asset }) {
  const kind = inferKind(asset)

  switch (kind) {
    case "image":
      return <ImagePreview asset={asset} />
    case "video":
      return <VideoPreview asset={asset} />
    case "audio":
      return <AudioPreview asset={asset} />
    case "pdf":
      return <PdfPreview asset={asset} />
    case "csv":
      return <CsvPreview asset={asset} />
    case "json":
      return <CodePreview asset={asset} language="json" />
    case "text":
      return <TextPreview asset={asset} />
    case "archive":
    default:
      return <ArchivePreview asset={asset} kind={kind} />
  }
}

export type AssetKind =
  | "image"
  | "video"
  | "audio"
  | "pdf"
  | "csv"
  | "json"
  | "text"
  | "archive"
  | "unknown"

function inferKind(a: Asset): AssetKind {
  const ct = (a.contentType || "").toLowerCase()
  const ext = a.filename.split(".").pop()?.toLowerCase() ?? ""

  if (ct.startsWith("image/") || ["png", "jpg", "jpeg", "webp", "gif", "svg", "avif"].includes(ext))
    return "image"
  if (ct.startsWith("video/") || ["mp4", "mov", "webm", "mkv"].includes(ext)) return "video"
  if (ct.startsWith("audio/") || ["mp3", "wav", "ogg", "flac", "m4a"].includes(ext)) return "audio"
  if (ct === "application/pdf" || ext === "pdf") return "pdf"
  if (ct === "text/csv" || ext === "csv") return "csv"
  if (ct === "application/json" || ext === "json") return "json"
  if (
    ct === "application/zip" ||
    ct === "application/x-zip-compressed" ||
    ct === "application/x-7z-compressed" ||
    ct === "application/x-tar" ||
    ct === "application/gzip" ||
    ["zip", "tar", "gz", "7z", "rar"].includes(ext)
  )
    return "archive"
  if (ct.startsWith("text/") || ["txt", "md", "log", "yml", "yaml", "ini", "toml"].includes(ext))
    return "text"
  return "unknown"
}

function PreviewSkeleton() {
  return (
    <div className="flex flex-col items-center gap-3 text-foreground/60">
      <div className="size-10 animate-spin rounded-full border-2 border-foreground/20 border-t-foreground/60" />
      <span className="font-mono text-[10.5px] uppercase tracking-[0.16em] text-foreground/40">
        Loading preview
      </span>
    </div>
  )
}
