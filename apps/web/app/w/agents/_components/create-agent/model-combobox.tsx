"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowRight01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"

interface ModelComboboxProps {
  models: string[]
  value?: string | null
  onSelect?: (model: string) => void
}

export function ModelCombobox({ models, value, onSelect: onSelectProp }: ModelComboboxProps) {
  const [open, setOpen] = useState(false)
  const [internal, setInternal] = useState(models[0] ?? "")
  const selected = value !== undefined ? (value ?? "") : internal

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button type='button' className="flex w-full items-center justify-between rounded-2xl border border-input bg-input/50 px-3 py-2 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30">
            <span className={`font-mono text-sm ${selected ? "text-foreground" : "text-muted-foreground"}`}>
              {selected || "Select a model..."}
            </span>
            <HugeiconsIcon icon={ArrowRight01Icon} size={14} className={`text-muted-foreground/40 transition-transform ${open ? "rotate-90" : ""}`} />
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
                    if (onSelectProp) onSelectProp(model)
                    else setInternal(model)
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
