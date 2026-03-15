"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Search, ChevronLeft, ChevronRight, Cable, ArrowRight } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog } from "@/components/ui/dialog";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { ProviderBadge } from "@/components/provider-badge";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api } from "@/api/client";
import { CreateIntegrationDialog } from "./create-integration-dialog";
import { DeleteIntegrationDialog } from "./delete-integration-dialog";
import { IntegrationCreatedDialog } from "./integration-created-dialog";
import { IntegrationMobileCard } from "./integration-mobile-card";
import { ProviderLogo } from "./provider-logo";
import { deleteIntegration } from "./api";
import { formatDate, type IntegrationResponse, type ModalState } from "./utils";

const PAGE_SIZE = 20;

const skeletonColumns = [
  { width: "30%" },
  { width: "20%" },
  { width: "20%" },
  { width: "20%" },
  { width: "10%" },
];

export default function IntegrationsPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];
  const pageNumber = cursors.length + 1;

  const [search, setSearch] = useState("");
  const [modal, setModal] = useState<ModalState>("closed");
  const [deleteTarget, setDeleteTarget] =
    useState<IntegrationResponse | null>(null);
  const [createdResult, setCreatedResult] =
    useState<IntegrationResponse | null>(null);

  const { data: page, isLoading } = $api.useQuery("get", "/v1/integrations", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });

  const integrations = page?.data ?? [];
  const hasMore = page?.has_more ?? false;

  const filtered = integrations.filter((integ) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      (integ.display_name ?? "").toLowerCase().includes(q) ||
      (integ.provider ?? "").toLowerCase().includes(q)
    );
  });

  const goNext = useCallback(() => {
    if (page?.next_cursor) {
      setCursors((prev) => [...prev, page.next_cursor!]);
    }
  }, [page]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteIntegration(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["integrations"] });
      setModal("closed");
      setDeleteTarget(null);
    },
  });

  function handleDelete() {
    if (!deleteTarget) return;
    deleteMutation.mutate(deleteTarget.id!);
  }

  const columns: DataTableColumn<IntegrationResponse>[] = [
    {
      id: "display_name",
      header: "Name",
      width: "35%",
      cellClassName: "text-[13px] font-medium text-foreground",
      cell: (row) => (
        <Link
          href={`/dashboard/integrations/${row.id}`}
          className="flex items-center gap-3"
        >
          <ProviderLogo providerId={row.provider ?? ""} size="size-7" />
          <span>{row.display_name}</span>
        </Link>
      ),
    },
    {
      id: "provider",
      header: "Provider",
      width: "15%",
      cell: (row) => <ProviderBadge provider={row.provider ?? ""} />,
    },
    {
      id: "created_at",
      header: "Created",
      width: "20%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.created_at ? formatDate(row.created_at) : ""),
    },
    {
      id: "updated_at",
      header: "Updated",
      width: "20%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.updated_at ? formatDate(row.updated_at) : ""),
    },
    {
      id: "actions",
      header: "",
      width: "10%",
      cell: (row) => (
        <div className="flex items-center justify-end gap-1">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 text-xs text-muted-foreground hover:text-destructive"
            onClick={() => {
              setDeleteTarget(row);
              setModal("delete-confirm");
            }}
          >
            Delete
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Integrations
        </h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search integrations..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-50 pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg" onClick={() => setModal("create")}>
            Add Integration
          </Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search integrations..."
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
        ) : integrations.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="flex flex-col items-center gap-6 max-w-sm text-center">
              <div className="flex size-16 items-center justify-center rounded-full border border-border bg-card">
                <Cable className="size-7 text-dim" />
              </div>
              <div className="flex flex-col gap-2">
                <span className="font-mono text-[15px] font-medium text-foreground">
                  No integrations yet
                </span>
                <span className="text-[13px] leading-5 text-muted-foreground">
                  Connect third-party services to enable OAuth flows and manage
                  connections for your users.
                </span>
              </div>
              <Button size="lg" onClick={() => setModal("create")}>
                Add Integration
                <ArrowRight className="ml-1.5 size-3.5" />
              </Button>
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">
              No integrations match your search.
            </span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? ""}
            mobileCard={(row) => (
              <IntegrationMobileCard
                integration={row}
                onDelete={() => {
                  setDeleteTarget(row);
                  setModal("delete-confirm");
                }}
              />
            )}
          />
        )}

        {/* Pagination */}
        {!isLoading && integrations.length > 0 && (
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

      {/* Create dialog */}
      <Dialog
        open={modal === "create"}
        onOpenChange={(open) => !open && setModal("closed")}
      >
        <CreateIntegrationDialog
          onCancel={() => setModal("closed")}
          onSuccess={(result) => {
            queryClient.invalidateQueries({ queryKey: ["integrations"] });
            setCreatedResult(result);
            setModal("success");
          }}
        />
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog
        open={modal === "delete-confirm"}
        onOpenChange={(open) => {
          if (!open) {
            setModal("closed");
            setDeleteTarget(null);
          }
        }}
      >
        <DeleteIntegrationDialog
          target={deleteTarget}
          isPending={deleteMutation.isPending}
          onClose={() => {
            setModal("closed");
            setDeleteTarget(null);
          }}
          onConfirm={handleDelete}
        />
      </Dialog>

      {/* Success dialog */}
      <Dialog
        open={modal === "success"}
        onOpenChange={(open) => {
          if (!open) {
            const id = createdResult?.id;
            setModal("closed");
            setCreatedResult(null);
            if (id) router.push(`/dashboard/integrations/${id}`);
          }
        }}
      >
        <IntegrationCreatedDialog
          result={createdResult}
          onClose={() => {
            const id = createdResult?.id;
            setModal("closed");
            setCreatedResult(null);
            if (id) router.push(`/dashboard/integrations/${id}`);
          }}
        />
      </Dialog>
    </>
  );
}
