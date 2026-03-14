"use client";

import { useState } from "react";
import Link from "next/link";
import { Search, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";

type SessionStatus = "Active" | "Expiring" | "Expired";

type Session = {
  id: string;
  identity: string;
  status: SessionStatus;
  providers: string[];
  expires: string;
  created: string;
};

const sessions: Session[] = [
  { id: "csess_a8f2…3e91", identity: "customer_42", status: "Active", providers: ["openai", "anthropic"], expires: "in 12 minutes", created: "Mar 9, 2026" },
  { id: "csess_b7d1…9c42", identity: "user_alex_morgan", status: "Active", providers: ["openai", "google", "anthropic"], expires: "in 8 minutes", created: "Mar 9, 2026" },
  { id: "csess_c4e9…1f73", identity: "svc_data_pipeline", status: "Expiring", providers: ["anthropic"], expires: "in 2 minutes", created: "Mar 9, 2026" },
  { id: "csess_d2f8…4a67", identity: "customer_42", status: "Expired", providers: ["openai"], expires: "expired", created: "Mar 8, 2026" },
  { id: "csess_e1a3…8d04", identity: "demo_user_trial", status: "Expired", providers: ["openai", "anthropic"], expires: "expired", created: "Mar 7, 2026" },
  { id: "csess_f3b7…c812", identity: "partner_widget_co", status: "Expired", providers: ["google"], expires: "expired", created: "Mar 5, 2026" },
];

type StatusFilter = "All" | "Active" | "Expired";
const statusCounts: Record<StatusFilter, number> = { All: 84, Active: 12, Expired: 72 };

const statusConfig: Record<SessionStatus, string> = {
  Active: "border-success/20 bg-success/10 text-success-foreground",
  Expiring: "border-warning/20 bg-warning/10 text-warning-foreground",
  Expired: "border-destructive/20 bg-destructive/10 text-destructive",
};

function SessionStatusBadge({ status }: { status: SessionStatus }) {
  return (
    <Badge variant="outline" className={`h-auto text-[11px] ${statusConfig[status]}`}>
      {status}
    </Badge>
  );
}

const columns: DataTableColumn<Session>[] = [
  {
    id: "sessionId",
    header: "Session ID",
    width: "17%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => row.id,
  },
  {
    id: "identity",
    header: "Identity",
    width: "16%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.identity,
  },
  {
    id: "status",
    header: "Status",
    width: "9%",
    cell: (row) => <SessionStatusBadge status={row.status} />,
  },
  {
    id: "providers",
    header: "Providers",
    width: "28%",
    cell: (row) => (
      <div className="flex flex-wrap gap-1.5">
        {row.providers.map((p) => (
          <Badge key={p} variant="secondary" className="h-auto bg-primary/8 font-mono text-[11px] text-chart-2">
            {p}
          </Badge>
        ))}
      </div>
    ),
  },
  {
    id: "expires",
    header: "Expires",
    width: "14%",
    cell: (row) => (
      <span className={`text-[13px] ${row.expires === "expired" ? "text-dim" : "text-success-foreground"}`}>
        {row.expires}
      </span>
    ),
  },
  {
    id: "created",
    header: "Created",
    width: "16%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.created,
  },
];

function SessionMobileCard({ session }: { session: Session }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="font-mono text-[13px] text-foreground">{session.id}</span>
          <span className="font-mono text-[11px] text-dim">{session.identity}</span>
        </div>
        <SessionStatusBadge status={session.status} />
      </div>
      <div className="flex flex-wrap gap-1.5">
        {session.providers.map((p) => (
          <Badge key={p} variant="secondary" className="h-auto bg-primary/8 font-mono text-[11px] text-chart-2">
            {p}
          </Badge>
        ))}
      </div>
      <div className="flex items-center justify-between text-xs">
        <span className={session.expires === "expired" ? "text-dim" : "text-success-foreground"}>
          {session.expires}
        </span>
        <span className="text-dim">{session.created}</span>
      </div>
    </div>
  );
}

export default function ConnectSessionsPage() {
  const [filter, setFilter] = useState<StatusFilter>("All");
  const [search, setSearch] = useState("");

  const filtered = sessions.filter((s) => {
    if (filter === "Active" && s.status !== "Active" && s.status !== "Expiring") return false;
    if (filter === "Expired" && s.status !== "Expired") return false;
    if (search && !s.id.toLowerCase().includes(search.toLowerCase()) && !s.identity.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link href="/dashboard/connect" className="text-[13px] text-dim hover:text-foreground">
            Connect UI
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">Sessions</span>
        </div>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
            Sessions
          </h1>
          <div className="flex items-center gap-3">
            <div className="relative hidden sm:block">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
              <Input
                placeholder="Search sessions..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="w-50 pl-9 font-mono text-[13px]"
              />
            </div>
            <Button size="lg" className="gap-1.5">
              <Plus className="size-4" />
              Create Session
            </Button>
          </div>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search sessions..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 items-center gap-1 px-4 pt-4 sm:px-6 lg:px-8">
        {(["All", "Active", "Expired"] as StatusFilter[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setFilter(tab)}
            className={`px-3 py-1.5 text-[13px] font-medium transition-colors ${
              filter === tab
                ? "bg-primary/8 text-chart-2"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab} ({statusCounts[tab]})
          </button>
        ))}
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        <DataTable
          columns={columns}
          data={filtered}
          keyExtractor={(row) => row.id}
          mobileCard={(row) => <SessionMobileCard session={row} />}
        />
      </section>
    </>
  );
}
