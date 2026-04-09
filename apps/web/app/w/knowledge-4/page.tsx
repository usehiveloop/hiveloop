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
  Tick02Icon,
  Cancel01Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Switch } from "@/components/ui/switch"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

/* ------------------------------------------------------------------ */
/*  Static data                                                       */
/* ------------------------------------------------------------------ */

interface RecentDocument {
  path: string
  chunks: number
  updatedAt: string
}

interface SyncEntry {
  date: string
  status: "completed" | "failed"
  documents: number
  duration: string
}

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
  autoSync: boolean
  recentDocuments: RecentDocument[]
  syncHistory: SyncEntry[]
  errorMessage?: string
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
    schedule: "every-6-hours",
    autoSync: true,
    recentDocuments: [
      { path: "docs/getting-started.mdx", chunks: 24, updatedAt: "2 min ago" },
      { path: "docs/api-reference/agents.mdx", chunks: 38, updatedAt: "2 min ago" },
      { path: "docs/guides/webhooks.mdx", chunks: 22, updatedAt: "2 min ago" },
      { path: "docs/changelog.mdx", chunks: 42, updatedAt: "2 min ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 2:14 PM", status: "completed", documents: 142, duration: "1m 23s" },
      { date: "Apr 9, 8:14 AM", status: "completed", documents: 140, duration: "1m 18s" },
      { date: "Apr 9, 2:14 AM", status: "completed", documents: 140, duration: "1m 21s" },
    ],
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
    schedule: "every-12-hours",
    autoSync: true,
    recentDocuments: [
      { path: "/v1/agents — Create Agent", chunks: 12, updatedAt: "1 hour ago" },
      { path: "/v1/connections — List", chunks: 10, updatedAt: "1 hour ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 1:00 PM", status: "completed", documents: 86, duration: "2m 45s" },
      { date: "Apr 9, 1:00 AM", status: "completed", documents: 84, duration: "2m 38s" },
    ],
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
    schedule: "every-24-hours",
    autoSync: true,
    recentDocuments: [],
    syncHistory: [],
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
    schedule: "every-1-hour",
    autoSync: true,
    recentDocuments: [
      { path: "Thread: Login redirect broken", chunks: 4, updatedAt: "30 min ago" },
      { path: "Thread: Billing webhook delayed", chunks: 6, updatedAt: "30 min ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 1:45 PM", status: "completed", documents: 1208, duration: "4m 12s" },
      { date: "Apr 9, 12:45 PM", status: "completed", documents: 1195, duration: "3m 58s" },
    ],
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
    schedule: "every-6-hours",
    autoSync: true,
    errorMessage: "Authentication token expired. Reconnect to resume syncing.",
    recentDocuments: [
      { path: "SUPPORT-1234: OAuth flow broken", chunks: 8, updatedAt: "3 hours ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 11:14 AM", status: "failed", documents: 0, duration: "0m 12s" },
      { date: "Apr 9, 5:14 AM", status: "completed", documents: 567, duration: "3m 31s" },
    ],
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
    schedule: "every-12-hours",
    autoSync: true,
    recentDocuments: [
      { path: "Product/Feature Specs/RAG Canvas", chunks: 15, updatedAt: "15 min ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 1:59 PM", status: "completed", documents: 89, duration: "1m 02s" },
    ],
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
    schedule: "every-24-hours",
    autoSync: false,
    recentDocuments: [],
    syncHistory: [],
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
    schedule: "every-6-hours",
    autoSync: true,
    recentDocuments: [
      { path: "Getting Started Guide", chunks: 20, updatedAt: "45 min ago" },
      { path: "Troubleshooting: Common Errors", chunks: 14, updatedAt: "45 min ago" },
    ],
    syncHistory: [
      { date: "Apr 9, 1:29 PM", status: "completed", documents: 213, duration: "2m 15s" },
    ],
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
  size?: "sm" | "md"
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
  const sizeClasses = size === "sm" ? "h-6 w-6 text-[9px] rounded-md" : "h-8 w-8 text-[10px] rounded-lg"
  return (
    <span className={`flex items-center justify-center ${config.bg} ${sizeClasses} font-bold text-white dark:text-black shrink-0`}>
      {config.label}
    </span>
  )
}

function StatusDot({ status }: { status: KnowledgeSource["status"] }) {
  if (status === "synced") return <span className="h-2 w-2 rounded-full bg-green-500 shrink-0" />
  if (status === "syncing") return <span className="h-2 w-2 rounded-full bg-blue-500 animate-pulse shrink-0" />
  if (status === "error") return <span className="h-2 w-2 rounded-full bg-destructive shrink-0" />
  return <span className="h-2 w-2 rounded-full bg-muted-foreground/30 shrink-0" />
}

/* ------------------------------------------------------------------ */
/*  Page                                                              */
/* ------------------------------------------------------------------ */

export default function KnowledgeHomeSplitSidebar() {
  const [search, setSearch] = useState("")
  const [selectedSource, setSelectedSource] = useState<KnowledgeSource>(sources[0]!)

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
      <div className="flex items-center justify-between mb-2">
        <h1 className="font-heading text-xl font-semibold text-foreground">Knowledge</h1>
        <Button size="default">
          <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
          Add source
        </Button>
      </div>
      <p className="text-sm text-muted-foreground mb-6">
        {sources.length} sources &middot; {formatNumber(totalDocuments)} documents &middot; {formatNumber(totalVectors)} vectors
      </p>

      {/* Split layout */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-4">
        {/* Left sidebar — source list */}
        <div className="lg:col-span-4">
          <div className="relative mb-3">
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

          <ScrollArea className="max-h-[640px]">
            <div className="flex flex-col gap-1">
              {filtered.map((source) => {
                const isSelected = selectedSource.id === source.id
                return (
                  <button
                    key={source.id}
                    type="button"
                    onClick={() => setSelectedSource(source)}
                    className={`flex items-center gap-2.5 rounded-xl px-3 py-2.5 text-left transition-colors cursor-pointer w-full ${
                      isSelected ? "bg-primary/10 border border-primary" : "border border-transparent hover:bg-muted"
                    }`}
                  >
                    <SourceIcon type={source.icon} size="sm" />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate leading-tight">{source.name}</p>
                      <p className="text-[10px] text-muted-foreground font-mono mt-0.5">
                        {source.type} &middot; {formatNumber(source.vectors)} vec
                      </p>
                    </div>
                    <StatusDot status={source.status} />
                  </button>
                )
              })}

              {filtered.length === 0 && (
                <div className="py-6 text-center text-sm text-muted-foreground">No sources found</div>
              )}
            </div>
          </ScrollArea>
        </div>

        {/* Right panel — source detail */}
        <div className="lg:col-span-8 rounded-xl border border-border">
          {/* Detail header */}
          <div className="flex items-start justify-between px-5 py-4 border-b border-border">
            <div className="flex items-center gap-3">
              <SourceIcon type={selectedSource.icon} />
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-base font-semibold text-foreground">{selectedSource.name}</h2>
                  <Badge variant="secondary" className="text-[10px]">{selectedSource.type}</Badge>
                  <StatusDot status={selectedSource.status} />
                </div>
                {selectedSource.errorMessage && (
                  <p className="text-[12px] text-destructive mt-0.5">{selectedSource.errorMessage}</p>
                )}
                {!selectedSource.errorMessage && (
                  <p className="text-[12px] text-muted-foreground mt-0.5">
                    {selectedSource.lastSynced ? `Last synced ${selectedSource.lastSynced}` : "Never synced"}
                  </p>
                )}
              </div>
            </div>
            <div className="flex items-center gap-1.5">
              <Button variant="outline" size="xs">
                <HugeiconsIcon icon={RefreshIcon} size={12} data-icon="inline-start" />
                Sync
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon-xs">
                    <HugeiconsIcon icon={MoreHorizontalIcon} size={14} className="text-muted-foreground" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" sideOffset={4}>
                  <DropdownMenuGroup>
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
          </div>

          {/* Stats */}
          <div className="grid grid-cols-4 divide-x divide-border border-b border-border">
            {[
              { label: "Documents", value: selectedSource.documents.toLocaleString() },
              { label: "Chunks", value: formatNumber(selectedSource.chunks) },
              { label: "Vectors", value: formatNumber(selectedSource.vectors), accent: true },
              { label: "Schedule", value: selectedSource.schedule.replace("every-", "").replace("-", " ") },
            ].map((stat) => (
              <div key={stat.label} className="px-4 py-3 text-center">
                <span className="text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50 block">{stat.label}</span>
                <span className={`font-mono text-base font-semibold tabular-nums mt-0.5 block ${stat.accent ? "text-primary" : "text-foreground"}`}>
                  {stat.value}
                </span>
              </div>
            ))}
          </div>

          {/* Quick config */}
          <div className="px-5 py-3 border-b border-border flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm text-foreground">Auto-sync</span>
              <Switch checked={selectedSource.autoSync} />
            </div>
            <Select defaultValue={selectedSource.schedule}>
              <SelectTrigger className="w-36 h-7 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="every-1-hour">Every 1 hour</SelectItem>
                <SelectItem value="every-6-hours">Every 6 hours</SelectItem>
                <SelectItem value="every-12-hours">Every 12 hours</SelectItem>
                <SelectItem value="every-24-hours">Every 24 hours</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Recent documents */}
          <div className="px-5 py-4">
            <h3 className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/50 mb-3">
              Recent documents
            </h3>
            {selectedSource.recentDocuments.length > 0 ? (
              <div className="flex flex-col gap-1.5">
                {selectedSource.recentDocuments.map((document, documentIndex) => (
                  <div
                    key={documentIndex}
                    className="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-muted/50 transition-colors cursor-pointer"
                  >
                    <span className="text-sm font-mono text-foreground truncate">{document.path}</span>
                    <div className="flex items-center gap-3 shrink-0 text-[11px] text-muted-foreground font-mono tabular-nums">
                      <span>{document.chunks} chunks</span>
                      <span>{document.updatedAt}</span>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground py-4 text-center">
                {selectedSource.status === "syncing" ? "Documents will appear once sync completes..." : "No documents indexed yet"}
              </p>
            )}
          </div>

          <Separator />

          {/* Sync history */}
          <div className="px-5 py-4">
            <h3 className="text-[10px] font-mono uppercase tracking-[1.5px] text-muted-foreground/50 mb-3">
              Sync history
            </h3>
            {selectedSource.syncHistory.length > 0 ? (
              <div className="flex flex-col gap-1.5">
                {selectedSource.syncHistory.map((entry, entryIndex) => (
                  <div
                    key={entryIndex}
                    className="flex items-center gap-3 rounded-lg px-3 py-2"
                  >
                    {entry.status === "completed" ? (
                      <HugeiconsIcon icon={Tick02Icon} size={14} className="text-green-500 shrink-0" />
                    ) : (
                      <HugeiconsIcon icon={Cancel01Icon} size={14} className="text-destructive shrink-0" />
                    )}
                    <span className="text-sm text-foreground">{entry.date}</span>
                    <div className="flex-1" />
                    <span className="text-[11px] text-muted-foreground font-mono tabular-nums">{entry.documents} docs</span>
                    <span className="text-[11px] text-muted-foreground font-mono tabular-nums">{entry.duration}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground py-4 text-center">No syncs yet</p>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
