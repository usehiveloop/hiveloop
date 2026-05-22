"use client"

import { useState, useMemo } from "react"
import { motion, AnimatePresence } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  SlackIcon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import {
  GithubIcon,
  GoogleDriveIcon,
  GoogleExcelIcon,
  FigmaIcon,
  StripeIcon,
  TrelloIcon,
  SentryIcon,
  PostgresIcon,
  GoogleCloudIcon,
  AwsIcon,
} from "@/components/icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import Link from "next/link"

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      className={className}
    >
      <path
        d="M3 8.5L6.5 12L13 5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

type Step = "slack" | "connections" | "business"

const steps: { id: Step; label: string }[] = [
  { id: "slack", label: "Invite hivy to slack" },
  { id: "connections", label: "Add connections" },
  { id: "business", label: "Your business" },
]

function StepIndicator({ current }: { current: Step }) {
  const currentIndex = steps.findIndex((s) => s.id === current)

  return (
    <div className="flex w-full max-w-lg items-start justify-between">
      {steps.map((step, index) => {
        const isActive = index === currentIndex
        const isDone = index < currentIndex
        const isPending = index > currentIndex

        return (
          <div key={step.id} className="flex flex-1 items-center last:flex-initial">
            <div className="flex flex-col items-center gap-2.5">
              {/* Step circle */}
              <div className="relative">
                {isActive && (
                  <motion.div
                    layoutId="active-step-ring"
                    className="absolute -inset-1.5 rounded-full border-2 border-primary/30"
                    transition={{ type: "spring", stiffness: 300, damping: 30 }}
                  />
                )}
                <motion.div
                  initial={false}
                  animate={{
                    scale: isActive ? 1 : 1,
                    backgroundColor: isDone
                      ? "var(--primary)"
                      : isActive
                        ? "var(--primary)"
                        : "transparent",
                    borderColor: isDone
                      ? "var(--primary)"
                      : isActive
                        ? "var(--primary)"
                        : "var(--border)",
                    color: isDone || isActive
                      ? "var(--primary-foreground)"
                      : "var(--muted-foreground)",
                  }}
                  transition={{ duration: 0.3, ease: "easeInOut" }}
                  className={cn(
                    "flex size-10 items-center justify-center rounded-full border-2 text-sm font-semibold",
                    isPending && "opacity-50"
                  )}
                >
                  {isDone ? (
                    <motion.div
                      initial={{ scale: 0, opacity: 0 }}
                      animate={{ scale: 1, opacity: 1 }}
                      transition={{ type: "spring", stiffness: 500, damping: 30 }}
                    >
                      <CheckIcon className="size-4" />
                    </motion.div>
                  ) : (
                    index + 1
                  )}
                </motion.div>
              </div>

              {/* Label */}
              <span
                className={cn(
                  "whitespace-nowrap text-xs font-medium transition-colors duration-300",
                  isActive
                    ? "text-foreground"
                    : isDone
                      ? "text-foreground"
                      : "text-muted-foreground/60"
                )}
              >
                {step.label}
              </span>
            </div>

            {/* Connector line */}
            {index < steps.length - 1 && (
              <div className="relative mx-2 -mt-6 h-[2px] flex-1 overflow-hidden rounded-full bg-border sm:mx-4">
                <motion.div
                  className="absolute inset-y-0 left-0 rounded-full bg-primary"
                  initial={false}
                  animate={{
                    width: isDone ? "100%" : "0%",
                  }}
                  transition={{ duration: 0.5, ease: "easeInOut" }}
                />
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

function SlackLogo({ className, size = 48 }: { className?: string; size?: number }) {
  return (
    <svg
      viewBox="0 0 127 127"
      width={size}
      height={size}
      className={className}
    >
      <path
        d="M27.2 80.0c0 7.3-5.9 13.2-13.2 13.2S.8 87.3.8 80c0-7.3 5.9-13.2 13.2-13.2h13.2v13.2zm6.6 0c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2v33c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V80z"
        fill="#E01E5A"
      />
      <path
        d="M47.0 27.0c-7.3 0-13.2-5.9-13.2-13.2S39.7.6 47.0.6s13.2 5.9 13.2 13.2v13.2H47.0zm0 6.7c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H13.5C6.2 60.1.3 54.2.3 46.9s5.9-13.2 13.2-13.2h33.5z"
        fill="#36C5F0"
      />
      <path
        d="M99.9 46.9c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2-5.9 13.2-13.2 13.2H99.9V46.9zm-6.6 0c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V13.5C66.9 6.2 72.8.3 80.1.3s13.2 5.9 13.2 13.2v33.4z"
        fill="#2EB67D"
      />
      <path
        d="M80.1 99.8c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2-13.2-5.9-13.2-13.2V99.8h13.2zm0-6.6c-7.3 0-13.2-5.9-13.2-13.2s5.9-13.2 13.2-13.2h33.5c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H80.1z"
        fill="#ECB22E"
      />
    </svg>
  )
}

function GhostLogo({ size = 40 }: { size?: number }) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      fill="currentColor"
      className="text-muted-foreground"
    >
      <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
      <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill="var(--background)" />
      <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill="var(--background)" />
    </svg>
  )
}

interface Channel {
  id: string
  name: string
  memberCount: number
  isPrivate: boolean
}

const mockChannels: Channel[] = [
  { id: "1", name: "general", memberCount: 42, isPrivate: false },
  { id: "2", name: "engineering", memberCount: 18, isPrivate: false },
  { id: "3", name: "design", memberCount: 8, isPrivate: false },
  { id: "4", name: "marketing", memberCount: 12, isPrivate: false },
  { id: "5", name: "product", memberCount: 6, isPrivate: false },
  { id: "6", name: "support", memberCount: 15, isPrivate: false },
  { id: "7", name: "announcements", memberCount: 42, isPrivate: false },
  { id: "8", name: "random", memberCount: 35, isPrivate: false },
  { id: "9", name: "finance", memberCount: 4, isPrivate: true },
  { id: "10", name: "leadership", memberCount: 5, isPrivate: true },
  { id: "11", name: "hr", memberCount: 3, isPrivate: true },
  { id: "12", name: "sales", memberCount: 9, isPrivate: false },
  { id: "13", name: "devops", memberCount: 7, isPrivate: false },
  { id: "14", name: "frontend", memberCount: 5, isPrivate: false },
  { id: "15", name: "backend", memberCount: 6, isPrivate: false },
  { id: "16", name: "ai-ml", memberCount: 4, isPrivate: false },
  { id: "17", name: "customer-success", memberCount: 11, isPrivate: false },
  { id: "18", name: "partnerships", memberCount: 3, isPrivate: true },
]

function SlackStep({ onContinue }: { onContinue: () => void }) {
  const [phase, setPhase] = useState<"idle" | "selecting">("idle")
  const [search, setSearch] = useState("")
  const [selectedIds, setSelectedIds] = useState<Set<string>>(
    () => new Set(mockChannels.map((c) => c.id))
  )

  const filteredChannels = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return mockChannels
    return mockChannels.filter((c) => c.name.toLowerCase().includes(query))
  }, [search])

  const handleConnect = () => {
    setPhase("selecting")
  }

  const toggleChannel = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleAll = () => {
    if (selectedIds.size === filteredChannels.length) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(filteredChannels.map((c) => c.id)))
    }
  }

  return (
    <AnimatePresence mode="wait">
      {phase === "idle" && (
        <motion.div
          key="idle"
          initial={{ opacity: 0, y: 12 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -12 }}
          transition={{ duration: 0.3, ease: "easeOut" }}
          className="flex flex-col items-center text-center"
        >
          <div className="mb-8 flex items-center gap-4">
            <GhostLogo size={48} />
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <div className="h-px w-8 bg-border" />
              <HugeiconsIcon icon={ArrowRight01Icon} size={14} />
              <div className="h-px w-8 bg-border" />
            </div>
            <SlackLogo size={48} />
          </div>

          <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
            Connect your Slack workspace
          </h1>
          <p className="mt-4 max-w-md text-base leading-relaxed text-muted-foreground">
            Hivy lives in Slack. Connecting your workspace lets Hivy join channels,
            answer questions, and complete tasks alongside your team.
          </p>

          <div className="mt-8">
            <Button
              size="lg"
              onClick={handleConnect}
              className="min-w-64 gap-2"
            >
              <HugeiconsIcon icon={SlackIcon} size={18} />
              Connect Slack workspace
            </Button>
          </div>
        </motion.div>
      )}

      {phase === "selecting" && (
        <motion.div
          key="selecting"
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: "easeOut" }}
          className="flex max-w-md w-full flex-col"
        >
          {/* Header */}
          <div className="mb-8 flex flex-col items-center text-center">
            <h2 className="font-heading text-2xl font-normal tracking-[-0.02em] text-foreground">
              Choose channels
            </h2>
            <p className="mt-1.5 text-sm text-muted-foreground">
              Select the channels Hivy should join
            </p>
          </div>

          <div className="relative mb-4">
            <HugeiconsIcon
              icon={Search01Icon}
              className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search channels"
              className="pl-9 bg-white dark:bg-input"
            />
          </div>

          <div className="mb-3 flex items-center justify-between px-1">
            <span className="text-xs text-muted-foreground">
              {selectedIds.size} of {filteredChannels.length} selected
            </span>
            <Button
              variant="ghost"
              size="sm"
              onClick={toggleAll}
              className="h-auto px-0 py-0 text-xs font-medium text-muted-foreground hover:text-foreground"
            >
              {selectedIds.size === filteredChannels.length ? "Deselect all" : "Select all"}
            </Button>
          </div>

          <ScrollArea className="mb-6 h-80 md:h-105">
            <div className="flex flex-col gap-2 pr-2">
              {filteredChannels.map((channel) => {
                const isSelected = selectedIds.has(channel.id)
                return (
                  <button
                    key={channel.id}
                    type="button"
                    onClick={() => toggleChannel(channel.id)}
                    className={cn(
                      "group flex cursor-pointer items-center gap-4 rounded-xl border px-5 py-2 text-left transition-all border-border bg-card hover:bg-muted",
                    )}
                  >
                    <div className="flex min-w-0 flex-1 flex-col">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-foreground">
                          #{channel.name}
                        </span>
                        {channel.isPrivate && (
                          <span className="rounded-full border border-border bg-secondary px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                            Private
                          </span>
                        )}
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {channel.memberCount} members
                      </span>
                    </div>

                    {/* Checkmark on the right */}
                    <div
                      className={cn(
                        "shrink-0 text-primary transition-opacity",
                        isSelected ? "opacity-100" : "opacity-0"
                      )}
                    >
                      <CheckIcon className="size-5" />
                    </div>
                  </button>
                )
              })}
              {filteredChannels.length === 0 && (
                <div className="flex h-40 items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
                  No channels found
                </div>
              )}
            </div>
          </ScrollArea>

          {/* Continue */}
          <Button
            size="lg"
            onClick={onContinue}
            disabled={selectedIds.size === 0}
            className="w-full gap-2"
          >
            Invite Hivy to {selectedIds.size} channel{selectedIds.size !== 1 ? "s" : ""}
            <HugeiconsIcon icon={ArrowRight01Icon} size={16} data-icon="inline-end" />
          </Button>
        </motion.div>
      )}
    </AnimatePresence>
  )
}

interface Integration {
  id: string
  name: string
  icon: React.ReactNode
}

const integrations: Integration[] = [
  { id: "github", name: "GitHub", icon: <GithubIcon size={20} /> },
  { id: "sheets", name: "Sheets", icon: <GoogleExcelIcon size={20} /> },
  { id: "drive", name: "Drive", icon: <GoogleDriveIcon size={20} /> },
  { id: "figma", name: "Figma", icon: <FigmaIcon size={20} /> },
  { id: "stripe", name: "Stripe", icon: <StripeIcon size={20} /> },
  { id: "trello", name: "Trello", icon: <TrelloIcon size={20} /> },
  { id: "sentry", name: "Sentry", icon: <SentryIcon size={20} /> },
  { id: "postgres", name: "Postgres", icon: <PostgresIcon size={20} /> },
  { id: "gcp", name: "Google Cloud", icon: <GoogleCloudIcon size={20} /> },
  { id: "aws", name: "AWS", icon: <AwsIcon size={20} /> },
]

function ConnectionsStep({ onContinue }: { onContinue: () => void }) {
  const [search, setSearch] = useState("")
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const filteredIntegrations = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return integrations
    return integrations.filter((i) => i.name.toLowerCase().includes(query))
  }, [search])

  const toggleIntegration = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: "easeOut" }}
      className="flex w-full max-w-xl flex-col"
    >
      {/* Header */}
      <div className="mb-8 text-center">
        <h1 className="font-heading text-3xl leading-[1.1] font-normal text-left tracking-[-0.02em] text-foreground md:text-4xl">
          Add connections
        </h1>
        <p className="text-left mt-4 max-w-lg text-base leading-relaxed text-muted-foreground">
          Connect the tools Hivy will work with — GitHub, Google Sheets, and more.
        </p>
      </div>

      {/* Search */}
      <div className="relative mb-6">
        <HugeiconsIcon
          icon={Search01Icon}
          className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search integrations"
          className="bg-white pl-9 dark:bg-input"
        />
      </div>

      {/* Grid */}
      <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {filteredIntegrations.map((integration) => {
          const isSelected = selectedIds.has(integration.id)
          return (
            <button
              key={integration.id}
              type="button"
              onClick={() => toggleIntegration(integration.id)}
              className={cn(
                "group relative flex cursor-pointer items-center gap-3 rounded-xl border px-4 py-3.5 text-left transition-all",
                isSelected
                  ? "border-primary/30 bg-primary/4"
                  : "border-border bg-card"
              )}
            >
              <div className="shrink-0">{integration.icon}</div>
              <span className="min-w-0 flex-1 text-sm font-medium text-foreground">
                {integration.name}
              </span>
              {isSelected && (
                <div className="flex size-4 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground">
                  <HugeiconsIcon icon={Tick02Icon} size={8} strokeWidth={2} />
                </div>
              )}
            </button>
          )
        })}
      </div>

      {filteredIntegrations.length === 0 && (
        <div className="mb-6 flex h-32 items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
          No integrations found
        </div>
      )}

      {/* Footer bar */}
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">
          {selectedIds.size} selected
        </span>
        <Button
          onClick={onContinue}
          className="gap-2"
        >
          Continue
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} data-icon="inline-end" />
        </Button>
      </div>
    </motion.div>
  )
}

function SlackMockup({ businessName }: { businessName: string }) {
  const channels = ["general", "engineering", "design", "announcements", "random"]
  const dms = ["Sarah", "hivy", "Mike"]

  return (
    <div className="w-full max-w-2xl overflow-hidden rounded-2xl shadow-2xl">
      <div className="flex h-[320px] md:h-[420px]">
        {/* Sidebar - hidden on mobile */}
        <div className="hidden w-44 shrink-0 flex-col md:flex" style={{ backgroundColor: "#3F0E40" }}>
          <div className="flex items-center gap-2 px-3 py-3">
            <div className="flex h-6 w-6 items-center justify-center rounded bg-white/10">
              <svg viewBox="0 0 127 127" className="h-3.5 w-3.5">
                <path d="M27.2 80.0c0 7.3-5.9 13.2-13.2 13.2S.8 87.3.8 80c0-7.3 5.9-13.2 13.2-13.2h13.2v13.2zm6.6 0c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2v33c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V80z" fill="#E01E5A" />
                <path d="M47.0 27.0c-7.3 0-13.2-5.9-13.2-13.2S39.7.6 47.0.6s13.2 5.9 13.2 13.2v13.2H47.0zm0 6.7c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H13.5C6.2 60.1.3 54.2.3 46.9s5.9-13.2 13.2-13.2h33.5z" fill="#36C5F0" />
                <path d="M99.9 46.9c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2-5.9 13.2-13.2 13.2H99.9V46.9zm-6.6 0c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V13.5C66.9 6.2 72.8.3 80.1.3s13.2 5.9 13.2 13.2v33.4z" fill="#2EB67D" />
                <path d="M80.1 99.8c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2-13.2-5.9-13.2-13.2V99.8h13.2zm0-6.6c-7.3 0-13.2-5.9-13.2-13.2s5.9-13.2 13.2-13.2h33.5c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H80.1z" fill="#ECB22E" />
              </svg>
            </div>
            <div className="min-w-0">
              <div className="truncate text-sm font-bold text-white">{businessName}</div>
            </div>
          </div>
          <div className="px-3 py-2">
            <div className="mb-1 px-2 text-xs font-semibold tracking-wide text-white/40 uppercase">Channels</div>
            {channels.map((c) => (
              <div key={c} className="flex items-center gap-2 rounded px-2 py-1 text-sm" style={{ color: c === "general" ? "white" : "rgba(255,255,255,0.6)", backgroundColor: c === "general" ? "#1164A3" : "transparent" }}>
                <span className="text-white/40">#</span>
                <span className="truncate">{c}</span>
              </div>
            ))}
          </div>
          <div className="px-3 py-2">
            <div className="mb-1 px-2 text-xs font-semibold tracking-wide text-white/40 uppercase">Direct messages</div>
            {dms.map((dm) => (
              <div key={dm} className="flex items-center gap-2 rounded px-2 py-1 text-sm text-white/60">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-green-400" />
                </span>
                <span className="truncate">{dm}</span>
                {dm === "hivy" && <span className="ml-auto rounded bg-white/10 px-1.5 py-0 text-[10px] text-white/60">APP</span>}
              </div>
            ))}
          </div>
        </div>

        {/* Chat area */}
        <div className="flex flex-1 flex-col bg-white">
          <div className="flex items-center border-b border-gray-200 px-4 py-3 md:px-5">
            <div className="flex items-center gap-2">
              <span className="text-lg font-bold text-gray-400">#</span>
              <span className="text-base font-bold text-gray-900">general</span>
            </div>
            <div className="ml-4 flex items-center gap-1 text-sm text-gray-400">
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
              <span>3</span>
            </div>
          </div>
          <div className="flex-1 overflow-y-auto px-4 py-4 md:px-5">
            {/* Welcome message from hivy */}
            <div className="flex items-start gap-3 py-2">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-sm font-bold text-white" style={{ backgroundColor: "var(--pill-from)" }}>
                <svg viewBox="0 0 640 640" className="h-5 w-5" fill="currentColor">
                  <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
                  <ellipse cx="318.5" cy="282" rx="45.5" ry="101" fill="white" />
                  <ellipse cx="457.5" cy="282" rx="45.5" ry="101" fill="white" />
                </svg>
              </div>
              <div className="min-w-0 flex-1 flex-col">
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
                  <span className="text-sm font-bold" style={{ color: "var(--pill-from)" }}>hivy</span>
                  <span className="rounded bg-gray-100 px-1 py-0 text-[10px] font-semibold text-gray-500">APP</span>
                  <span className="text-xs text-gray-400">9:41 AM</span>
                </div>
                <p className="mt-0.5 text-sm leading-relaxed text-gray-700">
                  Hey team! I'm hivy, your new AI coworker. I'm here to help you get things done — just mention me in any channel or send me a DM. 🎉
                </p>
              </div>
            </div>
          </div>
          <div className="border-t border-gray-200 px-4 py-3 md:px-5">
            <div className="flex items-center gap-2 rounded-lg border border-gray-300 px-4 py-2.5">
              <svg className="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" /></svg>
              <span className="text-sm text-gray-400">Message #general</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function BusinessStep() {
  const [name, setName] = useState("")
  const [website, setWebsite] = useState("")
  const [description, setDescription] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [phase, setPhase] = useState<"form" | "success">("form")

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitting(true)
    setTimeout(() => {
      setSubmitting(false)
      setPhase("success")
    }, 800)
  }

  if (phase === "success") {
    return (
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: "easeOut" }}
        className="flex w-full max-w-2xl flex-col items-center"
      >
        {/* Header */}
        <div className="mb-6 text-center">
          <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
            You're all set
          </h1>
          <p className="mx-auto mt-3 max-w-lg text-base leading-relaxed text-muted-foreground">
            hivy just joined your team. Ping @hivy on slack to assign your first task.
          </p>
        </div>

        <div className="mb-8 w-full">
          <SlackMockup businessName={name} />
        </div>

        <div className="flex items-center gap-4">
          <Button
          size="lg"
          asChild
          className="gap-2"
        >
          <a href="slack://" className="flex items-center gap-2">
            <HugeiconsIcon icon={SlackIcon} size={18} />
            Ping @hivy
          </a>
        </Button>
        <Button variant='ghost'>
          <Link href='/w'>
          Go to dashboard</Link>
        </Button>
        </div>
      </motion.div>
    )
  }

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, ease: "easeOut" }}
      className="flex w-full max-w-md flex-col"
    >
      {/* Header */}
      <div className="mb-8 text-center">
        <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Tell hivy more about your business
        </h1>
        <p className="mx-auto mt-4 max-w-md text-base leading-relaxed text-muted-foreground">
          This helps hivy understand your team and personalize your experience.
        </p>
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        <div className="space-y-2">
          <Label htmlFor="business-name">Business name</Label>
          <Input
            id="business-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Acme Inc"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="business-website">Website</Label>
          <Input
            id="business-website"
            type="url"
            value={website}
            onChange={(e) => setWebsite(e.target.value)}
            placeholder="https://acme.com"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="business-description">What does your business do?</Label>
          <Textarea
            id="business-description"
            value={description}
            className="min-h-20"
            onChange={(e) => setDescription(e.target.value)}
            placeholder="We build software for remote teams..."
            rows={8}
          />
        </div>

        <Button
          type="submit"
          size="lg"
          loading={submitting}
          disabled={!name.trim()}
          className="mt-2 w-full gap-2"
        >
          Finish
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} data-icon="inline-end" />
        </Button>
      </form>
    </motion.div>
  )
}

export default function OnboardingV2Page() {
  const [step, setStep] = useState<Step>("slack")

  return (
    <main
      className="relative flex min-h-screen flex-col items-center font-display text-foreground"

    >
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute top-0 left-1/2 h-160 w-160 -translate-x-1/2 rounded-full bg-(--glow-center) opacity-20 blur-[160px]" />
      </div>

      <div className="relative z-10 flex w-full flex-col items-center gap-6 px-4 pt-8 pb-6 sm:pt-12">
        <div className="flex items-center gap-2">
          <GhostLogo size={28} />
          <span className="font-heading text-lg font-medium tracking-tight text-foreground">
            hivy
          </span>
        </div>
        <StepIndicator current={step} />
      </div>

      <div className="relative z-10 w-full flex flex-1 flex-col items-center justify-center px-4 py-8 sm:py-12">
        {step === "slack" && (
          <SlackStep onContinue={() => setStep("connections")} />
        )}
        {step === "connections" && (
          <ConnectionsStep onContinue={() => setStep("business")} />
        )}
        {step === "business" && <BusinessStep />}
      </div>

      <div className="relative z-10 px-4 py-6 text-center text-xs text-muted-foreground/60">
        <p>
          Need help?{" "}
          <a
            href="mailto:hello@usehivy.com"
            className="text-muted-foreground hover:text-foreground underline underline-offset-2 transition-colors"
          >
            Contact support
          </a>
        </p>
      </div>
    </main>
  )
}
