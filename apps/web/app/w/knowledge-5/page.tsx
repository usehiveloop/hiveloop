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
  ArrowDown01Icon,
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
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/components/ui/tabs"

/* ------------------------------------------------------------------ */
/*  Static data                                                       */
/* ------------------------------------------------------------------ */

interface SyncDataPoint {
  day: string
  documents: number
}

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
  trend: number
  syncData: SyncDataPoint[]
}

const sources: KnowledgeSource[] = [
  {
    id: "src_1", name: "ziraloop/docs", type: "GitHub", icon: "github", status: "synced",
    documents: 142, vectors: 18_430, lastSynced: "2 min ago", schedule: "Every 6h", trend: 4,
    syncData: [{ day: "Mon", documents: 136 }, { day: "Tue", documents: 138 }, { day: "Wed", documents: 138 }, { day: "Thu", documents: 140 }, { day: "Fri", documents: 140 }, { day: "Sat", documents: 142 }, { day: "Sun", documents: 142 }],
  },
  {
    id: "src_2", name: "API Reference", type: "URL", icon: "url", status: "synced",
    documents: 86, vectors: 9_210, lastSynced: "1 hour ago", schedule: "Every 12h", trend: 2,
    syncData: [{ day: "Mon", documents: 82 }, { day: "Tue", documents: 82 }, { day: "Wed", documents: 84 }, { day: "Thu", documents: 84 }, { day: "Fri", documents: 86 }, { day: "Sat", documents: 86 }, { day: "Sun", documents: 86 }],
  },
  {
    id: "src_3", name: "Engineering Wiki", type: "Notion", icon: "notion", status: "syncing",
    documents: 324, vectors: 41_870, lastSynced: null, schedule: "Every 24h", trend: 0,
    syncData: [{ day: "Mon", documents: 0 }, { day: "Tue", documents: 0 }, { day: "Wed", documents: 0 }, { day: "Thu", documents: 0 }, { day: "Fri", documents: 0 }, { day: "Sat", documents: 0 }, { day: "Sun", documents: 210 }],
  },
  {
    id: "src_4", name: "#support-tickets", type: "Slack", icon: "slack", status: "synced",
    documents: 1_208, vectors: 52_140, lastSynced: "30 min ago", schedule: "Every 1h", trend: 18,
    syncData: [{ day: "Mon", documents: 1024 }, { day: "Tue", documents: 1056 }, { day: "Wed", documents: 1098 }, { day: "Thu", documents: 1132 }, { day: "Fri", documents: 1160 }, { day: "Sat", documents: 1182 }, { day: "Sun", documents: 1208 }],
  },
  {
    id: "src_5", name: "SUPPORT board", type: "Jira", icon: "jira", status: "error",
    documents: 567, vectors: 22_310, lastSynced: "3 hours ago", schedule: "Every 6h", trend: -2,
    syncData: [{ day: "Mon", documents: 560 }, { day: "Tue", documents: 564 }, { day: "Wed", documents: 567 }, { day: "Thu", documents: 567 }, { day: "Fri", documents: 567 }, { day: "Sat", documents: 567 }, { day: "Sun", documents: 567 }],
  },
  {
    id: "src_6", name: "Product Knowledge Base", type: "Slab", icon: "slab", status: "synced",
    documents: 89, vectors: 7_640, lastSynced: "15 min ago", schedule: "Every 12h", trend: 6,
    syncData: [{ day: "Mon", documents: 78 }, { day: "Tue", documents: 80 }, { day: "Wed", documents: 82 }, { day: "Thu", documents: 84 }, { day: "Fri", documents: 86 }, { day: "Sat", documents: 88 }, { day: "Sun", documents: 89 }],
  },
  {
    id: "src_7", name: "Onboarding Docs", type: "Google Docs", icon: "gdocs", status: "pending",
    documents: 0, vectors: 0, lastSynced: null, schedule: "Every 24h", trend: 0,
    syncData: [{ day: "Mon", documents: 0 }, { day: "Tue", documents: 0 }, { day: "Wed", documents: 0 }, { day: "Thu", documents: 0 }, { day: "Fri", documents: 0 }, { day: "Sat", documents: 0 }, { day: "Sun", documents: 0 }],
  },
  {
    id: "src_8", name: "Help Center Articles", type: "Confluence", icon: "confluence", status: "synced",
    documents: 213, vectors: 15_920, lastSynced: "45 min ago", schedule: "Every 6h", trend: 3,
    syncData: [{ day: "Mon", documents: 204 }, { day: "Tue", documents: 206 }, { day: "Wed", documents: 208 }, { day: "Thu", documents: 210 }, { day: "Fri", documents: 211 }, { day: "Sat", documents: 212 }, { day: "Sun", documents: 213 }],
  },
]

const totalDocuments = sources.reduce((sum, source) => sum + source.documents, 0)
const totalVectors = sources.reduce((sum, source) => sum + source.vectors, 0)
const syncedCount = sources.filter((source) => source.status === "synced").length
const errorCount = sources.filter((source) => source.status === "error").length

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
    <span className={`flex h-7 w-7 items-center justify-center rounded-lg ${config.bg} text-[10px] font-bold text-white dark:text-black shrink-0`}>
      {config.label}
    </span>
  )
}

interface SparklineProps {
  data: SyncDataPoint[]
  status: KnowledgeSource["status"]
}

function Sparkline({ data, status }: SparklineProps) {
  const maxValue = Math.max(...data.map((point) => point.documents), 1)
  const color = status === "error" ? "bg-destructive/60" : status === "syncing" ? "bg-blue-500/60" : status === "pending" ? "bg-muted-foreground/20" : "bg-green-500/60"

  return (
    <div className="flex items-end gap-[2px] h-5 w-20 shrink-0">
      {data.map((point, pointIndex) => {
        const height = maxValue > 0 ? Math.max((point.documents / maxValue) * 100, 4) : 4
        return (
          <div
            key={pointIndex}
            className={`flex-1 rounded-sm ${color} ${status === "syncing" && pointIndex === data.length - 1 ? "animate-pulse" : ""}`}
            style={{ height: `${height}%` }}
          />
        )
      })}
    </div>
  )
}

function StatusLabel({ status }: { status: KnowledgeSource["status"] }) {
  if (status === "synced") return <span className="text-[10px] text-green-600 dark:text-green-400 font-medium">Synced</span>
  if (status === "syncing") return <span className="text-[10px] text-blue-600 dark:text-blue-400 font-medium">Syncing</span>
  if (status === "error") return <span className="text-[10px] text-destructive font-medium">Error</span>
  return <span className="text-[10px] text-muted-foreground font-medium">Pending</span>
}

interface SourceActionsProps {
  source: KnowledgeSource
}

function SourceActions({ source }: SourceActionsProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-7 w-7 rounded-lg transition-colors hover:bg-muted outline-none">
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
  )
}

/* ------------------------------------------------------------------ */
/*  Page                                                              */
/* ------------------------------------------------------------------ */

export default function KnowledgeHomeMinimalSparklines() {
  const [search, setSearch] = useState("")
  const [activeTab, setActiveTab] = useState("all")

  const filtered = useMemo(() => {
    let results = sources

    if (activeTab !== "all") {
      results = results.filter((source) => source.status === activeTab)
    }

    if (search.trim()) {
      const query = search.toLowerCase()
      results = results.filter(
        (source) =>
          source.name.toLowerCase().includes(query) ||
          source.type.toLowerCase().includes(query),
      )
    }

    return results
  }, [search, activeTab])

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      {/* Hero banner */}
      <div className="rounded-2xl bg-gradient-to-br from-primary/8 to-primary/3 border border-primary/10 px-6 py-6 mb-8">
        <div className="flex items-start justify-between mb-5">
          <div>
            <h1 className="font-heading text-xl font-semibold text-foreground">Knowledge</h1>
            <p className="text-sm text-muted-foreground mt-1">
              Your agents&apos; shared context from {sources.length} sources
            </p>
          </div>
          <Button size="default">
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Add source
          </Button>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <div>
            <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/60 block mb-0.5">Documents</span>
            <div className="flex items-baseline gap-1.5">
              <span className="font-mono text-2xl font-semibold tabular-nums text-foreground">{formatNumber(totalDocuments)}</span>
              <span className="flex items-center gap-0.5 text-[11px] text-green-500">
                <HugeiconsIcon icon={ArrowUp01Icon} size={10} />
                12%
              </span>
            </div>
          </div>
          <div>
            <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/60 block mb-0.5">Vectors</span>
            <div className="flex items-baseline gap-1.5">
              <span className="font-mono text-2xl font-semibold tabular-nums text-primary">{formatNumber(totalVectors)}</span>
              <span className="flex items-center gap-0.5 text-[11px] text-green-500">
                <HugeiconsIcon icon={ArrowUp01Icon} size={10} />
                8%
              </span>
            </div>
          </div>
          <div>
            <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/60 block mb-0.5">Healthy</span>
            <div className="flex items-baseline gap-1.5">
              <span className="font-mono text-2xl font-semibold tabular-nums text-foreground">{syncedCount}/{sources.length}</span>
            </div>
          </div>
          <div>
            <span className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/60 block mb-0.5">Errors</span>
            <div className="flex items-baseline gap-1.5">
              <span className={`font-mono text-2xl font-semibold tabular-nums ${errorCount > 0 ? "text-destructive" : "text-foreground"}`}>{errorCount}</span>
              {errorCount > 0 && (
                <Button variant="ghost" size="xs" className="text-destructive text-[11px] h-auto py-0.5 px-1.5">
                  Fix all
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Tabs + Search */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-4">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList>
            <TabsTrigger value="all">All ({sources.length})</TabsTrigger>
            <TabsTrigger value="synced">Synced ({sources.filter((source) => source.status === "synced").length})</TabsTrigger>
            <TabsTrigger value="error">Errors ({errorCount})</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="relative max-w-48">
          <HugeiconsIcon
            icon={Search01Icon}
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            placeholder="Search..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="pl-8 h-8 text-sm"
          />
        </div>
      </div>

      {/* Source rows */}
      <div className="flex flex-col gap-1.5">
        {/* Desktop header */}
        <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
          <span className="flex-1 min-w-0">Source</span>
          <span className="w-20 shrink-0 text-center">7-day activity</span>
          <span className="w-16 shrink-0 text-right">Documents</span>
          <span className="w-16 shrink-0 text-right">Vectors</span>
          <span className="w-12 shrink-0 text-right">Trend</span>
          <span className="w-20 shrink-0 text-right">Last sync</span>
          <span className="w-14 shrink-0 text-center">Status</span>
          <span className="w-7 shrink-0" />
        </div>

        {filtered.map((source) => (
          <div key={source.id}>
            {/* Desktop */}
            <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary cursor-pointer">
              <div className="flex items-center gap-3 flex-1 min-w-0">
                <SourceIcon type={source.icon} />
                <div className="min-w-0">
                  <span className="text-sm font-medium text-foreground truncate block">{source.name}</span>
                  <span className="text-[10px] text-muted-foreground">{source.type} &middot; {source.schedule}</span>
                </div>
              </div>
              <div className="w-20 shrink-0 flex justify-center">
                <Sparkline data={source.syncData} status={source.status} />
              </div>
              <span className="w-16 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {source.documents.toLocaleString()}
              </span>
              <span className="w-16 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {formatNumber(source.vectors)}
              </span>
              <span className="w-12 shrink-0 text-right">
                {source.trend !== 0 ? (
                  <span className={`inline-flex items-center gap-0.5 text-[11px] ${source.trend > 0 ? "text-green-500" : "text-destructive"}`}>
                    <HugeiconsIcon icon={source.trend > 0 ? ArrowUp01Icon : ArrowDown01Icon} size={10} />
                    {Math.abs(source.trend)}%
                  </span>
                ) : (
                  <span className="text-[11px] text-muted-foreground/40">&mdash;</span>
                )}
              </span>
              <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                {source.lastSynced ?? "Never"}
              </span>
              <span className="w-14 shrink-0 text-center">
                <StatusLabel status={source.status} />
              </span>
              <div className="w-7 shrink-0">
                <SourceActions source={source} />
              </div>
            </div>

            {/* Mobile */}
            <div className="flex md:hidden flex-col gap-2.5 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2.5 min-w-0 flex-1">
                  <SourceIcon type={source.icon} />
                  <div className="min-w-0">
                    <span className="text-sm font-medium text-foreground truncate block">{source.name}</span>
                    <span className="text-[10px] text-muted-foreground">{source.type}</span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <StatusLabel status={source.status} />
                  <SourceActions source={source} />
                </div>
              </div>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <Sparkline data={source.syncData} status={source.status} />
                  {source.trend !== 0 && (
                    <span className={`text-[11px] ${source.trend > 0 ? "text-green-500" : "text-destructive"}`}>
                      {source.trend > 0 ? "+" : ""}{source.trend}%
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-3 text-xs text-muted-foreground font-mono tabular-nums">
                  <span>{source.documents.toLocaleString()} docs</span>
                  <span>{source.lastSynced ?? "Never"}</span>
                </div>
              </div>
            </div>
          </div>
        ))}

        {filtered.length === 0 && (
          <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
            No sources match your filter
          </div>
        )}
      </div>

      {/* Bottom action bar */}
      <div className="flex items-center justify-between mt-6 px-1">
        <span className="text-xs text-muted-foreground">
          Powered by <span className="font-mono">R2R</span> &middot; text-embedding-3-small &middot; 1,536 dimensions
        </span>
        <Button variant="ghost" size="sm">
          <HugeiconsIcon icon={Settings01Icon} size={14} data-icon="inline-start" />
          Settings
        </Button>
      </div>
    </div>
  )
}
