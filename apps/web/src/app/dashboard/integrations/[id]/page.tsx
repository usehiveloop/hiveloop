"use client";

import { useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  Cable,
  Shield,
  ChevronLeft,
  ChevronRight,
  Copy,
  Check,
  Unplug,
  Pencil,
  Eye,
  EyeOff,
  Trash2,
  CircleAlert,
} from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { $api } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog } from "@/components/ui/dialog";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { ProviderBadge } from "@/components/provider-badge";
import { StatusBadge } from "@/components/status-badge";
import { TableSkeleton } from "@/components/table-skeleton";
import { ProviderLogo } from "../provider-logo";
import { DeleteIntegrationDialog } from "../delete-integration-dialog";
import { CredentialFieldsForm } from "../credential-fields-form";
import { credentialFieldsForAuthMode } from "../credential-config";
import { updateIntegration, deleteIntegration, listProviders } from "../api";
import { formatDate, type ConnectionResponse, type NangoProvider } from "../utils";
import { useQuery } from "@tanstack/react-query";

const PAGE_SIZE = 20;

function StatCard({
  label,
  value,
  subtitle,
  icon: Icon,
}: {
  label: string;
  value: string;
  subtitle?: string;
  icon: typeof Cable;
}) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4 sm:p-5">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wider text-dim">
          {label}
        </span>
        <Icon className="size-4 text-dim" />
      </div>
      <span className="font-mono text-2xl font-medium leading-8.5 tracking-tight text-foreground sm:text-[28px]">
        {value}
      </span>
      {subtitle && <span className="text-xs text-dim">{subtitle}</span>}
    </div>
  );
}

function CopyableRow({
  label,
  value,
  sensitive,
}: {
  label: string;
  value: string;
  sensitive?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const [revealed, setRevealed] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  const displayValue =
    sensitive && !revealed ? "•".repeat(Math.min(value.length, 40)) : value;

  return (
    <div className="flex items-center justify-between gap-4">
      <span className="shrink-0 text-[13px] text-dim">{label}</span>
      <div className="flex items-center gap-1.5">
        <span className="font-mono text-[13px] text-foreground truncate max-w-80">
          {displayValue}
        </span>
        {sensitive && (
          <button
            onClick={() => setRevealed(!revealed)}
            className="shrink-0 text-dim hover:text-foreground"
          >
            {revealed ? (
              <EyeOff className="size-3" />
            ) : (
              <Eye className="size-3" />
            )}
          </button>
        )}
        <button
          onClick={handleCopy}
          className="shrink-0 text-dim hover:text-foreground"
        >
          {copied ? (
            <Check className="size-3 text-success-foreground" />
          ) : (
            <Copy className="size-3" />
          )}
        </button>
      </div>
    </div>
  );
}

function ConnectionMobileCard({ conn }: { conn: ConnectionResponse }) {
  return (
    <div className="flex flex-col gap-2 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <span className="font-mono text-[13px] text-foreground">
          {truncateId(conn.nango_connection_id ?? "")}
        </span>
        <StatusBadge status="Active" />
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {conn.identity_id ? truncateId(conn.identity_id) : "No identity"}
        </span>
        <span>{conn.created_at ? formatDate(conn.created_at) : ""}</span>
      </div>
    </div>
  );
}

function truncateId(id: string): string {
  if (id.length <= 16) return id;
  return `${id.slice(0, 10)}...${id.slice(-4)}`;
}

function LoadingSkeleton() {
  return (
    <>
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="h-4 w-32 animate-pulse bg-secondary" />
        <div className="h-7 w-48 animate-pulse bg-secondary" />
      </header>
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        <div className="flex flex-col gap-3 sm:gap-4 lg:flex-row">
          <div className="flex flex-1 flex-col gap-3 sm:gap-4">
            {Array.from({ length: 2 }).map((_, i) => (
              <div
                key={i}
                className="flex h-[130px] animate-pulse flex-col gap-3 border border-border bg-card p-4 sm:p-5"
              >
                <div className="h-3 w-24 bg-secondary" />
                <div className="h-8 w-16 bg-secondary" />
                <div className="h-3 w-20 bg-secondary" />
              </div>
            ))}
          </div>
          <div className="h-64 flex-1 animate-pulse border border-border bg-card" />
        </div>
        <TableSkeleton
          columns={[
            { width: "35%" },
            { width: "25%" },
            { width: "25%" },
            { width: "15%" },
          ]}
          rows={3}
        />
      </div>
    </>
  );
}

function CredentialsSection({
  config,
  provider,
  integrationId,
}: {
  config: Record<string, unknown>;
  provider: NangoProvider | undefined;
  integrationId: string;
}) {
  const queryClient = useQueryClient();
  const [rotating, setRotating] = useState(false);
  const [credentials, setCredentials] = useState<Record<string, string>>({});

  const credConfig = provider
    ? credentialFieldsForAuthMode(provider.auth_mode)
    : { fields: [] };

  const mutation = useMutation({
    mutationFn: (body: { credentials: Record<string, string> }) =>
      updateIntegration(integrationId, body),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/integrations/{id}"],
      });
      setRotating(false);
      setCredentials({});
    },
  });

  const creds = (config.credentials ?? {}) as Record<string, unknown>;

  function handleRotate() {
    if (!provider) return;
    const body: Record<string, string> = { type: provider.auth_mode };
    for (const f of credConfig.fields) {
      if (credentials[f.key]) {
        body[f.key] = credentials[f.key];
      }
    }
    mutation.mutate({ credentials: body });
  }

  const hasRequiredFields = credConfig.fields
    .filter((f) => !credConfig.optional?.includes(f.key))
    .every((f) => credentials[f.key]);

  return (
    <div className="flex flex-col gap-4 border border-border bg-card p-4 sm:p-5">
      <div className="flex items-center justify-between">
        <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">
          Credentials
        </span>
        {credConfig.fields.length > 0 && !rotating && (
          <button
            onClick={() => setRotating(true)}
            className="flex items-center gap-1 text-xs font-medium text-primary hover:underline"
          >
            <Pencil className="size-3" />
            Rotate
          </button>
        )}
      </div>

      {mutation.error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">
            {mutation.error.message}
          </span>
        </div>
      )}

      {!rotating ? (
        <div className="flex flex-col gap-3.5">
          {typeof creds.client_id === "string" && (
            <CopyableRow label="Client ID" value={creds.client_id} />
          )}
          {typeof creds.client_secret === "string" && (
            <CopyableRow
              label="Client Secret"
              value={creds.client_secret}
              sensitive
            />
          )}
          {typeof creds.scopes === "string" && creds.scopes !== "" && (
            <CopyableRow label="Scopes" value={creds.scopes} />
          )}
          {typeof creds.app_id === "string" && (
            <CopyableRow label="App ID" value={creds.app_id} />
          )}
          {typeof creds.app_link === "string" && (
            <CopyableRow label="App Link" value={creds.app_link} />
          )}
          {typeof creds.private_key === "string" && (
            <CopyableRow label="Private Key" value={creds.private_key} sensitive />
          )}
          {typeof creds.webhook_secret === "string" &&
            creds.webhook_secret !== null && (
              <CopyableRow
                label="Webhook Secret"
                value={creds.webhook_secret as string}
                sensitive
              />
            )}
          {Object.keys(creds).length === 0 && (
            <span className="text-[13px] text-muted-foreground">
              No credentials required for this provider.
            </span>
          )}
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <CredentialFieldsForm
            config={credConfig}
            values={credentials}
            onChange={(key, value) =>
              setCredentials((prev) => ({ ...prev, [key]: value }))
            }
            idPrefix="rotate-cred"
            placeholderPrefix="Enter new"
          />
          <div className="flex items-center justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setRotating(false);
                setCredentials({});
              }}
              disabled={mutation.isPending}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleRotate}
              disabled={!hasRequiredFields}
              loading={mutation.isPending}
            >
              Save Credentials
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export default function IntegrationDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();

  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];
  const pageNumber = cursors.length + 1;

  const [editingName, setEditingName] = useState(false);
  const [newName, setNewName] = useState("");
  const [showDelete, setShowDelete] = useState(false);

  const { data: integration, isLoading: integLoading } = $api.useQuery(
    "get",
    "/v1/integrations/{id}",
    { params: { path: { id } } },
  );

  const { data: providers = [] } = useQuery<NangoProvider[]>({
    queryKey: ["nango-providers"],
    queryFn: listProviders,
    staleTime: 5 * 60 * 1000,
  });

  const { data: connPage, isLoading: connsLoading } = $api.useQuery(
    "get",
    "/v1/integrations/{id}/connections",
    {
      params: {
        path: { id },
        query: {
          limit: PAGE_SIZE,
          ...(currentCursor ? { cursor: currentCursor } : {}),
        },
      },
    },
  );

  console.log({integration})

  const nameMutation = useMutation({
    mutationFn: (displayName: string) =>
      updateIntegration(id, { display_name: displayName }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/integrations/{id}"],
      });
      setEditingName(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteIntegration(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["integrations"] });
      router.push("/dashboard/integrations");
    },
  });

  const goNext = useCallback(() => {
    if (connPage?.next_cursor) {
      setCursors((prev) => [...prev, connPage.next_cursor!]);
    }
  }, [connPage]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  if (integLoading) return <LoadingSkeleton />;
  if (!integration) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-16">
        <span className="text-sm text-muted-foreground">
          Integration not found.
        </span>
        <Link
          href="/dashboard/integrations"
          className="text-[13px] text-chart-2"
        >
          Back to integrations
        </Link>
      </div>
    );
  }

  const config = (integration.nango_config ?? {}) as Record<string, unknown>;
  const connections = connPage?.data ?? [];
  const hasMore = connPage?.has_more ?? false;
  const provider = providers.find((p) => p.name === integration.provider);

  const connColumns: DataTableColumn<ConnectionResponse>[] = [
    {
      id: "nango_connection_id",
      header: "Connection ID",
      width: "35%",
      cellClassName: "font-mono text-[13px] text-foreground",
      cell: (row) => truncateId(row.nango_connection_id ?? ""),
    },
    {
      id: "identity_id",
      header: "Identity",
      width: "25%",
      cellClassName: "font-mono text-[13px] text-muted-foreground",
      cell: (row) => (row.identity_id ? truncateId(row.identity_id) : "—"),
    },
    {
      id: "created_at",
      header: "Created",
      width: "25%",
      cellClassName: "text-[13px] text-muted-foreground",
      cell: (row) => (row.created_at ? formatDate(row.created_at) : ""),
    },
    {
      id: "status",
      header: "Status",
      width: "15%",
      cell: () => <StatusBadge status="Active" />,
    },
  ];

  return (
    <>
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link
            href="/dashboard/integrations"
            className="text-[13px] text-dim hover:text-foreground"
          >
            Integrations
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">
            {integration.display_name}
          </span>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <ProviderLogo
              providerId={integration.provider ?? ""}
              size="size-8"
            />
            {editingName ? (
              <div className="flex items-center gap-2">
                <Input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="h-8 w-60 font-mono text-[15px]"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && newName.trim()) {
                      nameMutation.mutate(newName.trim());
                    }
                    if (e.key === "Escape") setEditingName(false);
                  }}
                />
                <Button
                  size="sm"
                  className="h-8"
                  onClick={() => nameMutation.mutate(newName.trim())}
                  disabled={!newName.trim()}
                  loading={nameMutation.isPending}
                >
                  Save
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8"
                  onClick={() => setEditingName(false)}
                >
                  Cancel
                </Button>
              </div>
            ) : (
              <button
                onClick={() => {
                  setNewName(integration.display_name ?? "");
                  setEditingName(true);
                }}
                className="group flex items-center gap-2"
              >
                <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
                  {integration.display_name}
                </h1>
                <Pencil className="size-3.5 text-dim opacity-0 group-hover:opacity-100" />
              </button>
            )}
            <ProviderBadge provider={integration.provider ?? ""} />
          </div>

          <Button
            variant="outline"
            size="sm"
            className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-destructive hover:border-destructive/30"
            onClick={() => setShowDelete(true)}
          >
            <Trash2 className="size-3" />
            Delete
          </Button>
        </div>
      </header>

      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        <div className="flex flex-col gap-3 sm:gap-4 lg:flex-row lg:items-start">
          <div className="flex flex-1 flex-col gap-3 sm:gap-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:gap-4 *:flex-1">
              <StatCard
                label="Total Connections"
                value={String(connections.length)}
                icon={Cable}
              />
              <StatCard
                label="Auth Mode"
                value={
                  typeof config.auth_mode === "string" ? config.auth_mode : "—"
                }
                icon={Shield}
              />
            </div>

            {/* Configuration */}
            <div className="flex flex-col gap-4 border border-border bg-card p-4 sm:p-5">
              <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">
                Configuration
              </span>
              <div className="flex flex-col gap-3.5">
                {typeof config.callback_url === "string" && (
                  <CopyableRow label="Callback URL" value={config.callback_url} />
                )}
                {typeof config.webhook_url === "string" && (
                  <CopyableRow label="Webhook URL" value={config.webhook_url} />
                )}
                {integration.created_at && (
                  <div className="flex items-center justify-between gap-4">
                    <span className="text-[13px] text-dim">Created</span>
                    <span className="font-mono text-[13px] text-muted-foreground">
                      {formatDate(integration.created_at)}
                    </span>
                  </div>
                )}
                <CopyableRow label="ID" value={integration.id ?? ""} />
              </div>
            </div>
          </div>

          {/* Credentials */}
          <div className="flex-1">
            <CredentialsSection
              config={config}
              provider={provider}
              integrationId={id}
            />
          </div>
        </div>

        <div className="flex flex-col">
          <div className="pb-4">
            <span className="text-sm font-medium text-foreground">
              Connections
            </span>
          </div>
          {connsLoading ? (
            <TableSkeleton
              columns={[
                { width: "35%" },
                { width: "25%" },
                { width: "25%" },
                { width: "15%" },
              ]}
              rows={3}
            />
          ) : connections.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20">
              <div className="flex flex-col items-center gap-5 max-w-xs text-center">
                <div className="flex size-14 items-center justify-center rounded-full border border-border bg-card">
                  <Unplug className="size-6 text-dim" />
                </div>
                <div className="flex flex-col gap-1.5">
                  <span className="font-mono text-[14px] font-medium text-foreground">
                    No connections yet
                  </span>
                  <span className="text-[13px] leading-5 text-muted-foreground">
                    Connections will appear here when users authenticate via this
                    integration.
                  </span>
                </div>
              </div>
            </div>
          ) : (
            <DataTable
              columns={connColumns}
              data={connections}
              keyExtractor={(row) => row.id ?? ""}
              mobileCard={(row) => <ConnectionMobileCard conn={row} />}
            />
          )}

          {/* Pagination */}
          {!connsLoading && connections.length > 0 && (
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
        </div>
      </div>

      {/* Delete dialog */}
      <Dialog open={showDelete} onOpenChange={(open) => !open && setShowDelete(false)}>
        <DeleteIntegrationDialog
          target={integration}
          isPending={deleteMutation.isPending}
          onClose={() => setShowDelete(false)}
          onConfirm={() => deleteMutation.mutate()}
        />
      </Dialog>
    </>
  );
}
