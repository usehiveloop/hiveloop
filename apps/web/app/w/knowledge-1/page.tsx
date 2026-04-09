"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Search01Icon,
  MoreHorizontalIcon,
  RefreshIcon,
  Delete02Icon,
  Settings01Icon,
  ArrowUp01Icon,
  Tick02Icon,
  Cancel01Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"

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
  lastSynced: string | null
  schedule: string
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
    lastSynced: "2 min ago",
    schedule: "Every 6 hours",
  },
  {
    id: "src_2",
    name: "API Reference",
    type: "URL",
    icon: "url",
    status: "synced",
    documents: 86,
    vectors: 9_210,
    lastSynced: "1 hour ago",
    schedule: "Every 12 hours",
  },
  {
    id: "src_3",
    name: "Engineering Wiki",
    type: "Notion",
    icon: "notion",
    status: "syncing",
    documents: 324,
    vectors: 41_870,
    lastSynced: null,
    schedule: "Every 24 hours",
  },
  {
    id: "src_4",
    name: "#support-tickets",
    type: "Slack",
    icon: "slack",
    status: "synced",
    documents: 1_208,
    vectors: 52_140,
    lastSynced: "30 min ago",
    schedule: "Every 1 hour",
  },
  {
    id: "src_5",
    name: "SUPPORT board",
    type: "Jira",
    icon: "jira",
    status: "error",
    documents: 567,
    vectors: 22_310,
    lastSynced: "3 hours ago",
    schedule: "Every 6 hours",
  },
  {
    id: "src_6",
    name: "Product Knowledge Base",
    type: "Slab",
    icon: "slab",
    status: "synced",
    documents: 89,
    vectors: 7_640,
    lastSynced: "15 min ago",
    schedule: "Every 12 hours",
  },
  {
    id: "src_7",
    name: "Onboarding Docs",
    type: "Google Docs",
    icon: "gdocs",
    status: "pending",
    documents: 0,
    vectors: 0,
    lastSynced: null,
    schedule: "Every 24 hours",
  },
  {
    id: "src_8",
    name: "Help Center Articles",
    type: "Confluence",
    icon: "confluence",
    status: "synced",
    documents: 213,
    vectors: 15_920,
    lastSynced: "45 min ago",
    schedule: "Every 6 hours",
  },
]

const totalDocuments = sources.reduce((sum, source) => sum + source.documents, 0)
const totalVectors = sources.reduce((sum, source) => sum + source.vectors, 0)
const syncedSources = sources.filter((source) => source.status === "synced").length
const errorSources = sources.filter((source) => source.status === "error").length

/* ------------------------------------------------------------------ */
/*  Helper components                                                 */
/* ------------------------------------------------------------------ */

function formatNumber(num: number): string {
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}k`
  return num.toLocaleString()
}

interface SourceIconProps {
  type: string
}

function SourceIcon({ type }: SourceIconProps) {
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
  return (
    <span className={`flex h-7 w-7 items-center justify-center rounded-lg ${config.bg} text-[10px] font-bold text-white dark:text-black`}>
      {config.label}
    </span>
  )
}

interface SyncStatusProps {
  status: KnowledgeSource["status"]
}

function SyncStatus({ status }: SyncStatusProps) {
  if (status === "synced") {
    return (
      <span className="relative flex h-2 w-2">
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-40" />
        <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
      </span>
    )
  }
  if (status === "syncing") {
    return <HugeiconsIcon icon={Loading03Icon} size={14} className="animate-spin text-blue-500" />
  }
  if (status === "error") {
    return (
      <span className="relative flex h-2 w-2">
        <span className="relative inline-flex h-2 w-2 rounded-full bg-destructive" />
      </span>
    )
  }
  return (
    <span className="relative flex h-2 w-2">
      <span className="relative inline-flex h-2 w-2 rounded-full bg-muted-foreground/30" />
    </span>
  )
}

interface SourceActionsProps {
  source: KnowledgeSource
}

function SourceActions({ source }: SourceActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
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
  )
}

/* ------------------------------------------------------------------ */
/*  Page                                                              */
/* ------------------------------------------------------------------ */

export default function KnowledgeDashboardPage() {
  const [search, setSearch] = useState("")

  const filtered = useMemo(() => {
    if (!search.trim()) return sources
    const query = search.toLowerCase()
    return sources.filter(
      (source) =>
        source.name.toLowerCase().includes(query) ||
        source.type.toLowerCase().includes(query),
    )
  }, [search])

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-heading text-xl font-semibold text-foreground">Knowledge</h1>
          <p className="text-sm text-muted-foreground mt-1">
            {sources.length} sources connected to your knowledge base
          </p>
        </div>
        <Button size="default">
          <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
          Add source
        </Button>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-8">
        <div className="flex flex-col rounded-xl border border-border p-4">
          <span className="text-xs text-muted-foreground">Total sources</span>
          <span className="font-mono text-2xl font-semibold tabular-nums text-foreground mt-1">
            {sources.length}
          </span>
          <span className="text-[11px] text-muted-foreground mt-1">
            {syncedSources} synced{errorSources > 0 && <span className="text-destructive"> &middot; {errorSources} error</span>}
          </span>
        </div>
        <div className="flex flex-col rounded-xl border border-border p-4">
          <span className="text-xs text-muted-foreground">Documents</span>
          <span className="font-mono text-2xl font-semibold tabular-nums text-foreground mt-1">
            {formatNumber(totalDocuments)}
          </span>
          <span className="flex items-center gap-0.5 text-[11px] text-green-500 mt-1">
            <HugeiconsIcon icon={ArrowUp01Icon} size={10} />
            12% this week
          </span>
        </div>
        <div className="flex flex-col rounded-xl border border-border p-4">
          <span className="text-xs text-muted-foreground">Vectors</span>
          <span className="font-mono text-2xl font-semibold tabular-nums text-primary mt-1">
            {formatNumber(totalVectors)}
          </span>
          <span className="flex items-center gap-0.5 text-[11px] text-green-500 mt-1">
            <HugeiconsIcon icon={ArrowUp01Icon} size={10} />
            8% this week
          </span>
        </div>
        <div className="flex flex-col rounded-xl border border-border p-4">
          <span className="text-xs text-muted-foreground">Sync health</span>
          <div className="flex items-center gap-2 mt-1">
            <span className="font-mono text-2xl font-semibold tabular-nums text-foreground">
              {Math.round((syncedSources / sources.length) * 100)}%
            </span>
          </div>
          <div className="flex gap-0.5 mt-2">
            {sources.map((source) => (
              <TooltipProvider key={source.id}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span
                      className={`h-1.5 flex-1 rounded-full ${
                        source.status === "synced"
                          ? "bg-green-500"
                          : source.status === "syncing"
                            ? "bg-blue-500 animate-pulse"
                            : source.status === "error"
                              ? "bg-destructive"
                              : "bg-muted-foreground/20"
                      }`}
                    />
                  </TooltipTrigger>
                  <TooltipContent>
                    <p className="text-xs">{source.name} &middot; {source.status}</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            ))}
          </div>
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

      {/* Sources table */}
      <div className="flex flex-col gap-2">
        {/* Desktop header */}
        <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
          <span className="flex-1 min-w-0">Source</span>
          <span className="w-20 shrink-0 text-right">Documents</span>
          <span className="w-20 shrink-0 text-right">Vectors</span>
          <span className="w-28 shrink-0 text-right">Schedule</span>
          <span className="w-24 shrink-0 text-right">Last sync</span>
          <span className="w-6 shrink-0" />
          <span className="w-8 shrink-0" />
        </div>

        {filtered.map((source) => (
          <div key={source.id}>
            {/* Desktop row */}
            <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary cursor-pointer">
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <SourceIcon type={source.icon} />
                <div className="min-w-0">
                  <span className="text-sm font-medium text-foreground truncate block">{source.name}</span>
                  <span className="text-[11px] text-muted-foreground">{source.type}</span>
                </div>
              </div>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {source.documents.toLocaleString()}
              </span>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {formatNumber(source.vectors)}
              </span>
              <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {source.schedule}
              </span>
              <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {source.lastSynced ?? "Never"}
              </span>
              <div className="w-6 shrink-0 flex justify-center">
                <SyncStatus status={source.status} />
              </div>
              <div className="w-8 shrink-0 flex justify-center">
                <SourceActions source={source} />
              </div>
            </div>

            {/* Mobile row */}
            <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <SourceIcon type={source.icon} />
                  <div className="min-w-0">
                    <span className="text-sm font-medium text-foreground truncate block">{source.name}</span>
                    <span className="text-[11px] text-muted-foreground">{source.type}</span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <SyncStatus status={source.status} />
                  <SourceActions source={source} />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                  <span>{source.documents.toLocaleString()} docs</span>
                  <span>{formatNumber(source.vectors)} vectors</span>
                  <span>{source.lastSynced ?? "Never"}</span>
                </div>
              </div>
            </div>
          </div>
        ))}

        {filtered.length === 0 && (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            No sources found
          </div>
        )}
      </div>

      {/* Recent activity */}
      <div className="mt-10">
        <h2 className="font-heading text-base font-semibold text-foreground mb-4">Recent sync activity</h2>
        <div className="flex flex-col gap-2">
          {[
            { source: "ziraloop/docs", event: "Sync completed", docs: 142, time: "2 min ago", status: "success" as const },
            { source: "#support-tickets", event: "Sync completed", docs: 23, time: "30 min ago", status: "success" as const },
            { source: "Engineering Wiki", event: "Sync in progress", docs: 324, time: "Just now", status: "syncing" as const },
            { source: "SUPPORT board", event: "Sync failed — authentication expired", docs: 0, time: "3 hours ago", status: "error" as const },
            { source: "Help Center Articles", event: "Sync completed", docs: 213, time: "45 min ago", status: "success" as const },
            { source: "Product Knowledge Base", event: "Sync completed", docs: 89, time: "15 min ago", status: "success" as const },
          ].map((activity, index) => (
            <div
              key={index}
              className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm"
            >
              <span
                className={`h-1.5 w-1.5 rounded-full shrink-0 ${
                  activity.status === "success"
                    ? "bg-green-500"
                    : activity.status === "syncing"
                      ? "bg-blue-500 animate-pulse"
                      : "bg-destructive"
                }`}
              />
              <span className="font-medium text-foreground min-w-0 truncate">{activity.source}</span>
              <span className="text-muted-foreground min-w-0 truncate hidden sm:inline">{activity.event}</span>
              {activity.docs > 0 && (
                <Badge variant="secondary" className="shrink-0 hidden sm:inline-flex">
                  {activity.docs} docs
                </Badge>
              )}
              <span className="ml-auto text-xs text-muted-foreground font-mono tabular-nums shrink-0">
                {activity.time}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
