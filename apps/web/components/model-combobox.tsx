"use client"

import { useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  Tick02Icon,
  Loading03Icon,
  FlashIcon,
  Clock01Icon,
  AiBrain01Icon,
  CodeIcon,
  AiImageIcon,
  GlobeIcon,
} from "@hugeicons/core-free-icons"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

export type ModelSummary = components["schemas"]["modelSummary"]

interface ModelComboboxProps {
  models: ModelSummary[]
  value?: string | null
  onSelect?: (model: string) => void
  loading?: boolean
  disabled?: boolean
}

export function ModelCombobox({ models, value, onSelect, loading, disabled }: ModelComboboxProps) {
  const [open, setOpen] = useState(false)
  const selected = value ?? ""

  const sorted = useMemo(
    () => [...models].sort((a, b) => (a.id ?? "").localeCompare(b.id ?? "")),
    [models],
  )
  const selectedModel = sorted.find((m) => m.id === selected)

  return (
    <Popover open={open} onOpenChange={disabled ? undefined : setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            disabled={disabled}
            className="flex w-full items-center justify-between gap-3 rounded-2xl border border-input bg-input/50 px-3 py-2 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <div className="flex flex-1 min-w-0 items-center gap-2">
              {selectedModel ? (
                <>
                  <span className="truncate font-medium text-foreground">{selectedModel.name || selectedModel.id}</span>
                  <span className="font-mono truncate text-xs text-muted-foreground">{selectedModel.id}</span>
                </>
              ) : (
                <span className="text-muted-foreground">{loading ? "Loading models..." : "Select a model..."}</span>
              )}
            </div>
            {selectedModel ? <ModelBadges model={selectedModel} compact /> : null}
            {loading ? (
              <HugeiconsIcon icon={Loading03Icon} size={14} className="text-muted-foreground animate-spin" />
            ) : (
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                size={14}
                className={`text-muted-foreground/40 transition-transform ${open ? "rotate-90" : ""}`}
              />
            )}
          </button>
        }
      />
      <PopoverContent className="w-(--anchor-width) p-0" align="start">
        <Command
          filter={(value, search) => {
            const haystack = value.toLowerCase()
            return haystack.includes(search.toLowerCase()) ? 1 : 0
          }}
        >
          <CommandInput placeholder="Search models..." />
          <CommandList className="max-h-[420px]">
            <CommandEmpty>No models found.</CommandEmpty>
            <CommandGroup>
              {sorted.map((model) => {
                const id = model.id ?? ""
                const name = model.name || id
                return (
                  <CommandItem
                    key={id}
                    value={`${id} ${name} ${model.family ?? ""} ${model.description ?? ""}`}
                    onSelect={() => {
                      onSelect?.(id)
                      setOpen(false)
                    }}
                    className="items-start gap-2 py-2"
                  >
                    <div className="flex flex-1 min-w-0 flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-sm text-foreground">{name}</span>
                        {model.family ? (
                          <span className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
                            {model.family}
                          </span>
                        ) : null}
                      </div>
                      <span className="font-mono text-xs text-muted-foreground">{id}</span>
                      <ModelBadges model={model} />
                    </div>
                    {selected === id && (
                      <HugeiconsIcon icon={Tick02Icon} size={14} className="ml-auto mt-1 shrink-0 text-primary" />
                    )}
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function ModelBadges({ model, compact }: { model: ModelSummary; compact?: boolean }) {
  const speed = speedBadge(model.speed)
  const reasoning = !!model.reasoning
  const toolCall = !!model.tool_call
  const vision = (model.modalities?.input ?? []).includes("image")
  const openWeights = !!model.open_weights
  const cost = costTier(model.cost?.input)

  return (
    <div className={cn("flex flex-wrap items-center gap-1", compact ? "shrink-0" : "")}>
      {speed ? (
        <Badge tone={speed.tone} icon={speed.icon} title={`Speed: ${speed.label}`}>
          {!compact ? speed.label : null}
        </Badge>
      ) : null}
      {reasoning ? (
        <Badge tone="violet" icon={AiBrain01Icon} title="Reasoning model">
          {!compact ? "Reasoning" : null}
        </Badge>
      ) : null}
      {toolCall ? (
        <Badge tone="blue" icon={CodeIcon} title="Tool calling">
          {!compact ? "Tools" : null}
        </Badge>
      ) : null}
      {vision ? (
        <Badge tone="amber" icon={AiImageIcon} title="Vision (image input)">
          {!compact ? "Vision" : null}
        </Badge>
      ) : null}
      {openWeights ? (
        <Badge tone="green" icon={GlobeIcon} title="Open weights">
          {!compact ? "Open" : null}
        </Badge>
      ) : null}
      {cost ? (
        <Badge tone="muted" title={`~$${(model.cost?.input ?? 0).toFixed(2)}/1M input · $${(model.cost?.output ?? 0).toFixed(2)}/1M output`}>
          {cost}
        </Badge>
      ) : null}
    </div>
  )
}

type Tone = "muted" | "green" | "amber" | "red" | "blue" | "violet"

function Badge({
  tone = "muted",
  icon,
  title,
  children,
}: {
  tone?: Tone
  icon?: Parameters<typeof HugeiconsIcon>[0]["icon"]
  title?: string
  children?: React.ReactNode
}) {
  const toneClass = {
    muted: "bg-muted text-muted-foreground",
    green: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
    amber: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
    red: "bg-rose-500/10 text-rose-700 dark:text-rose-400",
    blue: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
    violet: "bg-violet-500/10 text-violet-700 dark:text-violet-400",
  }[tone]
  return (
    <span
      title={title}
      className={cn(
        "inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium",
        toneClass,
      )}
    >
      {icon ? <HugeiconsIcon icon={icon} size={10} strokeWidth={2.2} /> : null}
      {children}
    </span>
  )
}

function speedBadge(speed: string | undefined): { label: string; tone: Tone; icon: Parameters<typeof HugeiconsIcon>[0]["icon"] } | null {
  if (!speed) return null
  const s = speed.toLowerCase()
  if (s === "fast") return { label: "Fast", tone: "green", icon: FlashIcon }
  if (s === "slow") return { label: "Slow", tone: "red", icon: Clock01Icon }
  if (s === "balanced" || s === "medium") return { label: "Balanced", tone: "amber", icon: Clock01Icon }
  return { label: speed, tone: "muted", icon: Clock01Icon }
}

function costTier(input: number | undefined): string | null {
  if (input == null) return null
  if (input <= 0.5) return "$"
  if (input <= 3) return "$$"
  return "$$$"
}
