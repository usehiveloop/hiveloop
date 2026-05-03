"use client"

import * as React from "react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Label } from "@/components/ui/label"
import { SettingsShell } from "@/components/settings-shell"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  Delete02Icon,
  MoreHorizontalCircle01Icon,
  Tick02Icon,
  Copy01Icon,
  ReloadIcon,
  UserRemove01Icon,
  Logout01Icon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { toast } from "sonner"
import { useAuth } from "@/lib/auth/auth-context"
import { useRouter } from "next/navigation"

const ASSIGNABLE_ROLES = ["Admin", "Member", "Viewer"] as const
const ROLE_DESCRIPTIONS: Record<string, string> = {
  Admin: "Full access: manage members, billing, and all settings.",
  Member: "Can create agents, run conversations, and connect integrations.",
  Viewer: "Read-only access to agents, conversations, and dashboards.",
}

function getInitials(name: string | undefined): string {
  const n = (name ?? "").trim()
  const parts = n.split(/\s+/)
  if (parts.length === 0) return "?"
  if (parts.length === 1) return (parts[0]?.[0] ?? "?").toUpperCase()
  return ((parts[0]?.[0] ?? "") + (parts[parts.length - 1]?.[0] ?? "")).toUpperCase()
}

function formatDate(dateStr: string): string {
  try {
    const d = new Date(dateStr)
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
  } catch {
    return "unknown"
  }
}

function isExpiringSoon(expiresAt: string): boolean {
  try {
    const d = new Date(expiresAt)
    return d.getTime() - Date.now() < 2 * 24 * 60 * 60 * 1000
  } catch {
    return false
  }
}

// ── Members list ──────────────────────────────────────────────────────────────

function MembersList() {
  const meQuery = $api.useQuery("get", "/auth/me")
  const me = meQuery.data?.user
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members", {
    // refetchInterval removed to avoid flashing
  })

  if (membersQuery.isLoading) {
    return <MembersSkeleton />
  }

  if (membersQuery.isError) {
    return (
      <div className="rounded-lg border border-border/60 bg-muted/30 px-4 py-6 text-center">
        <p className="text-[13px] text-destructive">Failed to load members.</p>
        <Button
          variant="ghost"
          size="sm"
          className="mt-2"
          onClick={() => membersQuery.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  const members = membersQuery.data?.data ?? []
  const currentUserId = me?.id
  const orgName = me?.orgs?.[0]?.name ?? ""

  return (
    <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
      {members.length === 0 ? (
        <li className="px-3.5 py-6 text-center text-[13px] text-muted-foreground">
          No members yet. Invite your team to get started.
        </li>
      ) : (
        members.map((m) => (
      <MemberRow
        key={m.user_id ?? m.email ?? ""}
        member={m}
        isCurrentUser={m.user_id === currentUserId}
        orgName={orgName}
      />
        ))
      )}
    </ul>
  )
}

function MemberRow({ member, isCurrentUser, orgName }: {
  member: { user_id?: string; email?: string; name?: string; role?: string; joined_at?: string }
  isCurrentUser: boolean
  orgName: string
}) {
  const m = {
    user_id: member.user_id ?? "",
    email: member.email ?? "",
    name: member.name ?? "",
    role: member.role ?? "",
    joined_at: member.joined_at ?? "",
  }

  return (
    <li className="flex items-center gap-3 px-3.5 py-2.5 transition-colors hover:bg-muted/40">
      <Avatar size="default" className="shrink-0">
        {m.name ? (
          <AvatarImage src="" alt={m.name} />
        ) : null}
        <AvatarFallback className="text-[11px] font-medium">
          {getInitials(m.name || m.email)}
        </AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-medium">{m.name || m.email}</p>
        <p className="truncate text-[12px] text-muted-foreground">{m.email}</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className="text-[10px]">{m.role}</Badge>
        <span className="text-[11px] text-muted-foreground hidden sm:inline">
          Joined {formatDate(m.joined_at)}
        </span>
      </div>
      {isCurrentUser ? (
        <LeaveOrgAction orgName={orgName} />
      ) : (
        <MemberRowMenu member={m} />
      )}
    </li>
  )
}

function LeaveOrgAction({ orgName }: { orgName: string }) {
  const router = useRouter()

  const handleLeave = () => {
    if (!confirm(`Leave "${orgName}"? You'll lose access to all its agents and data.`)) return
    // TODO: call DELETE /v1/orgs/current/members/{id} when backend supports it
    // For now, show a coming soon toast since backend doesn't support leaving yet
    toast.error("Leave organization is coming soon")
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      className="h-8 text-destructive hover:text-destructive"
      onClick={handleLeave}
    >
      <HugeiconsIcon icon={Logout01Icon} strokeWidth={2} className="size-3.5" />
      Leave
    </Button>
  )
}

function MemberRowMenu({ member }: {
  member: { user_id: string; email: string; name: string; role: string }
}) {
  const changeRoleMutation = $api.useMutation("patch", "/v1/orgs/current")
  const removeMutation = $api.useMutation("delete", "/v1/orgs/current/members/{id}" as any)
  const [changing, setChanging] = React.useState(false)

  // TODO: changeRole and removeMember when backend supports it
  // For now, show the menu with disabled actions

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-label={`More actions for ${member.name || member.email}`}
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground"
          />
        }
      >
        <HugeiconsIcon icon={MoreHorizontalCircle01Icon} strokeWidth={2} className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        <DropdownMenuGroup>
          <DropdownMenuItem disabled className="text-[12px] text-muted-foreground">
            Change role (coming soon)
          </DropdownMenuItem>
          <DropdownMenuItem
            variant="destructive"
            disabled
            className="text-[12px]"
          >
            <HugeiconsIcon icon={UserRemove01Icon} strokeWidth={2} className="size-3.5" />
            Remove (coming soon)
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ── Pending invites list ──────────────────────────────────────────────────────

function PendingInvitesList({ onInviteSent }: { onInviteSent?: () => void }) {
  const invitesQuery = $api.useQuery("get", "/v1/orgs/current/invites", {})
  const revokeMutation = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resendMutation = $api.useMutation("post", "/v1/orgs/current/invites/{id}/resend")

  const handleRevoke = (inviteId: string, email: string) => {
    if (!confirm(`Revoke invite for ${email}? They won't be able to join.`)) return
    revokeMutation.mutate(
      { params: { path: { id: inviteId } } },
      {
        onSuccess: () => {
          toast.success(`Invite for ${email} revoked`)
          invitesQuery.refetch()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to revoke invite"))
        },
      },
    )
  }

  const handleResend = (inviteId: string, email: string) => {
    resendMutation.mutate(
      { params: { path: { id: inviteId } } },
      {
        onSuccess: (data) => {
          toast.success(`Invite resent to ${email}`)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to resend invite"))
        },
      },
    )
  }

  const handleCopyLink = (inviteId: string) => {
    const url = `${window.location.origin}/invites/accept?token=${inviteId}`
    navigator.clipboard.writeText(url).then(
      () => toast.success("Invite link copied"),
      () => toast.error("Failed to copy link"),
    )
  }

  if (invitesQuery.isLoading) {
    return <PendingSkeleton />
  }

  if (invitesQuery.isError) {
    return (
      <div className="rounded-lg border border-border/60 bg-muted/30 px-4 py-6 text-center">
        <p className="text-[13px] text-destructive">Failed to load pending invites.</p>
        <Button
          variant="ghost"
          size="sm"
          className="mt-2"
          onClick={() => invitesQuery.refetch()}
        >
          Retry
        </Button>
      </div>
    )
  }

  const invites = invitesQuery.data?.data ?? []

  return (
    <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
      {invites.length === 0 ? (
        <li className="px-3.5 py-6 text-center text-[13px] text-muted-foreground">
          No pending invites.
        </li>
      ) : (
        invites.map((inv) => (
          <li key={inv.id ?? inv.email ?? ""} className="flex items-center gap-3 px-3.5 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{inv.email ?? ""}</p>
              <p className="text-[12px] text-muted-foreground">
                Invited {formatDate(inv.created_at ?? "")} as {inv.role ?? ""}
                {inv.expires_at && isExpiringSoon(inv.expires_at ?? "" ) ? (
                  <span className="ml-1 text-amber-600 dark:text-amber-400">(expires soon)</span>
                ) : null}
              </p>
            </div>
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                className="h-8 text-muted-foreground hover:text-foreground"
                onClick={() => handleResend(inv.id!, inv.email!)}
                loading={resendMutation.isPending && resendMutation.variables?.params?.path?.id === inv.id}
              >
                <HugeiconsIcon icon={ReloadIcon} strokeWidth={2} className="size-3.5" />
                Resend
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 text-muted-foreground hover:text-foreground"
                onClick={() => handleCopyLink(inv.id!)}
              >
                <HugeiconsIcon icon={Copy01Icon} strokeWidth={2} className="size-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 text-destructive hover:text-destructive"
                onClick={() => handleRevoke(inv.id!, inv.email!)}
                loading={revokeMutation.isPending && revokeMutation.variables?.params?.path?.id === inv.id}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} className="size-3.5" />
                Revoke
              </Button>
            </div>
          </li>
        ))
      )}
    </ul>
  )
}

// ── Invite modal ──────────────────────────────────────────────────────────────

const ROLE_INFO: Record<string, string> = {
  Admin: "Manage members, billing, and all settings.",
  Member: "Create agents, run conversations, connect integrations.",
  Viewer: "Read-only access to agents and conversations.",
}

type InviteRow = { id: string; email: string; role: string; error?: string }

function InviteDialog({ onSuccess }: { onSuccess?: () => void }) {
  const [open, setOpen] = React.useState(false)
  const [rows, setRows] = React.useState<InviteRow[]>([{ id: crypto.randomUUID(), email: "", role: "Member" }])
  const createMutation = $api.useMutation("post", "/v1/orgs/current/invites")
  const invitesQuery = $api.useQuery("get", "/v1/orgs/current/invites", {})

  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members", {})
  const existingEmails = React.useMemo(() => {
    const s = new Set<string>()
    ;(membersQuery.data?.data ?? []).forEach((m: any) => s.add((m.email ?? "").toLowerCase()))
    ;(invitesQuery.data?.data ?? []).forEach((inv: any) => s.add((inv.email ?? "").toLowerCase()))
    return s
  }, [membersQuery.data, invitesQuery.data])

  const validateEmail = (email: string): string | undefined => {
    const cleaned = email.trim().toLowerCase()
    if (!cleaned) return "Email required"
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(cleaned)) return "Invalid email"
    if (existingEmails.has(cleaned)) return "Already a member or invited"
    return undefined
  }

  const updateRow = (id: string, field: Partial<InviteRow>) => {
    setRows((prev) => prev.map((r) => r.id === id ? { ...r, ...field, error: undefined } : r))
  }

  const addRow = () => {
    setRows((prev) => [...prev, { id: crypto.randomUUID(), email: "", role: "Member" }])
  }

  const removeRow = (id: string) => {
    setRows((prev) => prev.length > 1 ? prev.filter((r) => r.id !== id) : prev)
  }

  const handleSend = () => {
    // Validate all rows
    const updated = rows.map((r) => ({ ...r, error: validateEmail(r.email) }))
    setRows(updated)
    const invalid = updated.filter((r) => r.error)
    if (invalid.length > 0) return

    const validRows = updated.filter((r) => r.email.trim())
    if (validRows.length === 0) return

    // Send invites sequentially to handle partial success
    let succeeded = 0
    let failed: { email: string; error: string }[] = []

    const sendNext = async (index: number) => {
      if (index >= validRows.length) {
        if (succeeded > 0) {
          toast.success(`Sent ${succeeded} invite${succeeded > 1 ? "s" : ""}`)
          invitesQuery.refetch()
          onSuccess?.()
        }
        if (failed.length > 0) {
          toast.error(`Failed to send ${failed.length} invite${failed.length > 1 ? "s" : ""}`)
        }
        if (succeeded === validRows.length) {
          setOpen(false)
          setRows([{ id: crypto.randomUUID(), email: "", role: "Member" }])
        }
        return
      }

      const row = validRows[index]
      createMutation.mutate(
        { body: { email: row.email.trim(), role: row.role } },
        {
          onSuccess: () => {
            succeeded++
            sendNext(index + 1)
          },
          onError: (error) => {
            failed.push({ email: row.email, error: extractErrorMessage(error, "Failed to send") })
            sendNext(index + 1)
          },
        },
      )
    }

    sendNext(0)
  }

  const isPending = createMutation.isPending

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" className="h-8" />}>
        Invite people
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Invite to workspace</DialogTitle>
          <DialogDescription>
            Invite teammates by email. They'll receive a link to join.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3 py-2 max-h-80 overflow-y-auto">
          {rows.map((row, idx) => (
            <div key={row.id} className="flex flex-col gap-1.5 rounded-md border border-border/60 p-3">
              <div className="flex items-start gap-2">
                <div className="flex-1">
                  <Label htmlFor={`invite-email-${row.id}`} className="text-[12px] font-medium sr-only">
                    Email
                  </Label>
                  <input
                    id={`invite-email-${row.id}`}
                    type="email"
                    value={row.email}
                    onChange={(e) => updateRow(row.id, { email: e.target.value })}
                    placeholder="teammate@company.com"
                    className="w-full rounded-md border border-input bg-transparent px-3 py-1.5 text-[13px] outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/50"
                  />
                  {row.error ? (
                    <p className="mt-1 text-[11px] text-destructive">{row.error}</p>
                  ) : null}
                </div>
                <select
                  value={row.role}
                  onChange={(e) => updateRow(row.id, { role: e.target.value })}
                  className="h-8 rounded-md border border-input bg-transparent px-2 text-[12px] outline-none focus:ring-2 focus:ring-ring/50"
                >
                  {ASSIGNABLE_ROLES.map((r) => (
                    <option key={r} value={r}>{r}</option>
                  ))}
                </select>
                {rows.length > 1 ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 px-2 text-muted-foreground hover:text-destructive"
                    onClick={() => removeRow(row.id)}
                  >
                    ×
                  </Button>
                ) : null}
              </div>
              <p className="text-[11px] text-muted-foreground">{ROLE_INFO[row.role]}</p>
            </div>
          ))}
        </div>

        <Button
          variant="ghost"
          size="sm"
          className="h-8 text-[12px]"
          onClick={addRow}
        >
          + Add another
        </Button>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost" size="sm" />}>
            Cancel
          </DialogClose>
          <Button
            size="sm"
            onClick={handleSend}
            loading={isPending}
          >
            Send invites
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ── Skeletons ────────────────────────────────────────────────────────────────

function MembersSkeleton() {
  return (
    <div className="divide-y divide-border/60 rounded-lg border border-border/60">
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-3 px-3.5 py-2.5">
          <div className="size-8 rounded-full bg-muted animate-pulse" />
          <div className="flex-1 space-y-1.5">
            <div className="h-3.5 w-24 rounded bg-muted animate-pulse" />
            <div className="h-3 w-32 rounded bg-muted animate-pulse" />
          </div>
          <div className="h-5 w-16 rounded bg-muted animate-pulse" />
        </div>
      ))}
    </div>
  )
}

function PendingSkeleton() {
  return (
    <div className="divide-y divide-border/60 rounded-lg border border-border/60">
      {[1, 2].map((i) => (
        <div key={i} className="flex items-center gap-3 px-3.5 py-2.5">
          <div className="flex-1 space-y-1.5">
            <div className="h-3.5 w-40 rounded bg-muted animate-pulse" />
            <div className="h-3 w-48 rounded bg-muted animate-pulse" />
          </div>
          <div className="h-7 w-16 rounded bg-muted animate-pulse" />
          <div className="h-7 w-16 rounded bg-muted animate-pulse" />
        </div>
      ))}
    </div>
  )
}

// ── Main page ────────────────────────────────────────────────────────────────

export default function Page() {
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members", {})
  const invitesQuery = $api.useQuery("get", "/v1/orgs/current/invites", {})
  const meQuery = $api.useQuery("get", "/auth/me")

  const memberCount = membersQuery.data?.data?.length ?? 0
  const pendingCount = invitesQuery.data?.data?.length ?? 0
  const orgName = meQuery.data?.orgs?.[0]?.name ?? "this workspace"

  const handleInviteSuccess = () => {
    invitesQuery.refetch()
  }

  return (
    <SettingsShell
      title="Members"
      description={`${memberCount} member${memberCount !== 1 ? "s" : ""}, ${pendingCount} pending invite${pendingCount !== 1 ? "s" : ""}.`}
      action={<InviteDialog onSuccess={handleInviteSuccess} />}
      dividers={false}
    >
      <section>
        <h2 className="mb-3 text-[13px] font-medium">All members</h2>
        <MembersList />
      </section>

      <section>
        <h2 className="mb-3 text-[13px] font-medium">Pending invites</h2>
        <PendingInvitesList onInviteSent={handleInviteSuccess} />
      </section>
    </SettingsShell>
  )
}
