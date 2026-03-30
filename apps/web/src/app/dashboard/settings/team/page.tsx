"use client";

import { useState } from "react";
import { Plus, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { DataTable, type DataTableColumn } from "@/components/data-table";

type Role = "Owner" | "Admin" | "Member";

type Member = {
  id: string;
  name: string;
  email: string;
  role: Role;
  joined: string;
};

const members: Member[] = [
  { id: "1", name: "Alex Morgan", email: "alex@acmecorp.com", role: "Owner", joined: "Jan 15, 2026" },
  { id: "2", name: "Jordan Lee", email: "jordan@acmecorp.com", role: "Admin", joined: "Feb 3, 2026" },
  { id: "3", name: "Sam Rivera", email: "sam@acmecorp.com", role: "Member", joined: "Feb 20, 2026" },
  { id: "4", name: "Casey Chen", email: "casey@acmecorp.com", role: "Member", joined: "Mar 1, 2026" },
];

const roleConfig: Record<Role, string> = {
  Owner: "border-primary/20 bg-primary/8 text-chart-2",
  Admin: "border-info/20 bg-info/8 text-info-foreground",
  Member: "border-border bg-secondary text-muted-foreground",
};

const columns: DataTableColumn<Member>[] = [
  {
    id: "name",
    header: "Name",
    width: "25%",
    cellClassName: "text-[13px] font-medium text-foreground",
    cell: (row) => row.name,
  },
  {
    id: "email",
    header: "Email",
    width: "30%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.email,
  },
  {
    id: "role",
    header: "Role",
    width: "15%",
    cell: (row) => (
      <Badge variant="outline" className={`h-auto text-[11px] ${roleConfig[row.role]}`}>
        {row.role}
      </Badge>
    ),
  },
  {
    id: "joined",
    header: "Joined",
    width: "20%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.joined,
  },
  {
    id: "actions",
    header: "",
    width: "10%",
    cell: (row) =>
      row.role !== "Owner" ? (
        <Button variant="ghost" size="sm" className="text-xs text-dim hover:text-foreground">
          Remove
        </Button>
      ) : null,
  },
];

// Maps UI roles to org membership roles: Owner = admin, Admin = admin, Member = viewer.
type InviteRole = "Admin" | "Member";

function InviteMemberModal({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [email, setEmail] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [role, setRole] = useState<InviteRole>("Member");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex flex-col gap-0 overflow-hidden p-0 sm:max-w-120" showCloseButton={false}>
        <DialogHeader className="flex-row items-start justify-between space-y-0 border-b border-border px-6 pb-5 pt-6">
          <div className="flex flex-col gap-1">
            <DialogTitle className="font-mono text-lg font-semibold">Invite Member</DialogTitle>
            <DialogDescription className="text-[13px]">
              Send an invitation to join this workspace.
            </DialogDescription>
          </div>
          <button onClick={() => onOpenChange(false)} className="text-dim hover:text-foreground">
            <X className="size-5" />
          </button>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5 px-6 py-5">
          {/* Email */}
          <div className="flex flex-col gap-2">
            <label className="text-[13px] text-foreground">Email</label>
            <Input
              type="email"
              required
              placeholder="colleague@company.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="text-[13px]"
            />
          </div>

          {/* Name row */}
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-2">
              <label className="text-[13px] text-foreground">First Name</label>
              <Input
                required
                placeholder="Jane"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                className="text-[13px]"
              />
            </div>
            <div className="flex flex-col gap-2">
              <label className="text-[13px] text-foreground">Last Name</label>
              <Input
                required
                placeholder="Doe"
                value={lastName}
                onChange={(e) => setLastName(e.target.value)}
                className="text-[13px]"
              />
            </div>
          </div>

          {/* Role */}
          <div className="flex flex-col gap-2">
            <label className="text-[13px] text-foreground">Role</label>
            <Select value={role} onValueChange={(v) => v && setRole(v as InviteRole)}>
              <SelectTrigger className="h-8.5 w-full text-[13px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="Admin">Admin — full access to all resources</SelectItem>
                <SelectItem value="Member">Member — read-only access</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-2 border-t border-border pt-5">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit">Send Invite</Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function MobileCard({ member }: { member: Member }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-0.5">
          <span className="text-[13px] font-medium text-foreground">{member.name}</span>
          <span className="font-mono text-[11px] text-muted-foreground">{member.email}</span>
        </div>
        <Badge variant="outline" className={`h-auto text-[11px] ${roleConfig[member.role]}`}>
          {member.role}
        </Badge>
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Joined {member.joined}</span>
        {member.role !== "Owner" && (
          <button className="text-dim hover:text-foreground">Remove</button>
        )}
      </div>
    </div>
  );
}

export default function SettingsTeamPage() {
  const [search, setSearch] = useState("");
  const [inviteOpen, setInviteOpen] = useState(false);

  const filtered = members.filter((m) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q);
  });

  return (
    <>
      <div className="flex flex-col gap-4 px-4 py-6 sm:px-6 lg:px-8">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search members..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full pl-9 font-mono text-[13px] sm:w-50"
            />
          </div>
          <Button size="lg" className="gap-1.5" onClick={() => setInviteOpen(true)}>
            <Plus className="size-4" />
            Invite Member
          </Button>
        </div>
        <DataTable
          columns={columns}
          data={filtered}
          keyExtractor={(row) => row.id}
          mobileCard={(row) => <MobileCard member={row} />}
        />
      </div>
      <InviteMemberModal open={inviteOpen} onOpenChange={setInviteOpen} />
    </>
  );
}
