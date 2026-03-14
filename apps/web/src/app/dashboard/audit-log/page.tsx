"use client";

import { useState, useCallback } from "react";
import { Search, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api } from "@/api/client";
import type { components } from "@/api/schema";

type AuditEntry = components["schemas"]["auditEntryResponse"];
type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
type ActionFilter = "All" | "Proxy" | "Management";

const PAGE_SIZE = 50;

const actionQueryMap: Record<ActionFilter, string | undefined> = {
  All: undefined,
  Proxy: "proxy.request",
  Management: "api.request",
};

const methodConfig: Record<string, { bg: string; text: string }> = {
  GET: { bg: "#3B82F614", text: "#3B82F6" },
  POST: { bg: "#22C55E14", text: "#22C55E" },
  PUT: { bg: "#F59E0B14", text: "#F59E0B" },
  PATCH: { bg: "#F59E0B14", text: "#F59E0B" },
  DELETE: { bg: "#EF444414", text: "#EF4444" },
};

function MethodBadge({ method }: { method: string }) {
  const config = methodConfig[method] ?? { bg: "#71717A14", text: "#71717A" };
  return (
    <span
      className="inline-block rounded-lg px-2.5 py-0.75 text-[11px] font-semibold"
      style={{ backgroundColor: config.bg, color: config.text }}
    >
      {method}
    </span>
  );
}

function statusColor(status: number): string {
  if (status >= 200 && status < 300) return "#22C55E";
  if (status >= 400 && status < 500) return "#F59E0B";
  return "#EF4444";
}

function formatTimestamp(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatLatency(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${ms}ms`;
}

function truncateId(id: string): string {
  if (id.length <= 12) return id;
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

const skeletonColumns = [
  { width: "14%" },
  { width: "14%" },
  { width: "8%" },
  { width: "26%" },
  { width: "8%" },
  { width: "10%" },
  { width: "20%" },
];

const columns: DataTableColumn<AuditEntry>[] = [
  {
    id: "timestamp",
    header: "Timestamp",
    width: "14%",
    cellClassName: "text-[13px] text-foreground",
    cell: (row) => row.created_at ? formatTimestamp(row.created_at) : "",
  },
  {
    id: "credential",
    header: "Credential",
    width: "14%",
    cellClassName: "font-mono text-xs text-muted-foreground",
    cell: (row) => row.credential_id ? truncateId(row.credential_id) : "—",
  },
  {
    id: "method",
    header: "Method",
    width: "8%",
    cell: (row) => row.method ? <MethodBadge method={row.method} /> : "—",
  },
  {
    id: "path",
    header: "Path",
    width: "26%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => row.path ?? "",
  },
  {
    id: "status",
    header: "Status",
    width: "8%",
    cell: (row) =>
      row.status ? (
        <span className="font-mono text-[13px]" style={{ color: statusColor(row.status) }}>
          {row.status}
        </span>
      ) : (
        "—"
      ),
  },
  {
    id: "latency",
    header: "Latency",
    width: "10%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => (row.latency_ms != null ? formatLatency(row.latency_ms) : "—"),
  },
  {
    id: "ipAddress",
    header: "IP Address",
    width: "20%",
    cellClassName: "font-mono text-[13px] text-dim",
    cell: (row) => row.ip_address ?? "—",
  },
];

function MobileCard({ event }: { event: AuditEntry }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-1">
          <span className="text-[13px] text-foreground">
            {event.created_at ? formatTimestamp(event.created_at) : ""}
          </span>
          <span className="font-mono text-[11px] text-muted-foreground">
            {event.credential_id ? truncateId(event.credential_id) : "—"}
          </span>
        </div>
        {event.method && <MethodBadge method={event.method} />}
      </div>
      <span className="font-mono text-[13px] text-foreground">{event.path}</span>
      <div className="flex items-center justify-between text-xs">
        {event.status ? (
          <span className="font-mono" style={{ color: statusColor(event.status) }}>
            {event.status}
          </span>
        ) : (
          <span>—</span>
        )}
        <span className="font-mono text-muted-foreground">
          {event.latency_ms != null ? formatLatency(event.latency_ms) : "—"}
        </span>
        <span className="font-mono text-dim">{event.ip_address ?? "—"}</span>
      </div>
    </div>
  );
}

export default function AuditLogPage() {
  const [filter, setFilter] = useState<ActionFilter>("All");
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];

  const actionQuery = actionQueryMap[filter];

  const { data, isLoading } = $api.useQuery("get", "/v1/audit", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
        ...(actionQuery ? { action: actionQuery } : {}),
      },
    },
  });

  const entries = data?.data ?? [];
  const hasMore = data?.has_more ?? false;
  const pageNumber = cursors.length + 1;

  const filtered = search
    ? entries.filter(
        (e) =>
          (e.path ?? "").toLowerCase().includes(search.toLowerCase()) ||
          (e.credential_id ?? "").toLowerCase().includes(search.toLowerCase()) ||
          (e.ip_address ?? "").includes(search),
      )
    : entries;

  const goNext = useCallback(() => {
    if (data?.next_cursor) {
      setCursors((prev) => [...prev, data.next_cursor!]);
    }
  }, [data]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  // Reset pagination when filter changes
  const setFilterAndReset = useCallback((f: ActionFilter) => {
    setFilter(f);
    setCursors([]);
  }, []);

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-5 sm:px-6 lg:px-8">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Audit Log
        </h1>
        <div className="flex items-center gap-2">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3.5 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search events..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-[145px] pl-9 font-mono text-[13px]"
            />
          </div>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search events..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 items-center gap-2 px-4 pt-4 sm:px-6 lg:px-8">
        {(["All", "Proxy", "Management"] as ActionFilter[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setFilterAndReset(tab)}
            className={`px-3 py-1.5 text-xs font-medium transition-colors ${
              filter === tab
                ? "bg-primary/8 text-chart-2"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab}
          </button>
        ))}
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        {isLoading ? (
          <TableSkeleton columns={skeletonColumns} rows={8} />
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">
              {entries.length === 0 ? "No audit log entries yet." : "No entries match your search."}
            </span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => String(row.id ?? "")}
            mobileCard={(row) => <MobileCard event={row} />}
            minWidth={1048}
          />
        )}

        {/* Pagination */}
        {!isLoading && entries.length > 0 && (
          <div className="mt-4 flex items-center justify-between border-t border-border pt-4">
            <span className="text-[13px] text-muted-foreground">Page {pageNumber}</span>
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
