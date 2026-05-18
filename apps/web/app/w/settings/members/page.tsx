"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
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
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  Delete02Icon,
  Loading03Icon,
  MoreHorizontalCircle01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import type { components } from "@/lib/api/schema"

type Member = components["schemas"]["orgMemberResponse"]
type Invite = components["schemas"]["orgInviteResponse"]

const ASSIGNABLE_ROLES = ["Admin", "Member", "Viewer"] as const

function getInitials(name?: string): string {
  if (!name) return ""
  return name
    .split(" ")
    .map((n) => n[0])
    .join("")
    .toUpperCase()
    .slice(0, 2)
}

function timeAgo(dateStr?: string): string {
  if (!dateStr) return ""
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diffMs = now - then
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  if (diffDays === 0) return "today"
  if (diffDays === 1) return "yesterday"
  return `${diffDays} days ago`
}

function MembersSkeleton() {
  return (
    <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
      {Array.from({ length: 3 }).map((_, i) => (
        <li key={i} className="flex items-center gap-3 px-3.5 py-2.5">
          <Skeleton className="size-8 rounded-full" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-40 rounded" />
            <Skeleton className="h-3 w-28 rounded" />
          </div>
          <Skeleton className="h-7 w-24 rounded-md" />
          <Skeleton className="size-7 rounded-md" />
        </li>
      ))}
    </ul>
  )
}

function InvitesSkeleton() {
  return (
    <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
      {Array.from({ length: 2 }).map((_, i) => (
        <li key={i} className="flex items-center gap-3 px-3.5 py-2.5">
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-48 rounded" />
            <Skeleton className="h-3 w-32 rounded" />
          </div>
          <Skeleton className="h-8 w-16 rounded-md" />
          <Skeleton className="h-8 w-16 rounded-md" />
        </li>
      ))}
    </ul>
  )
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-border/60 px-3.5 py-8 text-center">
      <p className="text-[13px] text-muted-foreground">{message}</p>
    </div>
  )
}

function ErrorState({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="rounded-lg border border-border/60 px-3.5 py-8 text-center">
      <p className="mb-3 text-[13px] text-muted-foreground">{message}</p>
      <Button variant="outline" size="sm" onClick={() => onRetry()}>
        Retry
      </Button>
    </div>
  )
}

export default function Page() {
  const queryClient = useQueryClient()

  const {
    data: membersData,
    isLoading: membersLoading,
    isError: membersError,
    refetch: refetchMembers,
  } = $api.useQuery("get", "/v1/orgs/current/members")

  const {
    data: invitesData,
    isLoading: invitesLoading,
    isError: invitesError,
    error: invitesErr,
    refetch: refetchInvites,
  } = $api.useQuery("get", "/v1/orgs/current/invites")

  const revokeInvite = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resendInvite = $api.useMutation("post", "/v1/orgs/current/invites/{id}/resend")

  const members = membersData?.data ?? []
  const pending = invitesData?.data ?? []
  const invitesForbidden =
    invitesError &&
    typeof invitesErr === "object" &&
    invitesErr !== null &&
    "status" in invitesErr &&
    (invitesErr as { status: number }).status === 403

  function handleRevoke(invite: Invite) {
    if (!invite.id) return
    revokeInvite.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => {
          toast.success("Invite revoked")
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/orgs/current/invites"],
          })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to revoke invite"))
        },
      },
    )
  }

  function handleResend(invite: Invite) {
    if (!invite.id) return
    resendInvite.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => {
          toast.success("Invite resent")
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/orgs/current/invites"],
          })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to resend invite"))
        },
      },
    )
  }

  const loading = membersLoading || invitesLoading
  const description = loading
    ? "Loading\u2026"
    : membersError
      ? "Could not load members"
      : `${members.length} member${members.length !== 1 ? "s" : ""}${!invitesForbidden && pending.length > 0 ? `, ${pending.length} pending invite${pending.length !== 1 ? "s" : ""}` : ""}.`

  return (
    <SettingsShell
      title="Members"
      description={description}
      dividers={false}
    >
      <section>
        <div className="mb-3 flex items-end justify-between gap-3">
          <h2 className="text-[13px] font-medium">All members</h2>
          {!invitesForbidden && <InviteDialog />}
        </div>

        {membersLoading ? (
          <MembersSkeleton />
        ) : membersError ? (
          <ErrorState
            message="Failed to load members"
            onRetry={refetchMembers}
          />
        ) : members.length === 0 ? (
          <EmptyState message="No members yet." />
        ) : (
          <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
            {members.map((m) => (
              <li
                key={m.user_id ?? m.email}
                className="flex items-center gap-3 px-3.5 py-2.5 transition-colors hover:bg-muted/40"
              >
                <div className="flex size-8 items-center justify-center rounded-full bg-muted font-mono text-[11px] font-medium text-muted-foreground">
                  {getInitials(m.name)}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-medium">{m.name}</p>
                  <p className="truncate text-[12px] text-muted-foreground">
                    {m.email}
                  </p>
                </div>
                <RoleSelect role={m.role ?? ""} />
                <RowMenu name={m.name ?? ""} disabled={m.role === "owner"} />
              </li>
            ))}
          </ul>
        )}
      </section>

      {!invitesForbidden && (
        <section>
          <h2 className="mb-3 text-[13px] font-medium">Pending invites</h2>
          {invitesLoading ? (
            <InvitesSkeleton />
          ) : invitesError ? (
            <ErrorState
              message="Failed to load pending invites"
              onRetry={refetchInvites}
            />
          ) : pending.length === 0 ? (
            <EmptyState message="No pending invites." />
          ) : (
            <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
              {pending.map((p) => (
                <li
                  key={p.id ?? p.email}
                  className="flex items-center gap-3 px-3.5 py-2.5"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px]">{p.email}</p>
                    <p className="text-[12px] text-muted-foreground">
                      Invited {timeAgo(p.created_at)} as{" "}
                      {p.role
                        ? p.role.charAt(0).toUpperCase() + p.role.slice(1)
                        : ""}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 text-muted-foreground hover:text-foreground"
                    onClick={() => handleResend(p)}
                    disabled={resendInvite.isPending}
                  >
                    {resendInvite.isPending ? (
                      <HugeiconsIcon
                        icon={Loading03Icon}
                        strokeWidth={2}
                        className="size-3.5 animate-spin"
                      />
                    ) : (
                      "Resend"
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 text-destructive hover:text-destructive"
                    onClick={() => handleRevoke(p)}
                    disabled={revokeInvite.isPending}
                  >
                    {revokeInvite.isPending ? (
                      <HugeiconsIcon
                        icon={Loading03Icon}
                        strokeWidth={2}
                        className="size-3.5 animate-spin"
                      />
                    ) : (
                      "Revoke"
                    )}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </SettingsShell>
  )
}

function RoleSelect({ role }: { role: string }) {
  const isOwner = role.toLowerCase() === "owner"

  if (isOwner) {
    return (
      <span className="inline-flex h-7 w-24 items-center px-2 text-[12px] text-muted-foreground">
        Owner
      </span>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            className="inline-flex h-7 w-24 items-center justify-between rounded-md px-2 text-[12px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground"
          />
        }
      >
        <span>
          {role ? role.charAt(0).toUpperCase() + role.slice(1) : ""}
        </span>
        <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} className="size-3" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        <DropdownMenuGroup>
          {ASSIGNABLE_ROLES.map((r) => (
            <DropdownMenuItem key={r}>
              <span className="flex-1">{r}</span>
              {r.toLowerCase() === role.toLowerCase() ? (
                <HugeiconsIcon
                  icon={Tick02Icon}
                  strokeWidth={2}
                  className="size-3.5 text-muted-foreground"
                />
              ) : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function RowMenu({ name, disabled }: { name: string; disabled?: boolean }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        disabled={disabled}
        render={
          <button
            type="button"
            aria-label={`More actions for ${name}`}
            className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
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
        <DropdownMenuItem variant="destructive">
          <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
          Remove
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function InviteDialog() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [emailsText, setEmailsText] = useState("")
  const [selectedRole, setSelectedRole] = useState("Member")
  const [sending, setSending] = useState(false)

  function reset() {
    setEmailsText("")
    setSelectedRole("Member")
  }

  async function handleSendInvites() {
    const emails = emailsText
      .split(/[\n,]+/)
      .map((e) => e.trim())
      .filter((e) => e.length > 0)

    if (emails.length === 0) {
      toast.error("Enter at least one email address")
      return
    }

    setSending(true)

    let successCount = 0
    for (const email of emails) {
      const { error } = await api.POST("/v1/orgs/current/invites", {
        body: { email, role: selectedRole.toLowerCase() },
      })
      if (error) {
        toast.error(extractErrorMessage(error, `Failed to invite ${email}`))
      } else {
        successCount++
      }
    }

    setSending(false)

    if (successCount > 0) {
      toast.success(
        `${successCount} invite${successCount > 1 ? "s" : ""} sent`,
      )
      reset()
      setOpen(false)
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/orgs/current/invites"],
      })
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" className="h-8" />}>
        Invite people
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Invite to Acme Inc</DialogTitle>
          <DialogDescription>
            Anyone with the link receives a join invite valid for 7 days.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="invite-emails" className="text-[13px] font-medium">
              Email addresses
            </Label>
            <textarea
              id="invite-emails"
              rows={3}
              placeholder="alex@acme.co, jamie@acme.co"
              className="resize-none rounded-md border border-input bg-transparent px-3 py-2 text-[13px] outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/50"
              value={emailsText}
              onChange={(e) => setEmailsText(e.target.value)}
            />
            <p className="text-[12px] text-muted-foreground">
              Separate multiple emails with commas or new lines.
            </p>
          </div>

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="invite-role" className="text-[13px] font-medium">
              Role
            </Label>
            <select
              id="invite-role"
              value={selectedRole}
              onChange={(e) => setSelectedRole(e.target.value)}
              className="h-9 rounded-md border border-input bg-transparent px-2.5 text-[13px] outline-none focus:ring-2 focus:ring-ring/50"
            >
              <option>Admin</option>
              <option>Member</option>
              <option>Viewer</option>
            </select>
          </div>
        </div>

        <DialogFooter>
          <DialogClose
            render={<Button variant="ghost" size="sm" />}
            onClick={() => reset()}
          >
            Cancel
          </DialogClose>
          <Button
            size="sm"
            loading={sending}
            disabled={sending}
            onClick={handleSendInvites}
          >
            {sending ? "Sending\u2026" : "Send invites"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
