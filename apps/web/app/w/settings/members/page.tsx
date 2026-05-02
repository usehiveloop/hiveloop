"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select"
import { SettingsShell } from "@/components/settings-shell"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  Delete02Icon,
  MoreHorizontalCircle01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

const ROLES = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
  { value: "viewer", label: "Viewer" },
] as const

const MEMBERS = [
  { name: "Aisha Patel", email: "aisha@acme.co", role: "Owner", initials: "AP" },
  { name: "Marcus Chen", email: "marcus@acme.co", role: "Admin", initials: "MC" },
  { name: "Sara Lindqvist", email: "sara@acme.co", role: "Member", initials: "SL" },
  { name: "Diego Hernández", email: "diego@acme.co", role: "Member", initials: "DH" },
  { name: "Yuki Tanaka", email: "yuki@acme.co", role: "Viewer", initials: "YT" },
]

const PENDING = [
  { email: "alex@acme.co", role: "Member", invitedAt: "2 days ago" },
  { email: "jamie@acme.co", role: "Viewer", invitedAt: "5 days ago" },
]

export default function Page() {
  return (
    <SettingsShell
      title="Members"
      description={`${MEMBERS.length} members, ${PENDING.length} pending invites.`}
      dividers={false}
    >
      <section>
        <div className="mb-3 flex items-end justify-between gap-3">
          <h2 className="text-[13px] font-medium">All members</h2>
          <InviteDialog />
        </div>

        <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
          {MEMBERS.map((m) => (
            <li
              key={m.email}
              className="flex items-center gap-3 px-3.5 py-2.5 transition-colors hover:bg-muted/40"
            >
              <div className="flex size-8 items-center justify-center rounded-full bg-muted font-mono text-[11px] font-medium text-muted-foreground">
                {m.initials}
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium">{m.name}</p>
                <p className="truncate text-[12px] text-muted-foreground">{m.email}</p>
              </div>
              <RoleSelect role={m.role} />
              <RowMenu name={m.name} disabled={m.role === "Owner"} />
            </li>
          ))}
        </ul>
      </section>

      <section>
        <h2 className="mb-3 text-[13px] font-medium">Pending invites</h2>
        <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
          {PENDING.map((p) => (
            <li key={p.email} className="flex items-center gap-3 px-3.5 py-2.5">
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px]">{p.email}</p>
                <p className="text-[12px] text-muted-foreground">
                  Invited {p.invitedAt} as {p.role}
                </p>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 text-muted-foreground hover:text-foreground"
              >
                Resend
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 text-destructive hover:text-destructive"
              >
                Revoke
              </Button>
            </li>
          ))}
        </ul>
      </section>
    </SettingsShell>
  )
}

function RoleSelect({ role }: { role: string }) {
  const isOwner = role === "Owner"

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
        <span>{role}</span>
        <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} className="size-3" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        <DropdownMenuGroup>
          {ROLES.map((r) => (
            <DropdownMenuItem key={r.value}>
              <span className="flex-1">{r.label}</span>
              {r.label === role ? (
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
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("member")
  const [fieldError, setFieldError] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const invite = $api.useMutation("post", "/v1/orgs/current/invites")

  function handleSubmit() {
    setFieldError(null)

    const trimmed = email.trim()
    if (!trimmed) {
      setFieldError("Email is required")
      return
    }

    invite.mutate(
      { body: { email: trimmed, role } },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/orgs/current/invites"] })
          setOpen(false)
          setEmail("")
          setRole("member")
        },
        onError: (error) => {
          const message = extractErrorMessage(error, "Failed to send invite")
          setFieldError(message)
        },
      },
    )
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
            Send an invite to join this workspace.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div className="flex flex-col gap-2">
            <Label htmlFor="invite-email" className="text-[13px] font-medium">
              Email address
            </Label>
            <Input
              id="invite-email"
              type="email"
              placeholder="alex@acme.co"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value)
                if (fieldError) setFieldError(null)
              }}
            />
          </div>

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="invite-role" className="text-[13px] font-medium">
              Role
            </Label>
            <Select value={role} onValueChange={(value) => value && setRole(value)}>
              <SelectTrigger id="invite-role" className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ROLES.map((r) => (
                  <SelectItem key={r.value} value={r.value}>
                    {r.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {fieldError ? (
            <p className="text-[12px] font-medium text-destructive">{fieldError}</p>
          ) : null}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost" size="sm" />}>
            Cancel
          </DialogClose>
          <Button size="sm" onClick={handleSubmit} disabled={invite.isPending}>
            {invite.isPending ? "Sending…" : "Send invite"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
