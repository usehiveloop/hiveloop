"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  BrowserIcon,
  FileEditIcon,
  FileMusicIcon,
  FileVideoIcon,
  FileZipIcon,
  Folder01Icon,
  Image01Icon,
  Pdf02Icon,
} from "@hugeicons/core-free-icons"
import { ScrollArea } from "@/components/ui/scroll-area"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type ApiAsset = components["schemas"]["assetListItem"]

type FolderEntry = { kind: "folder"; name: string; itemCount: number; latest: string }
type FileEntry = { kind: "file"; asset: ApiAsset }
type Entry = FolderEntry | FileEntry

type FileKind = "image" | "video" | "audio" | "pdf" | "zip" | "doc"

export function FilesView({ conversationId }: { conversationId?: string }) {
  const [folder, setFolder] = React.useState<string>("")

  const query = $api.useQuery(
    "get",
    "/v1/assets",
    {
      params: { query: { conversation_id: conversationId, limit: 200 } },
    },
    { enabled: Boolean(conversationId), refetchInterval: 5000 },
  )

  const assets = React.useMemo<ApiAsset[]>(() => query.data?.data ?? [], [query.data])

  const entries = React.useMemo<Entry[]>(() => buildEntries(assets, folder), [assets, folder])
  const isLoading = query.isLoading
  const isEmpty = !isLoading && assets.length === 0
  const isFolderEmpty = !isLoading && entries.length === 0 && assets.length > 0

  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col px-3 pb-28 pt-2">
        <Breadcrumb folder={folder} onNavigate={setFolder} />

        <div className="grid grid-cols-[1fr_120px_140px] gap-3 px-3 pb-2 font-mono text-[10px] uppercase tracking-[1.2px] text-muted-foreground/50">
          <span>Name</span>
          <span>Size</span>
          <span>Modified</span>
        </div>

        {isLoading ? <RowSkeletons /> : null}
        {isEmpty ? <EmptyState text="No files have been uploaded in this session yet." /> : null}
        {isFolderEmpty ? <EmptyState text="This folder is empty." /> : null}

        <div className="flex flex-col gap-0.5">
          {entries.map((entry, i) => (
            <Row
              key={entry.kind === "folder" ? `f:${entry.name}` : entry.asset.id}
              entry={entry}
              currentFolder={folder}
              onOpenFolder={(name) =>
                setFolder((prev) => (prev ? `${prev}/${name}` : name))
              }
              animationIndex={i}
            />
          ))}
        </div>
      </div>
    </ScrollArea>
  )
}

function buildEntries(assets: ApiAsset[], folder: string): Entry[] {
  const folderMap = new Map<string, FolderEntry>()
  const files: FileEntry[] = []

  const prefix = folder === "" ? "" : folder + "/"

  for (const a of assets) {
    const path = a.path ?? ""
    if (path === folder) {
      files.push({ kind: "file", asset: a })
      continue
    }
    if (folder === "" || path.startsWith(prefix)) {
      const remainder = folder === "" ? path : path.slice(prefix.length)
      if (!remainder) continue
      const childName = remainder.split("/")[0]
      const existing = folderMap.get(childName)
      const ts = a.updated_at ?? a.created_at ?? ""
      if (existing) {
        existing.itemCount += 1
        if (ts > existing.latest) existing.latest = ts
      } else {
        folderMap.set(childName, {
          kind: "folder",
          name: childName,
          itemCount: 1,
          latest: ts,
        })
      }
    }
  }

  const folders = Array.from(folderMap.values()).sort((a, b) => a.name.localeCompare(b.name))
  files.sort((a, b) => (b.asset.created_at ?? "").localeCompare(a.asset.created_at ?? ""))
  return [...folders, ...files]
}

function Breadcrumb({
  folder,
  onNavigate,
}: {
  folder: string
  onNavigate: (next: string) => void
}) {
  const segments = folder ? folder.split("/") : []

  return (
    <div className="flex items-center gap-2 px-3 py-3 text-[13px]">
      {folder ? (
        <button
          type="button"
          onClick={() => onNavigate(segments.slice(0, -1).join("/"))}
          className="flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
          aria-label="Up one level"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} size={12} />
        </button>
      ) : null}

      <button
        type="button"
        onClick={() => onNavigate("")}
        className={
          "transition-colors " +
          (folder
            ? "cursor-pointer text-muted-foreground hover:text-foreground"
            : "font-medium text-foreground")
        }
      >
        Home
      </button>

      {segments.map((seg, i) => {
        const target = segments.slice(0, i + 1).join("/")
        const isLast = i === segments.length - 1
        return (
          <React.Fragment key={target}>
            <HugeiconsIcon icon={ArrowRight01Icon} size={11} className="text-muted-foreground/40" />
            <button
              type="button"
              onClick={() => onNavigate(target)}
              className={
                "transition-colors " +
                (isLast
                  ? "font-medium text-foreground"
                  : "cursor-pointer text-muted-foreground hover:text-foreground")
              }
            >
              {seg}
            </button>
          </React.Fragment>
        )
      })}
    </div>
  )
}

function Row({
  entry,
  currentFolder,
  onOpenFolder,
  animationIndex,
}: {
  entry: Entry
  currentFolder: string
  onOpenFolder: (name: string) => void
  animationIndex: number
}) {
  const style = { animationDelay: `${Math.min(animationIndex, 10) * 18}ms` }

  if (entry.kind === "folder") {
    return (
      <button
        type="button"
        onClick={() => onOpenFolder(entry.name)}
        style={style}
        className="grid animate-in fade-in slide-in-from-bottom-1 grid-cols-[1fr_120px_140px] items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors duration-150 hover:bg-muted/50"
      >
        <span className="flex min-w-0 items-center gap-3">
          <KindBadge kind="folder" />
          <span className="truncate text-[13px] font-medium text-foreground">{entry.name}</span>
        </span>
        <span className="text-[12px] text-muted-foreground">{entry.itemCount} items</span>
        <span className="text-[12px] text-muted-foreground">{formatRelative(entry.latest)}</span>
      </button>
    )
  }

  const a = entry.asset
  const kind = inferKind(a.content_type ?? "", a.filename ?? "")
  const fullPath = currentFolder
    ? `${a.path}/${a.filename}`.replace(/^\/+/, "")
    : `${a.path ? a.path + "/" : ""}${a.filename ?? ""}`

  return (
    <a
      href={a.public_url ?? "#"}
      target="_blank"
      rel="noreferrer noopener"
      style={style}
      className="grid animate-in fade-in slide-in-from-bottom-1 grid-cols-[1fr_120px_140px] items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors duration-150 hover:bg-muted/50"
      title={fullPath}
    >
      <span className="flex min-w-0 items-center gap-3">
        <KindBadge kind={kind} />
        <span className="truncate text-[13px] font-medium text-foreground">{a.filename}</span>
      </span>
      <span className="text-[12px] text-muted-foreground">{formatBytes(a.bytes)}</span>
      <span className="text-[12px] text-muted-foreground">{formatRelative(a.created_at)}</span>
    </a>
  )
}

function KindBadge({ kind }: { kind: FileKind | "folder" }) {
  const config: Record<FileKind | "folder", { icon: typeof BrowserIcon; className: string }> = {
    folder: { icon: Folder01Icon, className: "bg-blue-500/10 text-blue-500" },
    image: { icon: Image01Icon, className: "bg-purple-500/10 text-purple-500" },
    video: { icon: FileVideoIcon, className: "bg-pink-500/10 text-pink-500" },
    audio: { icon: FileMusicIcon, className: "bg-amber-500/10 text-amber-500" },
    pdf: { icon: Pdf02Icon, className: "bg-red-500/10 text-red-500" },
    zip: { icon: FileZipIcon, className: "bg-emerald-500/10 text-emerald-500" },
    doc: { icon: FileEditIcon, className: "bg-muted text-foreground" },
  }
  const { icon, className } = config[kind]
  return (
    <span className={`flex size-9 shrink-0 items-center justify-center rounded-md ${className}`}>
      <HugeiconsIcon icon={icon} size={16} />
    </span>
  )
}

function inferKind(contentType: string, filename: string): FileKind {
  const ct = contentType.toLowerCase()
  if (ct.startsWith("image/")) return "image"
  if (ct.startsWith("video/")) return "video"
  if (ct.startsWith("audio/")) return "audio"
  if (ct === "application/pdf") return "pdf"
  if (
    ct === "application/zip" ||
    ct === "application/x-zip-compressed" ||
    ct === "application/x-7z-compressed" ||
    ct === "application/x-tar" ||
    ct === "application/gzip"
  )
    return "zip"
  const ext = filename.split(".").pop()?.toLowerCase() ?? ""
  if (["png", "jpg", "jpeg", "webp", "gif", "svg", "avif"].includes(ext)) return "image"
  if (["mp4", "mov", "webm", "mkv", "avi"].includes(ext)) return "video"
  if (["mp3", "wav", "ogg", "flac", "m4a"].includes(ext)) return "audio"
  if (ext === "pdf") return "pdf"
  if (["zip", "tar", "gz", "7z", "rar"].includes(ext)) return "zip"
  return "doc"
}

function formatBytes(bytes?: number): string {
  if (!bytes || bytes < 0) return "—"
  const units = ["B", "KB", "MB", "GB", "TB"]
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  return `${value.toFixed(value >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

function formatRelative(iso: string | undefined): string {
  if (!iso) return "—"
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return "—"
  const diff = Date.now() - t
  if (diff < 60_000) return "Just now"
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} hr ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days} day${days === 1 ? "" : "s"} ago`
  return new Date(iso).toLocaleDateString()
}

function RowSkeletons() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 5 }).map((_, i) => (
        <div
          key={i}
          className="grid grid-cols-[1fr_120px_140px] items-center gap-3 px-3 py-2"
        >
          <span className="flex items-center gap-3">
            <span className="size-9 shrink-0 animate-pulse rounded-md bg-muted/60" />
            <span className="h-3 w-40 animate-pulse rounded bg-muted/60" />
          </span>
          <span className="h-3 w-16 animate-pulse rounded bg-muted/60" />
          <span className="h-3 w-20 animate-pulse rounded bg-muted/60" />
        </div>
      ))}
    </div>
  )
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="mx-3 mt-4 flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-muted/20 px-6 py-10">
      <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <HugeiconsIcon icon={Folder01Icon} size={18} />
      </span>
      <p className="text-center text-[12px] text-muted-foreground">{text}</p>
    </div>
  )
}
