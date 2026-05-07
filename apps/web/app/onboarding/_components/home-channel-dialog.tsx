"use client"

import { useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  HashtagIcon,
  LockIcon,
  Loading03Icon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import type { components } from "@/lib/api/schema"

export type SlackChannel =
  components["schemas"]["github_com_usehiveloop_hiveloop_internal_profiles_slack.Channel"]

interface HomeChannelDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  channels: SlackChannel[]
  submitting: boolean
  errorMessage: string | null
  onConfirm: (channel: SlackChannel) => void
}

export function HomeChannelDialog({
  open,
  onOpenChange,
  channels,
  submitting,
  errorMessage,
  onConfirm,
}: HomeChannelDialogProps) {
  const [search, setSearch] = useState("")
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const sorted = useMemo(() => {
    return [...channels]
      .filter((c) => c.id && !c.is_archived)
      .sort((a, b) => {
        if (Boolean(a.is_member) !== Boolean(b.is_member)) {
          return a.is_member ? -1 : 1
        }
        return (a.name ?? "").localeCompare(b.name ?? "")
      })
  }, [channels])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return sorted
    return sorted.filter((c) => (c.name ?? "").toLowerCase().includes(query))
  }, [sorted, search])

  const selected = sorted.find((c) => c.id === selectedId) ?? null

  function handleConfirm() {
    if (!selected || submitting) return
    onConfirm(selected)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (submitting && !nextOpen) return
    if (!nextOpen) {
      setSearch("")
      setSelectedId(null)
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="gap-5 sm:max-w-md" showCloseButton={!submitting}>
        <DialogHeader>
          <DialogTitle>Pick a home channel</DialogTitle>
          <DialogDescription>
            Your AI employee will join this channel and use it as their default home. You can
            change this later.
          </DialogDescription>
        </DialogHeader>

        <div className="relative">
          <HugeiconsIcon
            icon={Search01Icon}
            className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            strokeWidth={2}
          />
          <Input
            placeholder="Search channels…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="pl-9"
            autoFocus
            disabled={submitting}
          />
        </div>

        <ScrollArea className="-mx-6 h-72 px-6">
          {filtered.length === 0 ? (
            <div className="flex h-full items-center justify-center">
              <p className="text-sm text-muted-foreground">
                {search ? "No channels match that search." : "No channels available."}
              </p>
            </div>
          ) : (
            <ul className="flex flex-col gap-1">
              {filtered.map((channel) => {
                const isSelected = channel.id === selectedId
                return (
                  <li key={channel.id}>
                    <button
                      type="button"
                      onClick={() => channel.id && setSelectedId(channel.id)}
                      disabled={submitting}
                      className={
                        "group flex w-full items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/30 disabled:cursor-not-allowed " +
                        (isSelected
                          ? "border-primary/40 bg-primary/5"
                          : "border-transparent bg-muted/40 hover:bg-muted")
                      }
                    >
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-background text-muted-foreground ring-1 ring-border">
                        <HugeiconsIcon
                          icon={channel.is_private ? LockIcon : HashtagIcon}
                          className="size-4"
                          strokeWidth={2}
                        />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {channel.name ?? "channel"}
                        </span>
                        <span className="block truncate text-[12px] text-muted-foreground">
                          {channel.is_member
                            ? "Bot is already a member"
                            : `${channel.num_members ?? 0} members`}
                        </span>
                      </span>
                      {isSelected ? (
                        <span
                          aria-hidden
                          className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
                        >
                          <HugeiconsIcon
                            icon={Tick02Icon}
                            className="size-3"
                            strokeWidth={2.75}
                          />
                        </span>
                      ) : null}
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </ScrollArea>

        {errorMessage ? (
          <div className="flex items-start gap-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-[13px] text-destructive">
            <HugeiconsIcon
              icon={Alert02Icon}
              className="mt-0.5 size-4 shrink-0"
              strokeWidth={2}
            />
            <span className="leading-relaxed">{errorMessage}</span>
          </div>
        ) : null}

        <div className="flex items-center justify-end gap-2">
          <Button
            onClick={handleConfirm}
            disabled={!selected || submitting}
            className="gap-2"
          >
            {submitting ? (
              <>
                <HugeiconsIcon
                  icon={Loading03Icon}
                  className="size-4 animate-spin"
                  strokeWidth={2}
                />
                Inviting bot…
              </>
            ) : selected ? (
              `Use #${selected.name}`
            ) : (
              "Choose a channel"
            )}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
