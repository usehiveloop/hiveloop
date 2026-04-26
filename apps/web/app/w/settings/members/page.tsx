"use client"

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
} from "@hugeicons/core-free-icons"

const ASSIGNABLE_ROLES = ["Admin", "Member", "Viewer"] as const

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
          {ASSIGNABLE_ROLES.map((r) => (
            <DropdownMenuItem key={r}>
              <span className="flex-1">{r}</span>
              {r === role ? (
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
  return (
    <Dialog>
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
              defaultValue="Member"
              className="h-9 rounded-md border border-input bg-transparent px-2.5 text-[13px] outline-none focus:ring-2 focus:ring-ring/50"
            >
              <option>Admin</option>
              <option>Member</option>
              <option>Viewer</option>
            </select>
          </div>
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost" size="sm" />}>
            Cancel
          </DialogClose>
          <Button size="sm">Send invites</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
