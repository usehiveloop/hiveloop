import { HugeiconsIcon } from "@hugeicons/react"
import {
  MoreHorizontalIcon,
  RefreshIcon,
  Delete02Icon,
  Settings01Icon,
} from "@hugeicons/core-free-icons"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { type Connection } from "../_data/connections"

function StatusDot({ status }: { status: Connection["status"] }) {
  const color =
    status === "active"
      ? "bg-green-500"
      : status === "error"
        ? "bg-destructive"
        : "bg-muted-foreground/50"

  return (
    <span className="relative flex h-2 w-2">
      {status === "active" && (
        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-500 opacity-40" />
      )}
      <span className={`relative inline-flex h-2 w-2 rounded-full ${color}`} />
    </span>
  )
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

export function ConnectionsTable({ connections }: { connections: Connection[] }) {
  if (connections.length === 0) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">
        No connections found
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-2">
      {/* Column headers — desktop only */}
      <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
        <span className="flex-1 min-w-0">Provider</span>
        <span className="w-20 shrink-0 text-right">Agents</span>
        <span className="w-24 shrink-0 text-right">Connected</span>
        <span className="w-6 shrink-0" />
        <span className="w-8 shrink-0" />
      </div>

      {connections.map((conn) => (
        <div key={conn.id}>
          {/* Desktop row */}
          <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary cursor-pointer">
            <div className="flex items-center gap-3 flex-1 min-w-0">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={conn.logo}
                alt={conn.displayName}
                className="h-5 w-5 shrink-0"
              />
              <span className="text-sm font-medium text-foreground truncate">
                {conn.displayName}
              </span>
            </div>
            <span className="w-20 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
              {conn.agentsUsing}
            </span>
            <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
              {formatDate(conn.connectedAt)}
            </span>
            <div className="w-6 shrink-0 flex justify-center">
              <StatusDot status={conn.status} />
            </div>
            <div className="w-8 shrink-0 flex justify-center">
              <ConnectionActions />
            </div>
          </div>

          {/* Mobile card */}
          <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary cursor-pointer">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3 min-w-0 flex-1">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={conn.logo}
                  alt={conn.displayName}
                  className="h-5 w-5 shrink-0"
                />
                <span className="text-sm font-medium text-foreground truncate">
                  {conn.displayName}
                </span>
              </div>
              <StatusDot status={conn.status} />
            </div>
            <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
              <span>{conn.agentsUsing} agents</span>
              <span>{formatDate(conn.connectedAt)}</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function ConnectionActions() {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4}>
        <DropdownMenuGroup>
          <DropdownMenuItem>
            <HugeiconsIcon icon={Settings01Icon} size={16} className="text-muted-foreground" />
            Settings
          </DropdownMenuItem>
          <DropdownMenuItem>
            <HugeiconsIcon icon={RefreshIcon} size={16} className="text-muted-foreground" />
            Reconnect
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive">
          <HugeiconsIcon icon={Delete02Icon} size={16} />
          Disconnect
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
