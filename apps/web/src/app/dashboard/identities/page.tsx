"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { Search, ChevronLeft, ChevronRight, Users, ArrowRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api } from "@/api/client";
import type { components } from "@/api/schema";
import { CreateIdentityDialog } from "./create-identity-dialog";

type IdentityResponse = components["schemas"]["identityResponse"];
type RateLimitParam = components["schemas"]["identityRateLimitParams"];

const PAGE_SIZE = 20;

const skeletonColumns = [
  { width: "20%" },
  { width: "28%" },
  { width: "10%" },
  { width: "26%" },
  { width: "16%" },
];

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function formatDuration(ms: number): string {
  if (ms >= 3_600_000) return `${Math.round(ms / 3_600_000)}h`;
  if (ms >= 60_000) return `${Math.round(ms / 60_000)}m`;
  return `${Math.round(ms / 1_000)}s`;
}

function RateLimitBadge({ rl }: { rl: RateLimitParam }) {
  const value = rl.limit != null && rl.duration != null
    ? `${rl.limit}/${formatDuration(rl.duration)}`
    : "—";
  return (
    <Badge
      variant="outline"
      className="h-auto border-primary/20 bg-primary/8 px-2 py-0.5 font-mono text-[11px] font-normal text-chart-2"
    >
      {rl.name ?? "rate"}: {value}
    </Badge>
  );
}

function MetaBadge({ label }: { label: string }) {
  return (
    <Badge
      variant="outline"
      className="h-auto border-border bg-secondary px-2 py-0.5 font-mono text-[11px] font-normal text-muted-foreground"
    >
      {label}
    </Badge>
  );
}

function IdentityMobileCard({ identity }: { identity: IdentityResponse }) {
  const ratelimits = identity.ratelimits ?? [];
  const meta = (identity.meta ?? {}) as Record<string, unknown>;
  const metaEntries = Object.entries(meta);

  return (
    <Link
      href={`/dashboard/identities/${identity.id}`}
      className="flex flex-col gap-3 border border-border bg-card p-4 transition-colors hover:bg-secondary/30"
    >
      <div className="flex items-start justify-between">
        <span className="font-mono text-[13px] font-medium text-foreground">
          {identity.external_id}
        </span>
        <span className="font-mono text-[13px] text-foreground">
          {identity.request_count ?? 0} reqs
        </span>
      </div>
      {ratelimits.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {ratelimits.map((rl) => (
            <RateLimitBadge key={rl.name} rl={rl} />
          ))}
        </div>
      )}
      <div className="flex items-center justify-between">
        <div className="flex flex-wrap gap-1.5">
          {metaEntries.map(([k, v]) => (
            <MetaBadge key={k} label={`${k}: ${String(v)}`} />
          ))}
        </div>
        <span className="text-xs text-dim">
          {identity.created_at ? formatDate(identity.created_at) : ""}
        </span>
      </div>
    </Link>
  );
}

export default function IdentitiesPage() {
  const [search, setSearch] = useState("");
  const [modal, setModal] = useState(false);
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];

  const { data: page, isLoading } = $api.useQuery("get", "/v1/identities", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });

  const identities = page?.data ?? [];
  const hasMore = page?.has_more ?? false;
  const pageNumber = cursors.length + 1;

  const filtered = search
    ? identities.filter((id) =>
        (id.external_id ?? "").toLowerCase().includes(search.toLowerCase()),
      )
    : identities;

  const goNext = useCallback(() => {
    if (page?.next_cursor) {
      setCursors((prev) => [...prev, page.next_cursor!]);
    }
  }, [page]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  const columns: DataTableColumn<IdentityResponse>[] = [
    {
      id: "external_id",
      header: "External ID",
      width: "20%",
      cellClassName: "font-mono text-[13px] text-foreground",
      cell: (row) => (
        <Link
          href={`/dashboard/identities/${row.id}`}
          className="hover:underline"
        >
          {row.external_id}
        </Link>
      ),
    },
    {
      id: "ratelimits",
      header: "Rate Limits",
      width: "28%",
      cell: (row) => {
        const ratelimits = row.ratelimits ?? [];
        return ratelimits.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {ratelimits.map((rl) => (
              <RateLimitBadge key={rl.name} rl={rl} />
            ))}
          </div>
        ) : (
          <span className="text-[13px] text-dim">&mdash;</span>
        );
      },
    },
    {
      id: "requests",
      header: "Requests",
      width: "10%",
      cellClassName: "font-mono text-[13px] text-foreground",
      cell: (row) => row.request_count ?? 0,
    },
    {
      id: "meta",
      header: "Meta",
      width: "26%",
      cell: (row) => {
        const meta = (row.meta ?? {}) as Record<string, unknown>;
        const entries = Object.entries(meta);
        return entries.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {entries.map(([k, v]) => (
              <MetaBadge key={k} label={`${k}: ${String(v)}`} />
            ))}
          </div>
        ) : (
          <span className="text-[13px] text-dim">&mdash;</span>
        );
      },
    },
    {
      id: "created_at",
      header: "Created",
      width: "16%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.created_at ? formatDate(row.created_at) : ""),
    },
  ];

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
          <Button size="lg" onClick={() => setModal(true)}>Create Identity</Button>
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

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        {isLoading ? (
          <TableSkeleton columns={skeletonColumns} rows={6} />
        ) : identities.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="flex flex-col items-center gap-6 max-w-sm text-center">
              <div className="flex size-16 items-center justify-center rounded-full border border-border bg-card">
                <Users className="size-7 text-dim" />
              </div>
              <div className="flex flex-col gap-2">
                <span className="font-mono text-[15px] font-medium text-foreground">
                  No identities yet
                </span>
                <span className="text-[13px] leading-5 text-muted-foreground">
                  Identities represent your end-users. Attach them to
                  credentials to enforce per-user rate limits and track usage.
                </span>
              </div>
              <Button size="lg" onClick={() => setModal(true)}>
                Create Identity
                <ArrowRight className="ml-1.5 size-3.5" />
              </Button>
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">
              No identities match your search.
            </span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? ""}
            rowClassName="hover:bg-secondary/30"
            mobileCard={(row) => <IdentityMobileCard identity={row} />}
          />
        )}

        {/* Pagination */}
        {!isLoading && identities.length > 0 && (
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

      <CreateIdentityDialog open={modal} onOpenChange={setModal} />
    </>
  );
}
