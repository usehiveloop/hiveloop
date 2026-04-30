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
  AiImageIcon,
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
    () =>
      [...models].sort((a, b) => {
        const diff = badgeCount(b) - badgeCount(a)
        if (diff !== 0) return diff
        return (a.id ?? "").localeCompare(b.id ?? "")
      }),
    [models],
  )
  const selectedModel = sorted.find((m) => m.id === selected)

  return (
    <div className="flex flex-col gap-2">
      <Popover open={open} onOpenChange={disabled ? undefined : setOpen}>
        <PopoverTrigger
          render={
            <button
              type="button"
              disabled={disabled}
              className="flex w-full items-center justify-between gap-3 rounded-2xl border border-input bg-input/50 px-3 py-2 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <div className="flex flex-1 min-w-0 flex-col items-start gap-0.5">
                {selectedModel ? (
                  <>
                    <span className="truncate font-medium text-foreground">{selectedModel.name || selectedModel.id}</span>
                    {selectedModel.description ? (
                      <span className="truncate text-xs text-muted-foreground">{selectedModel.description}</span>
                    ) : null}
                  </>
                ) : (
                  <span className="text-muted-foreground">{loading ? "Loading models..." : "Select a model..."}</span>
                )}
              </div>
              {selectedModel ? <ModelBadges model={selectedModel} /> : null}
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
                      <div className="flex flex-1 min-w-0 flex-col gap-0.5">
                        <div className="flex items-center gap-2">
                          <span className="flex-1 truncate font-medium text-sm text-foreground">{name}</span>
                          <ModelBadges model={model} />
                        </div>
                        {model.description ? (
                          <span className="text-xs text-muted-foreground">{model.description}</span>
                        ) : null}
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

      <ModelBadgeLegend />
    </div>
  )
}

function ModelBadges({ model }: { model: ModelSummary }) {
  const speed = speedBadge(model.speed)
  const reasoning = !!model.reasoning
  const vision = (model.modalities?.input ?? []).includes("image")
  const cost = costTier(model.cost?.input)

  if (!speed && !reasoning && !vision && !cost) return null

  return (
    <div className="flex shrink-0 items-center gap-1">
      {speed ? <BadgeIcon tone={speed.tone} icon={speed.icon} title={`Speed: ${speed.label}`} /> : null}
      {reasoning ? <BadgeIcon tone="violet" icon={AiBrain01Icon} title="Reasoning model" /> : null}
      {vision ? <BadgeIcon tone="amber" icon={AiImageIcon} title="Vision (image input)" /> : null}
      {cost ? (
        <BadgeText
          tone="muted"
          title={`~$${(model.cost?.input ?? 0).toFixed(2)}/1M input · $${(model.cost?.output ?? 0).toFixed(2)}/1M output`}
        >
          {cost}
        </BadgeText>
      ) : null}
    </div>
  )
}

function ModelBadgeLegend() {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-1 text-[11px] text-muted-foreground">
      <LegendEntry tone="green" icon={FlashIcon} label="Fast" />
      <LegendEntry tone="amber" icon={Clock01Icon} label="Balanced" />
      <LegendEntry tone="red" icon={Clock01Icon} label="Slow" />
      <LegendEntry tone="violet" icon={AiBrain01Icon} label="Reasoning" />
      <LegendEntry tone="amber" icon={AiImageIcon} label="Vision" />
      <LegendEntry tone="muted" textBadge="$" label="Cheap" />
      <LegendEntry tone="muted" textBadge="$$$" label="Expensive" />
    </div>
  )
}

function LegendEntry({
  tone,
  icon,
  textBadge,
  label,
}: {
  tone: Tone
  icon?: Parameters<typeof HugeiconsIcon>[0]["icon"]
  textBadge?: string
  label: string
}) {
  return (
    <span className="inline-flex items-center gap-1">
      {icon ? <BadgeIcon tone={tone} icon={icon} /> : null}
      {textBadge ? <BadgeText tone={tone}>{textBadge}</BadgeText> : null}
      <span>{label}</span>
    </span>
  )
}

type Tone = "muted" | "green" | "amber" | "red" | "blue" | "violet"

const toneClass: Record<Tone, string> = {
  muted: "bg-muted text-muted-foreground",
  green: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  amber: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
  red: "bg-rose-500/10 text-rose-700 dark:text-rose-400",
  blue: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
  violet: "bg-violet-500/10 text-violet-700 dark:text-violet-400",
}

function BadgeIcon({
  tone,
  icon,
  title,
}: {
  tone: Tone
  icon: Parameters<typeof HugeiconsIcon>[0]["icon"]
  title?: string
}) {
  return (
    <span
      title={title}
      className={cn("inline-flex h-5 w-5 items-center justify-center rounded-md", toneClass[tone])}
    >
      <HugeiconsIcon icon={icon} size={11} strokeWidth={2.2} />
    </span>
  )
}

function BadgeText({
  tone,
  title,
  children,
}: {
  tone: Tone
  title?: string
  children: React.ReactNode
}) {
  return (
    <span
      title={title}
      className={cn(
        "inline-flex h-5 items-center justify-center rounded-md px-1.5 text-[10px] font-semibold",
        toneClass[tone],
      )}
    >
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

function badgeCount(model: ModelSummary): number {
  const inputModalities = model.modalities?.input
  const supportedInputModalities = inputModalities === undefined ? [] : inputModalities

  const hasSpeed = model.speed !== undefined && model.speed !== ""
  const hasReasoning = model.reasoning === true
  const hasVision = supportedInputModalities.includes("image")
  const hasCost = model.cost !== undefined && model.cost.input !== undefined && model.cost.input !== null

  let count = 0
  if (hasSpeed) {
    count = count + 1
  }
  if (hasReasoning) {
    count = count + 1
  }
  if (hasVision) {
    count = count + 1
  }
  if (hasCost) {
    count = count + 1
  }
  return count
}

function costTier(input: number | undefined): string | null {
  if (input == null) return null
  if (input <= 0.5) return "$"
  if (input <= 3) return "$$"
  return "$$$"
}
