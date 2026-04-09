"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Search01Icon,
  RefreshIcon,
  Delete02Icon,
  Settings01Icon,
  MoreHorizontalIcon,
  Loading03Icon,
  ArrowUp01Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

/* ------------------------------------------------------------------ */
/*  Static data                                                       */
/* ------------------------------------------------------------------ */

interface KnowledgeSource {
  id: string
  name: string
  type: string
  icon: string
  status: "synced" | "syncing" | "error" | "pending"
  documents: number
  vectors: number
  chunks: number
  lastSynced: string | null
  schedule: string
  description: string
}

const sources: KnowledgeSource[] = [
  {
    id: "src_1",
    name: "ziraloop/docs",
    type: "GitHub",
    icon: "github",
    status: "synced",
    documents: 142,
    vectors: 18_430,
    chunks: 3_847,
    lastSynced: "2 min ago",
    schedule: "Every 6h",
    description: "Main documentation repository — markdown files from the docs/ directory",
  },
  {
    id: "src_2",
    name: "API Reference",
    type: "URL",
    icon: "url",
    status: "synced",
    documents: 86,
    vectors: 9_210,
    chunks: 1_920,
    lastSynced: "1 hour ago",
    schedule: "Every 12h",
    description: "Scraped API reference pages from docs.ziraloop.com/api",
  },
  {
    id: "src_3",
    name: "Engineering Wiki",
    type: "Notion",
    icon: "notion",
    status: "syncing",
    documents: 324,
    vectors: 41_870,
    chunks: 8_740,
    lastSynced: null,
    schedule: "Every 24h",
    description: "Internal engineering wiki covering architecture, runbooks, and onboarding",
  },
  {
    id: "src_4",
    name: "#support-tickets",
    type: "Slack",
    icon: "slack",
    status: "synced",
    documents: 1_208,
    vectors: 52_140,
    chunks: 10_890,
    lastSynced: "30 min ago",
    schedule: "Every 1h",
    description: "Customer support threads and resolutions from the #support-tickets channel",
  },
  {
    id: "src_5",
    name: "SUPPORT board",
    type: "Jira",
    icon: "jira",
    status: "error",
    documents: 567,
    vectors: 22_310,
    chunks: 4_660,
    lastSynced: "3 hours ago",
    schedule: "Every 6h",
    description: "Jira issues from the SUPPORT project — tickets, comments, and resolutions",
  },
  {
    id: "src_6",
    name: "Product Knowledge Base",
    type: "Slab",
    icon: "slab",
    status: "synced",
    documents: 89,
    vectors: 7_640,
    chunks: 1_595,
    lastSynced: "15 min ago",
    schedule: "Every 12h",
    description: "Product specs, feature briefs, and design docs on Slab",
  },
  {
    id: "src_7",
    name: "Onboarding Docs",
    type: "Google Docs",
    icon: "gdocs",
    status: "pending",
    documents: 0,
    vectors: 0,
    chunks: 0,
    lastSynced: null,
    schedule: "Every 24h",
    description: "New hire onboarding documents from Google Drive",
  },
  {
    id: "src_8",
    name: "Help Center Articles",
    type: "Confluence",
    icon: "confluence",
    status: "synced",
    documents: 213,
    vectors: 15_920,
    chunks: 3_325,
    lastSynced: "45 min ago",
    schedule: "Every 6h",
    description: "Public-facing help center articles and troubleshooting guides",
  },
]

const totalDocuments = sources.reduce((sum, source) => sum + source.documents, 0)
const totalVectors = sources.reduce((sum, source) => sum + source.vectors, 0)

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

function formatNumber(num: number): string {
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}k`
  return num.toLocaleString()
}

interface SourceIconProps {
  type: string
  size?: "sm" | "md" | "lg"
}

function SourceIcon({ type, size = "md" }: SourceIconProps) {
  const icons: Record<string, { bg: string; label: string }> = {
    github: { bg: "bg-neutral-900 dark:bg-white", label: "GH" },
    url: { bg: "bg-blue-600", label: "URL" },
    notion: { bg: "bg-neutral-800 dark:bg-neutral-200", label: "N" },
    slack: { bg: "bg-purple-600", label: "SL" },
    jira: { bg: "bg-blue-500", label: "JI" },
    slab: { bg: "bg-emerald-600", label: "SB" },
    gdocs: { bg: "bg-blue-500", label: "GD" },
    confluence: { bg: "bg-blue-700", label: "CF" },
  }
  const config = icons[type] ?? { bg: "bg-muted", label: "?" }
  const sizeClasses = size === "lg" ? "h-10 w-10 text-xs rounded-xl" : size === "sm" ? "h-6 w-6 text-[9px] rounded-md" : "h-8 w-8 text-[10px] rounded-lg"

  return (
    <span className={`flex items-center justify-center ${config.bg} ${sizeClasses} font-bold text-white dark:text-black shrink-0`}>
      {config.label}
    </span>
  )
}

interface StatusBadgeProps {
  status: KnowledgeSource["status"]
}

function StatusBadge({ status }: StatusBadgeProps) {
  if (status === "synced") {
    return (
      <span className="inline-flex items-center gap-1.5 text-[11px] text-green-600 dark:text-green-400">
        <span className="h-1.5 w-1.5 rounded-full bg-green-500" />
        Synced
      </span>
    )
  }
  if (status === "syncing") {
    return (
      <span className="inline-flex items-center gap-1.5 text-[11px] text-blue-600 dark:text-blue-400">
        <HugeiconsIcon icon={Loading03Icon} size={10} className="animate-spin" />
        Syncing
      </span>
    )
  }
  if (status === "error") {
    return (
      <span className="inline-flex items-center gap-1.5 text-[11px] text-destructive">
        <span className="h-1.5 w-1.5 rounded-full bg-destructive" />
        Error
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-[11px] text-muted-foreground">
      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/30" />
      Pending
    </span>
  )
}

/* ------------------------------------------------------------------ */
/*  Page                                                              */
/* ------------------------------------------------------------------ */

export default function KnowledgeHomeCardGrid() {
  const [search, setSearch] = useState("")

  const filtered = useMemo(() => {
    if (!search.trim()) return sources
    const query = search.toLowerCase()
    return sources.filter(
      (source) =>
        source.name.toLowerCase().includes(query) ||
        source.type.toLowerCase().includes(query) ||
        source.description.toLowerCase().includes(query),
    )
  }, [search])

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      {/* Header */}
      <div className="flex items-center justify-between mb-2">
        <h1 className="font-heading text-xl font-semibold text-foreground">Knowledge</h1>
        <Button size="default">
          <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
          Add source
        </Button>
      </div>
      <p className="text-sm text-muted-foreground mb-6">
        Your agents pull context from these sources during conversations.
      </p>

      {/* Summary banner */}
      <div className="flex items-center gap-6 rounded-xl bg-muted/50 px-5 py-4 mb-6">
        <div>
          <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/50 block">Sources</span>
          <span className="font-mono text-lg font-semibold tabular-nums text-foreground">{sources.length}</span>
        </div>
        <Separator orientation="vertical" className="h-8" />
        <div>
          <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/50 block">Documents</span>
          <span className="font-mono text-lg font-semibold tabular-nums text-foreground">{formatNumber(totalDocuments)}</span>
        </div>
        <Separator orientation="vertical" className="h-8" />
        <div>
          <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/50 block">Vectors</span>
          <span className="font-mono text-lg font-semibold tabular-nums text-primary">{formatNumber(totalVectors)}</span>
        </div>
        <div className="flex-1" />
        <div className="hidden sm:flex items-center gap-0.5">
          {sources.map((source) => (
            <span
              key={source.id}
              className={`h-2 w-2 rounded-full ${
                source.status === "synced"
                  ? "bg-green-500"
                  : source.status === "syncing"
                    ? "bg-blue-500 animate-pulse"
                    : source.status === "error"
                      ? "bg-destructive"
                      : "bg-muted-foreground/20"
              }`}
            />
          ))}
        </div>
      </div>

      {/* Search */}
      <div className="relative mb-6 max-w-sm">
        <HugeiconsIcon
          icon={Search01Icon}
          size={16}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search sources..."
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="pl-9"
        />
      </div>

      {/* Card grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {filtered.map((source) => (
          <div
            key={source.id}
            className="group rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer"
          >
            {/* Card header */}
            <div className="flex items-start justify-between mb-3">
              <div className="flex items-center gap-3 min-w-0">
                <SourceIcon type={source.icon} size="lg" />
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-foreground truncate">{source.name}</p>
                  <div className="flex items-center gap-2 mt-0.5">
                    <Badge variant="secondary" className="text-[10px]">{source.type}</Badge>
                    <StatusBadge status={source.status} />
                  </div>
                </div>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger className="flex items-center justify-center h-7 w-7 rounded-lg transition-colors hover:bg-muted outline-none opacity-0 group-hover:opacity-100">
                  <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" sideOffset={4}>
                  <DropdownMenuGroup>
                    <DropdownMenuItem>
                      <HugeiconsIcon icon={RefreshIcon} size={16} className="text-muted-foreground" />
                      Sync now
                    </DropdownMenuItem>
                    <DropdownMenuItem>
                      <HugeiconsIcon icon={Settings01Icon} size={16} className="text-muted-foreground" />
                      Configure
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive">
                    <HugeiconsIcon icon={Delete02Icon} size={16} />
                    Disconnect
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {/* Description */}
            <p className="text-[13px] text-muted-foreground leading-relaxed mb-4 line-clamp-2">
              {source.description}
            </p>

            {/* Stats row */}
            <div className="flex items-center gap-4 text-[11px] font-mono tabular-nums text-muted-foreground">
              <span>{source.documents.toLocaleString()} docs</span>
              <span>{formatNumber(source.chunks)} chunks</span>
              <span>{formatNumber(source.vectors)} vectors</span>
            </div>

            {/* Footer */}
            <Separator className="my-3" />
            <div className="flex items-center justify-between text-[11px] text-muted-foreground">
              <span>{source.schedule}</span>
              <span>{source.lastSynced ? `Synced ${source.lastSynced}` : "Never synced"}</span>
            </div>
          </div>
        ))}

        {/* Add source card */}
        <button
          type="button"
          className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border p-8 transition-colors hover:border-primary hover:bg-muted/30 cursor-pointer"
        >
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-muted mb-3">
            <HugeiconsIcon icon={Add01Icon} size={18} className="text-muted-foreground" />
          </div>
          <p className="text-sm font-medium text-foreground">Add a source</p>
          <p className="text-[13px] text-muted-foreground mt-0.5">Connect docs, repos, or tools</p>
        </button>
      </div>

      {filtered.length === 0 && (
        <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
          No sources match your search
        </div>
      )}
    </div>
  )
}
