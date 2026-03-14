"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { Search, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge, type Status } from "@/components/status-badge";
import { ProviderBadge } from "@/components/provider-badge";
import { RemainingBar, RemainingBarCompact } from "@/components/remaining-bar";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api } from "@/api/client";
import type { components } from "@/api/schema";

type CredentialResponse = components["schemas"]["credentialResponse"];

const PAGE_SIZE = 20;

function deriveStatus(cred: CredentialResponse): Status {
  if (cred.revoked_at) return "Revoked";
  if (cred.remaining != null && cred.remaining <= 0) return "Expiring";
  return "Active";
}

function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  if (diff < 60_000) return "just now";
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return `${Math.round(diff / 86_400_000)}d ago`;
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toString();
}

function truncateId(id: string): string {
  if (id.length <= 12) return id;
  return `${id.slice(0, 8)}...${id.slice(-4)}`;
}

type CredentialRow = CredentialResponse & { status: Status };

const skeletonColumns = [
  { width: "22%" },
  { width: "10%" },
  { width: "9%" },
  { width: "18%" },
  { width: "12%" },
  { width: "12%" },
  { width: "17%" },
];

const columns: DataTableColumn<CredentialRow>[] = [
  {
    id: "credential",
    header: "Credential",
    width: "22%",
    cell: (row) => (
      <Link href={`/dashboard/credentials/${row.id}`} className="flex flex-col">
        <span className="text-[13px] font-medium leading-4 text-foreground">{row.label || "Untitled"}</span>
        <span className="font-mono text-[11px] leading-3.5 text-dim">{truncateId(row.id ?? "")}</span>
      </Link>
    ),
  },
  {
    id: "provider",
    header: "Provider",
    width: "10%",
    cell: (row) => row.provider_id ? <ProviderBadge provider={row.provider_id} /> : <span className="text-xs text-dim">—</span>,
  },
  {
    id: "status",
    header: "Status",
    width: "9%",
    cell: (row) => <StatusBadge status={row.status} />,
  },
  {
    id: "remaining",
    header: "Remaining",
    width: "18%",
    cell: (row) => {
      if (row.status === "Revoked") return <span className="text-xs text-dim">—</span>;
      if (row.remaining == null) return <span className="text-xs text-dim">Unlimited</span>;
      const max = row.refill_amount ?? row.remaining;
      const percent = max > 0 ? Math.round((row.remaining / max) * 100) : 0;
      return <RemainingBar current={formatCount(row.remaining)} max={formatCount(max)} percent={percent} />;
    },
  },
  {
    id: "identity",
    header: "Identity",
    width: "12%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.identity_id ? truncateId(row.identity_id) : "—",
  },
  {
    id: "lastUsed",
    header: "Last Used",
    width: "12%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.last_used_at ? relativeTime(row.last_used_at) : "Never",
  },
  {
    id: "requests",
    header: "Requests",
    width: "17%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => formatCount(row.request_count ?? 0),
  },
];

function CredentialMobileCard({ cred }: { cred: CredentialRow }) {
  const remaining = cred.remaining != null ? {
    current: formatCount(cred.remaining),
    max: formatCount(cred.refill_amount ?? cred.remaining),
    percent: (cred.refill_amount ?? cred.remaining) > 0 ? Math.round((cred.remaining / (cred.refill_amount ?? cred.remaining)) * 100) : 0,
  } : null;

  return (
    <Link href={`/dashboard/credentials/${cred.id}`} className="flex flex-col gap-3 border border-border bg-card p-4 transition-colors hover:bg-secondary/30">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="text-[13px] font-medium leading-4 text-foreground">{cred.label || "Untitled"}</span>
          <span className="font-mono text-[11px] leading-3.5 text-dim">{truncateId(cred.id ?? "")}</span>
        </div>
        <StatusBadge status={cred.status} />
      </div>
      <div className="flex items-center gap-3">
        {cred.provider_id && <ProviderBadge provider={cred.provider_id} />}
        {cred.identity_id && (
          <span className="font-mono text-[11px] text-muted-foreground">{truncateId(cred.identity_id)}</span>
        )}
        <span className="font-mono text-[13px] text-foreground">{formatCount(cred.request_count ?? 0)} reqs</span>
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>{cred.last_used_at ? relativeTime(cred.last_used_at) : "Never"}</span>
        {remaining ? (
          <RemainingBarCompact {...remaining} />
        ) : cred.status === "Revoked" ? (
          <span className="text-dim">—</span>
        ) : (
          <span className="text-dim">Unlimited</span>
        )}
      </div>
    </Link>
  );
}

export default function CredentialsPage() {
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];

  const { data, isLoading } = $api.useQuery("get", "/v1/credentials", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });

  const credentials = (data?.data ?? []).map((c) => ({ ...c, status: deriveStatus(c) }));
  const hasMore = data?.has_more ?? false;
  const pageNumber = cursors.length + 1;

  const filtered = search
    ? credentials.filter((c) =>
        (c.label ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (c.id ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (c.provider_id ?? "").toLowerCase().includes(search.toLowerCase())
      )
    : credentials;

  const goNext = useCallback(() => {
    if (data?.next_cursor) {
      setCursors((prev) => [...prev, data.next_cursor!]);
    }
  }, [data]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Credentials
        </h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search credentials..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-60 pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg">New Credential</Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search credentials..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        {isLoading ? (
          <TableSkeleton columns={skeletonColumns} rows={6} />
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">
              {credentials.length === 0 ? "No credentials yet. Create one to get started." : "No credentials match your search."}
            </span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? ""}
            rowClassName="hover:bg-secondary/30"
            mobileCard={(row) => <CredentialMobileCard cred={row} />}
          />
        )}

        {/* Pagination */}
        {!isLoading && credentials.length > 0 && (
          <div className="mt-4 flex items-center justify-between border-t border-border pt-4">
            <span className="text-[13px] text-muted-foreground">
              Page {pageNumber}
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={cursors.length === 0}
                onClick={goPrev}
                className="h-8 gap-1 text-[13px]"
              >
                <ChevronLeft className="size-3.5" />
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={!hasMore}
                onClick={goNext}
                className="h-8 gap-1 text-[13px]"
              >
                Next
                <ChevronRight className="size-3.5" />
              </Button>
            </div>
          </div>
        )}
      </section>
    </>
  );
}
