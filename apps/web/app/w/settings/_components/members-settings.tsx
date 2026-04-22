"use client"

import * as React from "react"
import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { ConfirmDialog } from "@/components/confirm-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  MailSend01Icon,
} from "@hugeicons/core-free-icons"

type InviteRole = "admin" | "member" | "viewer"

function formatDate(iso?: string) {
  if (!iso) return "\u2014"
  return new Date(iso).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function InviteForm({ onInvited }: { onInvited: () => void }) {
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<InviteRole>("member")
  const createInvite = $api.useMutation("post", "/v1/orgs/current/invites")

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    const trimmed = email.trim()
    if (!trimmed) return
    createInvite.mutate(
      { body: { email: trimmed, role } },
      {
        onSuccess: () => {
          toast.success(`Invitation sent to ${trimmed}`)
          setEmail("")
          setRole("member")
          onInvited()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to send invitation"))
        },
      },
    )
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3 sm:flex-row sm:items-end">
      <div className="flex-1 flex flex-col gap-2">
        <Label htmlFor="invite-email">Email address</Label>
        <Input
          id="invite-email"
          type="email"
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          placeholder="teammate@company.com"
          required
        />
      </div>
      <div className="flex flex-col gap-2 sm:w-40">
        <Label htmlFor="invite-role">Role</Label>
        <Select value={role} onValueChange={(value) => setRole(value as InviteRole)}>
          <SelectTrigger id="invite-role">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="admin">Admin</SelectItem>
            <SelectItem value="member">Member</SelectItem>
            <SelectItem value="viewer">Viewer</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Button
        type="submit"
        loading={createInvite.isPending}
        disabled={!email.trim()}
        className="sm:w-auto"
      >
        <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
        Send invite
      </Button>
    </form>
  )
}

export function MembersSettings() {
  const queryClient = useQueryClient()
  const [revoking, setRevoking] = useState<{ id: string; email: string } | null>(null)

  const invitesQuery = $api.useQuery("get", "/v1/orgs/current/invites")
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")

  const revokeInvite = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resendInvite = $api.useMutation("post", "/v1/orgs/current/invites/{id}/resend")

  const invites = invitesQuery.data?.data ?? []
  const members = membersQuery.data?.data ?? []

  function refreshInvites() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/orgs/current/invites"] })
  }

  function handleResend(id: string, email: string) {
    resendInvite.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          toast.success(`Invitation resent to ${email}`)
          refreshInvites()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to resend invitation"))
        },
      },
    )
  }

  function handleRevokeConfirm() {
    if (!revoking) return
    revokeInvite.mutate(
      { params: { path: { id: revoking.id } } },
      {
        onSuccess: () => {
          toast.success(`Invitation to ${revoking.email} revoked`)
          refreshInvites()
          setRevoking(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to revoke invitation"))
          setRevoking(null)
        },
      },
    )
  }

  return (
    <div className="space-y-10">
      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-medium text-foreground">Invite teammates</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Invite people to this workspace by email. They&apos;ll get a link to accept.
          </p>
        </div>
        <InviteForm onInvited={refreshInvites} />
      </section>

      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-medium text-foreground">Pending invitations</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Invites that have been sent but not yet accepted.
          </p>
        </div>

        {invitesQuery.isLoading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <Skeleton key={i} className="h-13 w-full rounded-xl" />
            ))}
          </div>
        ) : invites.length === 0 ? (
          <p className="text-sm text-muted-foreground">No pending invitations.</p>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Email</span>
              <span className="w-20 shrink-0">Role</span>
              <span className="w-40 shrink-0">Invited by</span>
              <span className="w-28 shrink-0 text-right">Sent</span>
              <span className="w-8 shrink-0" />
            </div>
            {invites.map((invite) => (
              <div key={invite.id}>
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{invite.email}</p>
                  </div>
                  <div className="w-20 shrink-0">
                    <Badge variant="secondary" className="text-[10px]">{invite.role}</Badge>
                  </div>
                  <span className="w-40 shrink-0 text-[11px] text-muted-foreground truncate">
                    {invite.invited_by_name || invite.invited_by_email || "\u2014"}
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {formatDate(invite.created_at)}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <DropdownMenu>
                      <DropdownMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
                        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" sideOffset={4}>
                        <DropdownMenuGroup>
                          <DropdownMenuItem
                            onClick={() => invite.id && invite.email && handleResend(invite.id, invite.email)}
                          >
                            <HugeiconsIcon icon={MailSend01Icon} size={16} className="text-muted-foreground" />
                            Resend
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                        <DropdownMenuSeparator />
                        <DropdownMenuGroup>
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() =>
                              invite.id && invite.email && setRevoking({ id: invite.id, email: invite.email })
                            }
                          >
                            <HugeiconsIcon icon={Delete02Icon} size={16} />
                            Revoke
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>

                {/* Mobile */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4">
                  <div className="flex items-center justify-between">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate">{invite.email}</p>
                      <p className="text-xs text-muted-foreground">
                        {invite.invited_by_name || invite.invited_by_email || "\u2014"}
                      </p>
                    </div>
                    <Badge variant="secondary" className="text-[10px]">{invite.role}</Badge>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => invite.id && invite.email && handleResend(invite.id, invite.email)}
                    >
                      Resend
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        invite.id && invite.email && setRevoking({ id: invite.id, email: invite.email })
                      }
                    >
                      Revoke
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-3">
        <div>
          <h3 className="text-sm font-medium text-foreground">Team members</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Everyone with access to this workspace.
          </p>
        </div>

        {membersQuery.isLoading ? (
          <div className="flex flex-col gap-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <Skeleton key={i} className="h-13 w-full rounded-xl" />
            ))}
          </div>
        ) : members.length <= 1 ? (
          <div className="flex flex-col gap-2">
            {members.map((member) => (
              <div
                key={member.user_id}
                className="flex items-center gap-3 rounded-xl border border-border px-4 py-2.5"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground truncate">
                    {member.name || member.email}
                  </p>
                  <p className="text-xs text-muted-foreground truncate">{member.email}</p>
                </div>
                <Badge variant="secondary" className="text-[10px]">{member.role}</Badge>
              </div>
            ))}
            <p className="text-sm text-muted-foreground">
              You&apos;re the only member. Invite someone above to get started.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-56 shrink-0">Email</span>
              <span className="w-20 shrink-0">Role</span>
              <span className="w-28 shrink-0 text-right">Joined</span>
            </div>
            {members.map((member) => (
              <div
                key={member.user_id}
                className="flex flex-col gap-1 rounded-xl border border-border px-4 py-2.5 md:flex-row md:items-center md:gap-3"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground truncate">
                    {member.name || member.email}
                  </p>
                </div>
                <span className="w-56 shrink-0 text-[11px] text-muted-foreground truncate md:block">
                  {member.email}
                </span>
                <div className="w-20 shrink-0">
                  <Badge variant="secondary" className="text-[10px]">{member.role}</Badge>
                </div>
                <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                  {formatDate(member.joined_at)}
                </span>
              </div>
            ))}
          </div>
        )}
      </section>

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => { if (!open) setRevoking(null) }}
        title="Revoke invitation"
        description={`This will permanently revoke the invitation to ${revoking?.email ?? ""}.`}
        confirmText="revoke"
        confirmLabel="Revoke invite"
        destructive
        loading={revokeInvite.isPending}
        onConfirm={handleRevokeConfirm}
      />
    </div>
  )
}
