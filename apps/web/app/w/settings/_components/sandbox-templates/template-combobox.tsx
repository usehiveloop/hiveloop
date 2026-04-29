"use client"

import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowDown01Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"

export interface TemplateOption {
  id: string
  name: string
  size: string
  description?: string
}

interface TemplateComboboxProps {
  templates: TemplateOption[]
  value?: string
  onSelect?: (templateId: string) => void
  placeholder?: string
}

const NONE_VALUE = ""
const NONE_LABEL = "None (default base image)"

export function TemplateCombobox({
  templates,
  value,
  onSelect,
  placeholder = "Select a template…",
}: TemplateComboboxProps) {
  const [open, setOpen] = React.useState(false)
  const selected = templates.find((t) => t.id === value)
  const hasSelection = value !== undefined && value !== NONE_VALUE

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="flex h-9 w-full items-center justify-between rounded-3xl border border-transparent bg-input/50 px-3 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30"
          >
            <span className={hasSelection ? "text-foreground" : "text-muted-foreground"}>
              {hasSelection ? `${selected?.name} (${selected?.size})` : placeholder}
            </span>
            <HugeiconsIcon
              icon={ArrowDown01Icon}
              size={14}
              className={
                "text-muted-foreground/60 transition-transform " +
                (open ? "rotate-180" : "")
              }
            />
          </button>
        }
      />
      <PopoverContent className="w-(--anchor-width) p-0" align="start">
        <Command>
          <CommandInput placeholder="Search templates…" />
          <CommandList>
            <CommandEmpty>No templates found.</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value={NONE_LABEL}
                onSelect={() => {
                  onSelect?.(NONE_VALUE)
                  setOpen(false)
                }}
                className="justify-between"
              >
                <span className="text-muted-foreground">{NONE_LABEL}</span>
                {!hasSelection ? (
                  <HugeiconsIcon icon={Tick02Icon} size={14} className="shrink-0 text-primary" />
                ) : null}
              </CommandItem>
              {templates.map((template) => (
                <CommandItem
                  key={template.id}
                  value={`${template.name} ${template.size} ${template.description ?? ""}`}
                  onSelect={() => {
                    onSelect?.(template.id)
                    setOpen(false)
                  }}
                  className="justify-between"
                >
                  <div className="min-w-0">
                    <p className="truncate">{template.name}</p>
                    {template.description ? (
                      <p className="truncate text-[11px] text-muted-foreground">{template.description}</p>
                    ) : null}
                    <p className="truncate text-[11px] text-muted-foreground/70">{template.size}</p>
                  </div>
                  {selected?.id === template.id ? (
                    <HugeiconsIcon icon={Tick02Icon} size={14} className="shrink-0 text-primary" />
                  ) : null}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
