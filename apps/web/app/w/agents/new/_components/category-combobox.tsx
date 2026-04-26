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

export const AGENT_CATEGORIES = [
  "Engineering",
  "Product",
  "Design",
  "Marketing",
  "Sales",
  "Customer Support",
  "Customer Success",
  "Operations",
  "People & HR",
  "Finance",
  "Legal",
  "Data & Analytics",
  "Security",
  "IT",
  "Research",
] as const

interface CategoryComboboxProps {
  value?: string
  onSelect?: (category: string) => void
  placeholder?: string
}

export function CategoryCombobox({
  value,
  onSelect,
  placeholder = "Select a category…",
}: CategoryComboboxProps) {
  const [open, setOpen] = React.useState(false)
  const selected = value ?? ""

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="flex h-9 w-full items-center justify-between rounded-3xl border border-transparent bg-input/50 px-3 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30"
          >
            <span
              className={
                selected ? "text-foreground" : "text-muted-foreground"
              }
            >
              {selected || placeholder}
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
          <CommandInput placeholder="Search categories…" />
          <CommandList>
            <CommandEmpty>No categories found.</CommandEmpty>
            <CommandGroup>
              {AGENT_CATEGORIES.map((category) => (
                <CommandItem
                  key={category}
                  value={category}
                  onSelect={() => {
                    onSelect?.(category)
                    setOpen(false)
                  }}
                  className="justify-between"
                >
                  <span>{category}</span>
                  {selected === category ? (
                    <HugeiconsIcon
                      icon={Tick02Icon}
                      size={14}
                      className="text-primary"
                    />
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
