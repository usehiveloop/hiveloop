"use client"

import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  BrowserIcon,
  FileEditIcon,
  Folder01Icon,
  Image01Icon,
  Pdf02Icon,
} from "@hugeicons/core-free-icons"
import { ScrollArea } from "@/components/ui/scroll-area"

type FileItem = {
  id: string
  name: string
  type: "folder" | "doc" | "image" | "pdf"
  size?: string
  modified: string
  itemCount?: number
}

const ITEMS: FileItem[] = [
  { id: "f1", name: "Q4 reports", type: "folder", itemCount: 12, modified: "2 days ago" },
  { id: "f2", name: "Meeting notes", type: "folder", itemCount: 47, modified: "5 hours ago" },
  { id: "f3", name: "Drafts", type: "folder", itemCount: 8, modified: "1 week ago" },
  { id: "d1", name: "Roadmap_2026.md", type: "doc", size: "12 KB", modified: "1 hour ago" },
  { id: "d2", name: "RFC-payments-v2.md", type: "doc", size: "34 KB", modified: "3 hours ago" },
  { id: "d3", name: "Customer interview — Northwind.md", type: "doc", size: "8 KB", modified: "Yesterday" },
  { id: "p1", name: "Invoice_template.pdf", type: "pdf", size: "180 KB", modified: "1 day ago" },
  { id: "p2", name: "Brand guidelines v3.pdf", type: "pdf", size: "4.2 MB", modified: "3 days ago" },
  { id: "i1", name: "Hero illustration.png", type: "image", size: "2.4 MB", modified: "3 days ago" },
  { id: "i2", name: "Architecture diagram.png", type: "image", size: "780 KB", modified: "1 week ago" },
]

export function FilesView() {
  return (
    <ScrollArea className="h-full">
      <div className="flex flex-col px-3 pb-28 pt-2">
        <div className="flex items-center gap-2 px-3 py-3 text-[13px]">
          <span className="cursor-pointer text-muted-foreground transition-colors hover:text-foreground">
            Home
          </span>
          <HugeiconsIcon icon={ArrowRight01Icon} size={11} className="text-muted-foreground/40" />
          <span className="font-medium text-foreground">Generated outputs</span>
        </div>

        <div className="grid grid-cols-[1fr_120px_140px] gap-3 px-3 pb-2 font-mono text-[10px] uppercase tracking-[1.2px] text-muted-foreground/50">
          <span>Name</span>
          <span>Size</span>
          <span>Modified</span>
        </div>

        <div className="flex flex-col gap-0.5">
          {ITEMS.map((item) => (
            <FileRow key={item.id} item={item} />
          ))}
        </div>
      </div>
    </ScrollArea>
  )
}

function FileRow({ item }: { item: FileItem }) {
  return (
    <button
      type="button"
      className="grid grid-cols-[1fr_120px_140px] items-center gap-3 rounded-lg px-3 py-2 text-left transition-colors hover:bg-muted/50"
    >
      <span className="flex min-w-0 items-center gap-3">
        <FileTypeIcon type={item.type} />
        <span className="truncate text-[13px] font-medium text-foreground">{item.name}</span>
      </span>
      <span className="text-[12px] text-muted-foreground">
        {item.type === "folder" ? `${item.itemCount} items` : item.size}
      </span>
      <span className="text-[12px] text-muted-foreground">{item.modified}</span>
    </button>
  )
}

function FileTypeIcon({ type }: { type: FileItem["type"] }) {
  const config: Record<FileItem["type"], { icon: typeof BrowserIcon; className: string }> = {
    folder: { icon: Folder01Icon, className: "bg-blue-500/10 text-blue-500" },
    doc: { icon: FileEditIcon, className: "bg-muted text-foreground" },
    image: { icon: Image01Icon, className: "bg-purple-500/10 text-purple-500" },
    pdf: { icon: Pdf02Icon, className: "bg-red-500/10 text-red-500" },
  }
  const { icon, className } = config[type]
  return (
    <span className={`flex size-9 shrink-0 items-center justify-center rounded-md ${className}`}>
      <HugeiconsIcon icon={icon} size={16} />
    </span>
  )
}
