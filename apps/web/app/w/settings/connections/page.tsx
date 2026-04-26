"use client"

import * as React from "react"
import { Button } from "@/components/ui/button"
import { SettingsShell } from "@/components/settings-shell"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { IntegrationLogo } from "@/components/integration-logo"
import { AddConnectionDialog } from "@/app/w/connections/_components/add-connection-dialog"
import { connections as STATIC_CONNECTIONS } from "@/app/w/connections/_data/connections"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Alert02Icon,
  Delete02Icon,
  MoreHorizontalCircle01Icon,
  RefreshIcon,
  Settings01Icon,
} from "@hugeicons/core-free-icons"

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

const STATUS_LABEL: Record<string, string> = {
  active: "Connected",
  error: "Needs attention",
  expired: "Expired",
}

export default function Page() {
  const [addOpen, setAddOpen] = React.useState(false)
  const [search, setSearch] = React.useState("")

  const errored = STATIC_CONNECTIONS.filter((c) => c.status !== "active").length

  return (
    <SettingsShell
      title="Connections"
      description={
        errored > 0
          ? `${STATIC_CONNECTIONS.length} connections, ${errored} need attention.`
          : `${STATIC_CONNECTIONS.length} connections.`
      }
      action={
        <Button size="sm" className="h-8" onClick={() => setAddOpen(true)}>
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            className="size-4"
            data-icon="inline-start"
          />
          Add connection
        </Button>
      }
      dividers={false}
    >
      <section>
        <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
          {STATIC_CONNECTIONS.map((c) => {
            const isError = c.status !== "active"
            return (
              <li
                key={c.id}
                className="flex items-center gap-3 px-3.5 py-2.5 transition-colors hover:bg-muted/40"
              >
                <IntegrationLogo provider={c.provider} size={24} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-medium">
                    {c.displayName}
                  </p>
                  <p className="truncate text-[12px] text-muted-foreground">
                    {c.agentsUsing > 0
                      ? `Used by ${c.agentsUsing} ${c.agentsUsing === 1 ? "agent" : "agents"}`
                      : "Not in use"}
                    <span className="px-1 text-muted-foreground/40">·</span>
                    Connected {formatDate(c.connectedAt)}
                  </p>
                </div>

                <span
                  className={
                    "inline-flex items-center gap-1 text-[12px] " +
                    (isError ? "text-destructive" : "text-muted-foreground")
                  }
                >
                  {isError ? (
                    <HugeiconsIcon
                      icon={Alert02Icon}
                      strokeWidth={2}
                      className="size-3.5"
                    />
                  ) : null}
                  {STATUS_LABEL[c.status] ?? c.status}
                </span>

                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <button
                        type="button"
                        aria-label={`Actions for ${c.displayName}`}
                        className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground"
                      />
                    }
                  >
                    <HugeiconsIcon
                      icon={MoreHorizontalCircle01Icon}
                      strokeWidth={2}
                      className="size-4"
                    />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-44">
                    <DropdownMenuItem>
                      <HugeiconsIcon icon={Settings01Icon} strokeWidth={2} />
                      Configure
                    </DropdownMenuItem>
                    <DropdownMenuItem>
                      <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
                      Reconnect
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive">
                      <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
                      Disconnect
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </li>
            )
          })}
        </ul>
      </section>

      <AddConnectionDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        search={search}
        onSearchChange={setSearch}
        connectingId={null}
        onConnect={() => setAddOpen(false)}
      />
    </SettingsShell>
  )
}
