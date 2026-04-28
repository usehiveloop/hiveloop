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

export interface CategoryOption {
  id: string
  name: string
  description?: string
}

interface CategoryComboboxProps {
  categories: CategoryOption[]
  value?: string
  onSelect?: (categoryId: string) => void
  loading?: boolean
  placeholder?: string
}

export function CategoryCombobox({
  categories,
  value,
  onSelect,
  loading,
  placeholder = "Select a category…",
}: CategoryComboboxProps) {
  const [open, setOpen] = React.useState(false)
  const selected = categories.find((c) => c.id === value)

  return (
    <Popover open={open} onOpenChange={loading ? undefined : setOpen}>
      <PopoverTrigger
        render={
          <button
            type="button"
            disabled={loading}
            className="flex h-9 w-full items-center justify-between rounded-3xl border border-transparent bg-input/50 px-3 text-sm transition-colors hover:bg-input/70 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <span
              className={
                selected ? "text-foreground" : "text-muted-foreground"
              }
            >
              {loading ? "Loading categories…" : selected?.name ?? placeholder}
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
              {categories.map((category) => (
                <CommandItem
                  key={category.id}
                  value={`${category.name} ${category.description ?? ""}`}
                  onSelect={() => {
                    onSelect?.(category.id)
                    setOpen(false)
                  }}
                  className="justify-between"
                >
                  <div className="min-w-0">
                    <p className="truncate">{category.name}</p>
                    {category.description ? (
                      <p className="truncate text-[11px] text-muted-foreground">
                        {category.description}
                      </p>
                    ) : null}
                  </div>
                  {selected?.id === category.id ? (
                    <HugeiconsIcon
                      icon={Tick02Icon}
                      size={14}
                      className="shrink-0 text-primary"
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
