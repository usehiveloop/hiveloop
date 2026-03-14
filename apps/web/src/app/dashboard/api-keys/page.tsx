"use client";

import { useState, useCallback } from "react";
import { Search, ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Dialog } from "@/components/ui/dialog";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge } from "@/components/status-badge";
import { TableSkeleton } from "@/components/table-skeleton";
import { useQueryClient } from "@tanstack/react-query";
import { $api } from "@/api/client";
import { deriveStatus, formatDate, relativeTime, type APIKeyResponse, type CreateAPIKeyResult, type StatusFilter, type ModalState } from "./utils";
import { CreateAPIKeyDialog } from "./create-api-key-dialog";
import { KeyCreatedDialog } from "./key-created-dialog";
import { RevokeAPIKeyDialog } from "./revoke-api-key-dialog";
import { APIKeyMobileCard } from "./api-key-mobile-card";

const PAGE_SIZE = 20;

const skeletonColumns = [
  { width: "22%" },
  { width: "16%" },
  { width: "18%" },
  { width: "8%" },
  { width: "12%" },
  { width: "14%" },
  { width: "10%" },
];

export default function APIKeysPage() {
  const queryClient = useQueryClient();
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];

  const { data: page, isLoading } = $api.useQuery("get", "/v1/api-keys", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });
  const keys = page?.data ?? [];
  const hasMore = page?.has_more ?? false;
  const pageNumber = cursors.length + 1;

  const goNext = useCallback(() => {
    if (page?.next_cursor) {
      setCursors((prev) => [...prev, page.next_cursor!]);
    }
  }, [page]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);
  const revokeMutation = $api.useMutation("delete", "/v1/api-keys/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["get", "/v1/api-keys"] }),
  });
  const [filter, setFilter] = useState<StatusFilter>("All");
  const [search, setSearch] = useState("");
  const [modal, setModal] = useState<ModalState>("closed");
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResult | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<APIKeyResponse | null>(null);

  const keysWithStatus = keys.map((k) => ({ ...k, status: deriveStatus(k) }));

  const statusCounts: Record<StatusFilter, number> = {
    All: keysWithStatus.length,
    Active: keysWithStatus.filter((k) => k.status === "Active").length,
    Expired: keysWithStatus.filter((k) => k.status === "Expiring").length,
    Revoked: keysWithStatus.filter((k) => k.status === "Revoked").length,
  };

  const filtered = keysWithStatus.filter((key) => {
    if (filter === "Active" && key.status !== "Active") return false;
    if (filter === "Expired" && key.status !== "Expiring") return false;
    if (filter === "Revoked" && key.status !== "Revoked") return false;
    if (search && !(key.name ?? "").toLowerCase().includes(search.toLowerCase()) && !(key.key_prefix ?? "").toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  function handleRevoke() {
    if (!revokeTarget?.id) return;
    revokeMutation.mutate(
      { params: { path: { id: revokeTarget.id } } },
      {
        onSuccess: () => {
          setModal("closed");
          setRevokeTarget(null);
        },
      },
    );
  }

  const columns: DataTableColumn<(typeof keysWithStatus)[0]>[] = [
    {
      id: "name",
      header: "Name",
      width: "22%",
      cellClassName: "text-[13px] font-medium text-foreground",
      cell: (row) => row.name ?? "",
    },
    {
      id: "key_prefix",
      header: "Key",
      width: "16%",
      cellClassName: "font-mono text-[13px] text-foreground",
      cell: (row) => <span>{row.key_prefix}...&bull;&bull;&bull;&bull;</span>,
    },
    {
      id: "scopes",
      header: "Scopes",
      width: "18%",
      cell: (row) => (
        <div className="flex flex-wrap gap-1">
          {(row.scopes ?? []).map((s) => (
            <Badge key={s} variant="outline" className="text-[11px] font-normal">
              {s}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      width: "8%",
      cell: (row) => <StatusBadge status={row.status} />,
    },
    {
      id: "last_used",
      header: "Last Used",
      width: "12%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.last_used_at ? relativeTime(row.last_used_at) : "Never"),
    },
    {
      id: "created",
      header: "Created",
      width: "14%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => row.created_at ? formatDate(row.created_at) : "",
    },
    {
      id: "actions",
      header: "",
      width: "10%",
      cell: (row) =>
        row.status !== "Revoked" ? (
          <Button
            variant="ghost"
            size="sm"
            className="h-7 text-xs text-muted-foreground hover:text-destructive"
            onClick={() => {
              setRevokeTarget(row);
              setModal("revoke-confirm");
            }}
          >
            Revoke
          </Button>
        ) : null,
    },
  ];

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">API Keys</h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search keys..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-45 pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg" onClick={() => setModal("create")}>Create API Key</Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search keys..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 flex-wrap items-center gap-3 px-4 pt-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-1">
          {(["All", "Active", "Expired", "Revoked"] as StatusFilter[]).map((tab) => (
            <button
              key={tab}
              onClick={() => setFilter(tab)}
              className={`px-3 py-1.5 text-[13px] font-medium transition-colors ${
                filter === tab ? "bg-primary/8 text-chart-2" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              {tab} ({statusCounts[tab]})
            </button>
          ))}
        </div>
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        {isLoading ? (
          <TableSkeleton columns={skeletonColumns} rows={6} />
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">
              {keys.length === 0 ? "No API keys yet. Create one to get started." : "No keys match your filters."}
            </span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? ""}
            mobileCard={(row) => <APIKeyMobileCard apiKey={row} onRevoke={() => { setRevokeTarget(row); setModal("revoke-confirm"); }} />}
          />
        )}

        {/* Pagination */}
        {!isLoading && keys.length > 0 && (
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

      {/* Dialogs */}
      <Dialog open={modal === "create"} onOpenChange={(open) => !open && setModal("closed")}>
        <CreateAPIKeyDialog
          onCancel={() => setModal("closed")}
          onSuccess={(result) => {
            setCreatedKey(result);
            setModal("success");
          }}
        />
      </Dialog>

      <Dialog open={modal === "success"} onOpenChange={(open) => !open && setModal("closed")}>
        <KeyCreatedDialog keyResult={createdKey} onClose={() => setModal("closed")} />
      </Dialog>

      <Dialog open={modal === "revoke-confirm"} onOpenChange={(open) => !open && setModal("closed")}>
        <RevokeAPIKeyDialog
          target={revokeTarget}
          isPending={revokeMutation.isPending}
          onClose={() => setModal("closed")}
          onConfirm={handleRevoke}
        />
      </Dialog>
    </>
  );
}
