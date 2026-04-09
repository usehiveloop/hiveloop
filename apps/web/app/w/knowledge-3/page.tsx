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
  Tick02Icon,
  Alert02Icon,
  Cancel01Icon,
  ArrowUp01Icon,
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
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion"

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
  errorMessage?: string
}

const sources: KnowledgeSource[] = [
  { id: "src_1", name: "ziraloop/docs", type: "GitHub", icon: "github", status: "synced", documents: 142, vectors: 18_430, lastSynced: "2 min ago", schedule: "Every 6h" },
  { id: "src_2", name: "API Reference", type: "URL", icon: "url", status: "synced", documents: 86, vectors: 9_210, lastSynced: "1 hour ago", schedule: "Every 12h" },
  { id: "src_3", name: "Engineering Wiki", type: "Notion", icon: "notion", status: "syncing", documents: 324, vectors: 41_870, lastSynced: null, schedule: "Every 24h" },
  { id: "src_4", name: "#support-tickets", type: "Slack", icon: "slack", status: "synced", documents: 1_208, vectors: 52_140, lastSynced: "30 min ago", schedule: "Every 1h" },
  { id: "src_5", name: "SUPPORT board", type: "Jira", icon: "jira", status: "error", documents: 567, vectors: 22_310, lastSynced: "3 hours ago", schedule: "Every 6h", errorMessage: "Authentication token expired. Please reconnect." },
  { id: "src_6", name: "Product Knowledge Base", type: "Slab", icon: "slab", status: "synced", documents: 89, vectors: 7_640, lastSynced: "15 min ago", schedule: "Every 12h" },
  { id: "src_7", name: "Onboarding Docs", type: "Google Docs", icon: "gdocs", status: "pending", documents: 0, vectors: 0, lastSynced: null, schedule: "Every 24h" },
  { id: "src_8", name: "Help Center Articles", type: "Confluence", icon: "confluence", status: "synced", documents: 213, vectors: 15_920, lastSynced: "45 min ago", schedule: "Every 6h" },
  { id: "src_9", name: "Compliance Docs", type: "Google Docs", icon: "gdocs", status: "error", documents: 34, vectors: 2_810, lastSynced: "6 hours ago", schedule: "Every 24h", errorMessage: "Google Drive permissions changed. Re-authorize access." },
  { id: "src_10", name: "Design System", type: "GitBook", icon: "gitbook", status: "syncing", documents: 67, vectors: 5_430, lastSynced: null, schedule: "Every 12h" },
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
    gitbook: { bg: "bg-blue-500", label: "GB" },
  }
  const config = icons[type] ?? { bg: "bg-muted", label: "?" }
  return (
    <span className={`flex h-7 w-7 items-center justify-center rounded-lg ${config.bg} text-[10px] font-bold text-white dark:text-black shrink-0`}>
      {config.label}
    </span>
  )
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
/*  Status group definitions                                          */
/* ------------------------------------------------------------------ */

interface StatusGroup {
  key: string
  label: string
  description: string
  icon: typeof Tick02Icon
  iconColor: string
  dotColor: string
  statuses: KnowledgeSource["status"][]
}

const statusGroups: StatusGroup[] = [
  {
    key: "healthy",
    label: "Healthy",
    description: "Sources synced and up to date",
    icon: Tick02Icon,
    iconColor: "text-green-500",
    dotColor: "bg-green-500",
    statuses: ["synced"],
  },
  {
    key: "syncing",
    label: "Syncing",
    description: "Currently indexing new content",
    icon: Loading03Icon,
    iconColor: "text-blue-500",
    dotColor: "bg-blue-500",
    statuses: ["syncing"],
  },
  {
    key: "attention",
    label: "Needs attention",
    description: "Sources with sync failures that need fixing",
    icon: Alert02Icon,
    iconColor: "text-destructive",
    dotColor: "bg-destructive",
    statuses: ["error"],
  },
  {
    key: "pending",
    label: "Pending setup",
    description: "Sources connected but not yet synced",
    icon: Cancel01Icon,
    iconColor: "text-muted-foreground",
    dotColor: "bg-muted-foreground/30",
    statuses: ["pending"],
  },
]

/* ------------------------------------------------------------------ */
/*  Page                                                              */
/* ------------------------------------------------------------------ */

export default function KnowledgeHomeStatusGrouped() {
  const [search, setSearch] = useState("")

  const filteredSources = useMemo(() => {
    if (!search.trim()) return sources
    const query = search.toLowerCase()
    return sources.filter(
      (source) =>
        source.name.toLowerCase().includes(query) ||
        source.type.toLowerCase().includes(query),
    )
  }, [search])

  const groupedSources = useMemo(() => {
    return statusGroups.map((group) => ({
      ...group,
      sources: filteredSources.filter((source) => group.statuses.includes(source.status)),
    }))
  }, [filteredSources])

  const activeGroups = groupedSources.filter((group) => group.sources.length > 0)

  return (
    <div className="max-w-464 mx-auto w-full px-4 py-8">
      {/* Header */}
      <div className="flex items-center justify-between mb-2">
        <h1 className="font-heading text-xl font-semibold text-foreground">Knowledge</h1>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm">
            <HugeiconsIcon icon={RefreshIcon} size={14} data-icon="inline-start" />
            Sync all
          </Button>
          <Button size="default">
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Add source
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground mb-6">
        {sources.length} sources &middot; {formatNumber(totalDocuments)} documents &middot; {formatNumber(totalVectors)} vectors
      </p>

      {/* Status pills */}
      <div className="flex flex-wrap gap-2 mb-6">
        {statusGroups.map((group) => {
          const count = sources.filter((source) => group.statuses.includes(source.status)).length
          if (count === 0) return null
          return (
            <div
              key={group.key}
              className="inline-flex items-center gap-2 rounded-full border border-border px-3 py-1.5"
            >
              <span className={`h-2 w-2 rounded-full ${group.dotColor} ${group.key === "syncing" ? "animate-pulse" : ""}`} />
              <span className="text-xs text-foreground font-medium">{count} {group.label.toLowerCase()}</span>
            </div>
          )
        })}
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

      {/* Grouped sources */}
      <Accordion type="multiple" defaultValue={activeGroups.map((group) => group.key)}>
        {groupedSources.map((group) => {
          if (group.sources.length === 0) return null

          return (
            <AccordionItem key={group.key} value={group.key} className="border-none mb-4">
              <AccordionTrigger className="px-0 py-2 hover:no-underline">
                <div className="flex items-center gap-2.5">
                  <HugeiconsIcon
                    icon={group.icon}
                    size={16}
                    className={`${group.iconColor} ${group.key === "syncing" ? "animate-spin" : ""}`}
                  />
                  <span className="text-sm font-semibold text-foreground">{group.label}</span>
                  <Badge variant="secondary" className="text-[10px] font-mono">{group.sources.length}</Badge>
                </div>
              </AccordionTrigger>
              <AccordionContent className="pb-0 pt-1">
                <p className="text-[13px] text-muted-foreground mb-3">{group.description}</p>
                <div className="flex flex-col gap-2">
                  {group.sources.map((source) => (
                    <div key={source.id}>
                      {/* Desktop row */}
                      <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-3 transition-colors hover:border-primary cursor-pointer">
                        <SourceIcon type={source.icon} />
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium text-foreground truncate">{source.name}</span>
                            <Badge variant="outline" className="text-[10px]">{source.type}</Badge>
                          </div>
                          {source.errorMessage && (
                            <p className="text-[11px] text-destructive mt-0.5">{source.errorMessage}</p>
                          )}
                        </div>
                        <div className="flex items-center gap-6 text-[11px] font-mono tabular-nums text-muted-foreground shrink-0">
                          <span className="w-16 text-right">{source.documents.toLocaleString()} docs</span>
                          <span className="w-20 text-right">{formatNumber(source.vectors)} vec</span>
                          <span className="w-24 text-right">{source.lastSynced ?? "Never"}</span>
                        </div>
                        {source.status === "error" && (
                          <Button variant="outline" size="xs" className="shrink-0">
                            Fix
                          </Button>
                        )}
                        {source.status === "pending" && (
                          <Button variant="outline" size="xs" className="shrink-0">
                            <HugeiconsIcon icon={RefreshIcon} size={12} data-icon="inline-start" />
                            Start sync
                          </Button>
                        )}
                        <SourceActions source={source} />
                      </div>

                      {/* Mobile row */}
                      <div className="flex md:hidden flex-col gap-2 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2.5 min-w-0">
                            <SourceIcon type={source.icon} />
                            <div className="min-w-0">
                              <span className="text-sm font-medium text-foreground truncate block">{source.name}</span>
                              <Badge variant="outline" className="text-[10px] mt-0.5">{source.type}</Badge>
                            </div>
                          </div>
                          <SourceActions source={source} />
                        </div>
                        {source.errorMessage && (
                          <p className="text-[11px] text-destructive">{source.errorMessage}</p>
                        )}
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3 text-xs text-muted-foreground font-mono tabular-nums">
                            <span>{source.documents.toLocaleString()} docs</span>
                            <span>{formatNumber(source.vectors)} vec</span>
                            <span>{source.lastSynced ?? "Never"}</span>
                          </div>
                          {source.status === "error" && (
                            <Button variant="outline" size="xs">Fix</Button>
                          )}
                          {source.status === "pending" && (
                            <Button variant="outline" size="xs">Start sync</Button>
                          )}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </AccordionContent>
            </AccordionItem>
          )
        })}
      </Accordion>

      {filteredSources.length === 0 && (
        <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
          No sources match your search
        </div>
      )}
    </div>
  )
}
