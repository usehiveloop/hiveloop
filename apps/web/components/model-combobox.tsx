"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, Tick02Icon, Loading03Icon } from "@hugeicons/core-free-icons"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"

interface ModelComboboxProps {
  models: string[]
  value?: string | null
  onSelect?: (model: string) => void
  loading?: boolean
  disabled?: boolean
}

export function ModelCombobox({ models, value, onSelect: onSelectProp, loading, disabled }: ModelComboboxProps) {
  const [open, setOpen] = useState(false)
  const selected = value ?? ""

  return (
    <Popover open={open} onOpenChange={disabled ? undefined : setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            disabled={disabled}
            className="flex w-full items-center justify-between rounded-2xl border border-input bg-input/50 px-3 py-2 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <span className={`font-mono text-sm ${selected ? "text-foreground" : "text-muted-foreground"}`}>
              {loading ? "Loading models..." : selected || "Select a model..."}
            </span>
            {loading ? (
              <HugeiconsIcon icon={Loading03Icon} size={14} className="text-muted-foreground animate-spin" />
            ) : (
              <HugeiconsIcon icon={ArrowRight01Icon} size={14} className={`text-muted-foreground/40 transition-transform ${open ? "rotate-90" : ""}`} />
            )}
          </button>
        }
      />
      <PopoverContent className="w-(--anchor-width) p-0" align="start">
        <Command>
          <CommandInput placeholder="Search models..." />
          <CommandList>
            <CommandEmpty>No models found.</CommandEmpty>
            <CommandGroup>
              {models.map((model) => (
                <CommandItem
                  key={model}
                  value={model}
                  onSelect={() => {
                    onSelectProp?.(model)
                    setOpen(false)
                  }}
                  className="font-mono text-sm"
                >
                  {model}
                  {selected === model && (
                    <HugeiconsIcon icon={Tick02Icon} size={14} className="ml-auto text-primary" />
                  )}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
