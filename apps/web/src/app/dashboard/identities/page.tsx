"use client";

import { useState } from "react";
import Link from "next/link";
import { Search, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";

type RateLimit = {
  type: string;
  value: string;
};

type Identity = {
  externalId: string;
  rateLimits: RateLimit[];
  credentials: number;
  meta: Record<string, string>;
  created: string;
};

const identities: Identity[] = [
  { externalId: "customer_42", rateLimits: [{ type: "requests", value: "100/60s" }, { type: "tokens", value: "50k/60s" }], credentials: 3, meta: { plan: "pro", region: "us" }, created: "Mar 1, 2026" },
  { externalId: "user_alex_morgan", rateLimits: [{ type: "requests", value: "50/60s" }], credentials: 2, meta: { plan: "enterprise", team: "backend" }, created: "Feb 28, 2026" },
  { externalId: "org_acme_staging", rateLimits: [], credentials: 5, meta: { env: "staging" }, created: "Feb 25, 2026" },
  { externalId: "svc_data_pipeline", rateLimits: [{ type: "requests", value: "500/60s" }, { type: "tokens", value: "200k/60s" }], credentials: 1, meta: { plan: "pro", type: "service" }, created: "Feb 20, 2026" },
  { externalId: "demo_user_trial", rateLimits: [{ type: "requests", value: "10/60s" }], credentials: 1, meta: { plan: "trial" }, created: "Feb 18, 2026" },
  { externalId: "ci_runner_main", rateLimits: [], credentials: 4, meta: {}, created: "Feb 14, 2026" },
  { externalId: "partner_widget_co", rateLimits: [{ type: "requests", value: "200/60s" }], credentials: 2, meta: { plan: "pro", partner: "true" }, created: "Feb 10, 2026" },
];

type LimitsFilter = "All" | "With Limits" | "No Limits";

const limitsCounts: Record<LimitsFilter, number> = {
  All: 156,
  "With Limits": 89,
  "No Limits": 67,
};

function RateLimitBadge({ type, value }: RateLimit) {
  return (
    <Badge variant="outline" className="h-auto border-primary/20 bg-primary/8 px-2 py-0.5 font-mono text-[11px] font-normal text-chart-2">
      {type}: {value}
    </Badge>
  );
}

function MetaBadge({ label }: { label: string }) {
  return (
    <Badge variant="outline" className="h-auto border-border bg-secondary px-2 py-0.5 font-mono text-[11px] font-normal text-muted-foreground">
      {label}
    </Badge>
  );
}

const columns: DataTableColumn<Identity>[] = [
  {
    id: "externalId",
    header: "External ID",
    width: "20%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => (
      <Link href={`/dashboard/identities/${row.externalId}`} className="hover:underline">
        {row.externalId}
      </Link>
    ),
  },
  {
    id: "rateLimits",
    header: "Rate Limits",
    width: "28%",
    cell: (row) =>
      row.rateLimits.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {row.rateLimits.map((rl) => (
            <RateLimitBadge key={rl.type} {...rl} />
          ))}
        </div>
      ) : (
        <span className="text-[13px] text-dim">&mdash;</span>
      ),
  },
  {
    id: "credentials",
    header: "Credentials",
    width: "10%",
    cellClassName: "text-[13px] text-foreground",
    cell: (row) => row.credentials,
  },
  {
    id: "meta",
    header: "Meta",
    width: "26%",
    cell: (row) => {
      const entries = Object.entries(row.meta);
      return entries.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {entries.map(([k, v]) => (
            <MetaBadge key={k} label={`${k}: ${v}`} />
          ))}
        </div>
      ) : (
        <span className="text-[13px] text-dim">&mdash;</span>
      );
    },
  },
  {
    id: "created",
    header: "Created",
    width: "16%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.created,
  },
];

function IdentityMobileCard({ identity }: { identity: Identity }) {
  return (
    <Link
      href={`/dashboard/identities/${identity.externalId}`}
      className="flex flex-col gap-3 border border-border bg-card p-4 transition-colors hover:bg-secondary/30"
    >
      <div className="flex items-start justify-between">
        <span className="font-mono text-[13px] font-medium text-foreground">{identity.externalId}</span>
        <span className="text-[13px] text-muted-foreground">{identity.credentials} creds</span>
      </div>
      {identity.rateLimits.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {identity.rateLimits.map((rl) => (
            <RateLimitBadge key={rl.type} {...rl} />
          ))}
        </div>
      )}
      <div className="flex items-center justify-between">
        <div className="flex flex-wrap gap-1.5">
          {Object.entries(identity.meta).map(([k, v]) => (
            <MetaBadge key={k} label={`${k}: ${v}`} />
          ))}
        </div>
        <span className="text-xs text-dim">{identity.created}</span>
      </div>
    </Link>
  );
}

export default function IdentitiesPage() {
  const [filter, setFilter] = useState<LimitsFilter>("All");
  const [search, setSearch] = useState("");

  const filtered = identities.filter((id) => {
    if (filter === "With Limits" && id.rateLimits.length === 0) return false;
    if (filter === "No Limits" && id.rateLimits.length > 0) return false;
    if (search && !id.externalId.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Identities
        </h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search identities..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-50 pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg" className="gap-1.5">
            <Plus className="size-4" />
            Create Identity
          </Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search identities..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 flex-wrap items-center gap-3 px-4 pt-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-1">
          {(["All", "With Limits", "No Limits"] as LimitsFilter[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setFilter(tab)}
              className={`px-3 py-1.5 text-[13px] font-medium transition-colors ${
                filter === tab
                  ? "bg-primary/8 text-chart-2"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab} ({limitsCounts[tab]})
            </button>
          ))}
        </div>
        <div className="hidden h-5 w-px bg-border sm:block" />
        <div className="hidden items-center gap-2 sm:flex">
          <button className="flex items-center gap-1.5 px-3 py-1.5 text-[13px] text-muted-foreground transition-colors hover:text-foreground">
            Meta Filter
            <svg className="size-3" viewBox="0 0 12 12" fill="none">
              <path d="M3 5L6 8L9 5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
        </div>
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        <DataTable
          columns={columns}
          data={filtered}
          keyExtractor={(row) => row.externalId}
          rowClassName="hover:bg-secondary/30"
          mobileCard={(row) => <IdentityMobileCard identity={row} />}
        />
      </section>
    </>
  );
}
