"use client";

import { useState, useCallback } from "react";
import { Search, ChevronLeft, ChevronRight, Timer, Trash2 } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api, fetchClient } from "@/api/client";
import type { components } from "@/api/schema";

type Session = components["schemas"]["connectSessionListItem"];
type StatusFilter = "all" | "active" | "activated" | "expired";

const PAGE_SIZE = 20;

const statusConfig: Record<string, string> = {
  active: "border-success/20 bg-success/10 text-success-foreground",
  activated: "border-info/20 bg-info/10 text-info-foreground",
  expired: "border-destructive/20 bg-destructive/10 text-destructive",
};

function SessionStatusBadge({ status }: { status: string }) {
  return (
    <Badge variant="outline" className={`h-auto text-[11px] capitalize ${statusConfig[status] ?? "text-dim"}`}>
      {status}
    </Badge>
  );
}

function relativeExpiry(expiresAt: string): { text: string; expired: boolean } {
  const diff = new Date(expiresAt).getTime() - Date.now();
  if (diff <= 0) return { text: "expired", expired: true };
  if (diff < 60_000) return { text: `in ${Math.ceil(diff / 1000)}s`, expired: false };
  if (diff < 3_600_000) return { text: `in ${Math.round(diff / 60_000)}m`, expired: false };
  return { text: `in ${Math.round(diff / 3_600_000)}h`, expired: false };
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

function truncateId(id: string): string {
  if (id.length <= 16) return id;
  return `${id.slice(0, 8)}...${id.slice(-4)}`;
}

const skeletonColumns = [
  { width: "17%" },
  { width: "16%" },
  { width: "9%" },
  { width: "22%" },
  { width: "14%" },
  { width: "14%" },
  { width: "8%" },
];

function DeleteSessionButton({ id, onDeleted }: { id: string; onDeleted: () => void }) {
  const [pending, setPending] = useState(false);

  async function handleDelete() {
    setPending(true);
    try {
      await fetchClient.DELETE("/v1/connect/sessions/{id}", { params: { path: { id } } });
      onDeleted();
    } finally {
      setPending(false);
    }
  }

  return (
    <button
      onClick={handleDelete}
      disabled={pending}
      className="text-dim transition-colors hover:text-destructive disabled:opacity-50"
      title="Delete session"
    >
      <Trash2 className="size-3.5" />
    </button>
  );
}

export default function WidgetSessionsPage() {
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [search, setSearch] = useState("");
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];

  const { data, isLoading } = $api.useQuery("get", "/v1/connect/sessions", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(filter !== "all" ? { status: filter } : {}),
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });

  const sessions = data?.data ?? [];
  const hasMore = data?.has_more ?? false;
  const pageNumber = cursors.length + 1;

  const filtered = search
    ? sessions.filter((s) =>
        (s.session_token ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (s.external_id ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (s.identity_id ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (s.id ?? "").toLowerCase().includes(search.toLowerCase())
      )
    : sessions;

  const goNext = useCallback(() => {
    if (data?.next_cursor) {
      setCursors((prev) => [...prev, data.next_cursor!]);
    }
  }, [data]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  function handleFilterChange(f: StatusFilter) {
    setFilter(f);
    setCursors([]);
  }

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/connect/sessions"] });
  }

  const columns: DataTableColumn<Session>[] = [
    {
      id: "token",
      header: "Session",
      width: "17%",
      cell: (row) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] text-foreground">{row.session_token}</span>
          <span className="font-mono text-[11px] text-dim">{truncateId(row.id ?? "")}</span>
        </div>
      ),
    },
    {
      id: "identity",
      header: "Identity",
      width: "16%",
      cell: (row) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] text-muted-foreground">{row.external_id || "—"}</span>
          {row.identity_id && <span className="font-mono text-[11px] text-dim">{truncateId(row.identity_id)}</span>}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      width: "9%",
      cell: (row) => <SessionStatusBadge status={row.status ?? "active"} />,
    },
    {
      id: "permissions",
      header: "Permissions",
      width: "22%",
      cell: (row) => {
        const perms = row.permissions ?? [];
        if (perms.length === 0) return <span className="text-xs text-dim">—</span>;
        return (
          <div className="flex flex-wrap gap-1.5">
            {perms.map((p) => (
              <Badge key={p} variant="secondary" className="h-auto bg-primary/8 font-mono text-[11px] text-chart-2">
                {p}
              </Badge>
            ))}
          </div>
        );
      },
    },
    {
      id: "expires",
      header: "Expires",
      width: "14%",
      cell: (row) => {
        const { text, expired } = relativeExpiry(row.expires_at ?? "");
        return (
          <span className={`text-[13px] ${expired ? "text-dim" : "text-success-foreground"}`}>
            {text}
          </span>
        );
      },
    },
    {
      id: "created",
      header: "Created",
      width: "14%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => row.created_at ? formatDate(row.created_at) : "—",
    },
    {
      id: "actions",
      header: "",
      width: "8%",
      cell: (row) => (
        <div className="flex justify-end">
          <DeleteSessionButton id={row.id ?? ""} onDeleted={invalidate} />
        </div>
      ),
    },
  ];

  function SessionMobileCard({ session }: { session: Session }) {
    const { text: expiryText, expired } = relativeExpiry(session.expires_at ?? "");
    return (
      <div className="flex flex-col gap-3 border border-border bg-card p-4">
        <div className="flex items-start justify-between">
          <div className="flex flex-col">
            <span className="font-mono text-[13px] text-foreground">{session.session_token}</span>
            <span className="font-mono text-[11px] text-dim">{session.external_id || truncateId(session.id ?? "")}</span>
          </div>
          <div className="flex items-center gap-2">
            <SessionStatusBadge status={session.status ?? "active"} />
            <DeleteSessionButton id={session.id ?? ""} onDeleted={invalidate} />
          </div>
        </div>
        {(session.permissions ?? []).length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {(session.permissions ?? []).map((p) => (
              <Badge key={p} variant="secondary" className="h-auto bg-primary/8 font-mono text-[11px] text-chart-2">
                {p}
              </Badge>
            ))}
          </div>
        )}
        <div className="flex items-center justify-between text-xs">
          <span className={expired ? "text-dim" : "text-success-foreground"}>
            {expiryText}
          </span>
          <span className="text-dim">{session.created_at ? formatDate(session.created_at) : ""}</span>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Sessions
        </h1>
        <div className="relative hidden sm:block">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search sessions..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-50 pl-9 font-mono text-[13px]"
          />
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
        {(["all", "active", "activated", "expired"] as StatusFilter[]).map((tab) => (
          <button
            key={tab}
            onClick={() => handleFilterChange(tab)}
            className={`px-3 py-1.5 text-[13px] font-medium capitalize transition-colors ${
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
          <TableSkeleton columns={skeletonColumns} rows={6} />
        ) : sessions.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="flex flex-col items-center gap-6 max-w-sm text-center">
              <div className="flex size-16 items-center justify-center rounded-full border border-border bg-card">
                <Timer className="size-7 text-dim" />
              </div>
              <div className="flex flex-col gap-2">
                <span className="font-mono text-[15px] font-medium text-foreground">
                  No sessions yet
                </span>
                <span className="text-[13px] leading-5 text-muted-foreground">
                  Sessions are created via the API when embedding the Connect widget. Each session is short-lived and scoped to a specific identity.
                </span>
              </div>
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">No sessions match your search.</span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? ""}
            rowClassName="hover:bg-secondary/30"
            mobileCard={(row) => <SessionMobileCard session={row} />}
          />
        )}

        {/* Pagination */}
        {!isLoading && sessions.length > 0 && (
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
