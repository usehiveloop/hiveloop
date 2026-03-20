"use client";

import { useState, useCallback, useRef, useMemo } from "react";
import Link from "next/link";
import { Search, X, Copy, Check, CircleAlert, ChevronDown, ChevronRight, ChevronLeft, Coins, Shield } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge, type Status } from "@/components/status-badge";
import { RemainingBar, RemainingBarCompact } from "@/components/remaining-bar";
import { TableSkeleton } from "@/components/table-skeleton";
import { $api, fetchClient } from "@/api/client";
import type { components } from "@/api/schema";

type TokenListItem = components["schemas"]["tokenListItem"];

const PAGE_SIZE = 20;

function deriveTokenStatus(t: TokenListItem): Status {
  if (t.revoked_at) return "Revoked";
  if (t.expires_at && new Date(t.expires_at) < new Date()) return "Revoked";
  if (t.remaining != null && t.remaining <= 0) return "Expiring";
  return "Active";
}

function relativeTime(dateStr: string): string {
  const diff = new Date(dateStr).getTime() - Date.now();
  if (diff < 0) return "expired";
  if (diff < 60_000) return "< 1m";
  if (diff < 3_600_000) return `in ${Math.round(diff / 60_000)}m`;
  if (diff < 86_400_000) return `in ${Math.round(diff / 3_600_000)}h`;
  return `in ${Math.round(diff / 86_400_000)}d`;
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toString();
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function truncateJTI(jti: string): string {
  if (jti.length <= 16) return jti;
  return `${jti.slice(0, 10)}…${jti.slice(-4)}`;
}

function truncateId(id: string): string {
  if (id.length <= 12) return id;
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

type TokenRow = TokenListItem & { status: Status };

const columns: DataTableColumn<TokenRow>[] = [
  {
    id: "jti",
    header: "JTI",
    width: "22%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => truncateJTI(row.jti ?? ""),
  },
  {
    id: "credential",
    header: "Credential",
    width: "15%",
    cell: (row) => (
      <Link href={`/dashboard/credentials/${row.credential_id}`} className="font-mono text-[13px] text-chart-2">
        {truncateId(row.credential_id ?? "")}
      </Link>
    ),
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
    id: "expires",
    header: "Expires",
    width: "14%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.expires_at ? relativeTime(row.expires_at) : "—",
  },
  {
    id: "created",
    header: "Created",
    width: "22%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.created_at ? formatDate(row.created_at) : "—",
  },
];

function TokenMobileCard({ token }: { token: TokenRow }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="font-mono text-[13px] text-foreground">{truncateJTI(token.jti ?? "")}</span>
          <Link href={`/dashboard/credentials/${token.credential_id}`} className="font-mono text-[11px] text-chart-2">
            {truncateId(token.credential_id ?? "")}
          </Link>
        </div>
        <StatusBadge status={token.status} />
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Expires {token.expires_at ? relativeTime(token.expires_at) : "—"}</span>
        {token.remaining != null ? (
          <RemainingBarCompact
            current={formatCount(token.remaining)}
            max={formatCount(token.refill_amount ?? token.remaining)}
            percent={(token.refill_amount ?? token.remaining) > 0 ? Math.round((token.remaining / (token.refill_amount ?? token.remaining)) * 100) : 0}
          />
        ) : token.status === "Revoked" ? (
          <span className="text-dim">—</span>
        ) : (
          <span className="text-dim">Unlimited</span>
        )}
      </div>
    </div>
  );
}

const skeletonColumns = [
  { width: "22%" },
  { width: "15%" },
  { width: "9%" },
  { width: "18%" },
  { width: "14%" },
  { width: "22%" },
];

type ModalState = "closed" | "mint" | "success";

type MintResult = {
  token: string;
  jti: string;
  expiresAt: string;
  mcpEndpoint?: string;
  credentialId: string;
  ttl: string;
};

export default function TokensPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [credentialFilter, setCredentialFilter] = useState<string | null>(null);
  const [cursors, setCursors] = useState<string[]>([]);
  const currentCursor = cursors[cursors.length - 1];
  const [modal, setModal] = useState<ModalState>("closed");
  const [mintResult, setMintResult] = useState<MintResult | null>(null);

  const { data: tokenPage, isLoading } = $api.useQuery("get", "/v1/tokens", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
        ...(credentialFilter ? { credential_id: credentialFilter } : {}),
      },
    },
  });

  const { data: credPage } = $api.useQuery("get", "/v1/credentials", {
    params: { query: { limit: 100 } },
  });

  const allTokens: TokenRow[] = (tokenPage?.data ?? []).map((t) => ({
    ...t,
    status: deriveTokenStatus(t),
  }));

  const filtered = search
    ? allTokens.filter((t) =>
        (t.jti ?? "").toLowerCase().includes(search.toLowerCase()) ||
        (t.credential_id ?? "").toLowerCase().includes(search.toLowerCase())
      )
    : allTokens;

  const hasMore = tokenPage?.has_more ?? false;
  const pageNumber = cursors.length + 1;
  const credentials = credPage?.data ?? [];

  const goNext = useCallback(() => {
    if (tokenPage?.next_cursor) {
      setCursors((prev) => [...prev, tokenPage.next_cursor!]);
    }
  }, [tokenPage]);

  const goPrev = useCallback(() => {
    setCursors((prev) => prev.slice(0, -1));
  }, []);

  const handleMintSuccess = useCallback((result: MintResult) => {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/tokens"] });
    setMintResult(result);
    setModal("success");
  }, [queryClient]);

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Tokens</h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search tokens..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-45 pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg" onClick={() => setModal("mint")}>Mint Token</Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search tokens..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 flex-wrap items-center gap-3 px-4 pt-4 sm:px-6 lg:px-8">
        <div className="hidden items-center gap-2 sm:flex">
          <Select value={credentialFilter ?? ""} onValueChange={(v) => { setCredentialFilter(v || null); setCursors([]); }}>
            <SelectTrigger className="h-8 text-[13px] text-muted-foreground">
              <SelectValue placeholder="All Credentials" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">All Credentials</SelectItem>
              {credentials.map((c) => (
                <SelectItem key={c.id} value={c.id ?? ""}>{c.label || c.id}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        {isLoading ? (
          <TableSkeleton columns={skeletonColumns} rows={6} />
        ) : allTokens.length === 0 && !credentialFilter ? (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="flex flex-col items-center gap-6 max-w-sm text-center">
              <div className="flex size-16 items-center justify-center rounded-full border border-border bg-card">
                <Coins className="size-7 text-dim" />
              </div>
              <div className="flex flex-col gap-2">
                <span className="font-mono text-[15px] font-medium text-foreground">No tokens yet</span>
                <span className="text-[13px] leading-5 text-muted-foreground">
                  Mint short-lived proxy tokens scoped to a credential. Tokens authenticate requests to the LLM proxy.
                </span>
              </div>
              <Button size="lg" onClick={() => setModal("mint")}>Mint Token</Button>
            </div>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16">
            <span className="text-sm text-muted-foreground">No tokens match your search.</span>
          </div>
        ) : (
          <DataTable
            columns={columns}
            data={filtered}
            keyExtractor={(row) => row.id ?? row.jti ?? ""}
            mobileCard={(row) => <TokenMobileCard token={row} />}
          />
        )}

        {/* Pagination */}
        {!isLoading && allTokens.length > 0 && (
          <div className="mt-4 flex items-center justify-between border-t border-border pt-4">
            <span className="text-[13px] text-muted-foreground">Page {pageNumber}</span>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" disabled={cursors.length === 0} onClick={goPrev} className="h-8 gap-1 text-[13px]">
                <ChevronLeft className="size-3.5" />
                Previous
              </Button>
              <Button variant="outline" size="sm" disabled={!hasMore} onClick={goNext} className="h-8 gap-1 text-[13px]">
                Next
                <ChevronRight className="size-3.5" />
              </Button>
            </div>
          </div>
        )}
      </section>

      {/* Mint Token Dialog */}
      <Dialog open={modal === "mint"} onOpenChange={(open) => !open && setModal("closed")}>
        <MintTokenForm
          credentials={credentials}
          onCancel={() => setModal("closed")}
          onSuccess={handleMintSuccess}
        />
      </Dialog>

      {/* Mint Success Dialog */}
      <Dialog open={modal === "success"} onOpenChange={(open) => !open && setModal("closed")}>
        <MintSuccessContent result={mintResult} onClose={() => setModal("closed")} />
      </Dialog>
    </>
  );
}

// --- Scope Selection Component ---

type ScopeSelection = {
  connectionId: string;
  actions: string[];
  resources: Record<string, string[]>;
};

type AvailableScopeConnection = components["schemas"]["availableScopeConnection"];
type ActionItem = components["schemas"]["availableScopeAction"];

const ACTION_ROW_HEIGHT = 44; // Height of each action row in px

function ActionList({
  actions,
  scopeEntry,
  connId,
  searchQuery,
  onToggleAction,
}: {
  actions: ActionItem[];
  scopeEntry: ScopeSelection | undefined;
  connId: string;
  searchQuery: string;
  onToggleAction: (connId: string, actionKey: string) => void;
}) {
  const parentRef = useRef<HTMLDivElement>(null);
  
  const filteredActions = useMemo(() => {
    if (!searchQuery.trim()) return actions;
    const query = searchQuery.toLowerCase();
    return actions.filter(
      (a) =>
        (a.display_name ?? "").toLowerCase().includes(query) ||
        (a.description ?? "").toLowerCase().includes(query) ||
        (a.key ?? "").toLowerCase().includes(query)
    );
  }, [actions, searchQuery]);

  const virtualizer = useVirtualizer({
    count: filteredActions.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ACTION_ROW_HEIGHT,
    overscan: 5,
  });

  const virtualItems = virtualizer.getVirtualItems();

  return (
    <div
      ref={parentRef}
      className="relative h-64 overflow-auto border border-border"
    >
      {filteredActions.length === 0 ? (
        <div className="flex h-full flex-col items-center justify-center">
          <span className="text-[12px] text-muted-foreground">No actions match your search</span>
        </div>
      ) : (
        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            width: "100%",
            position: "relative",
          }}
        >
          {virtualItems.map((virtualItem) => {
            const action = filteredActions[virtualItem.index];
            const isChecked = scopeEntry?.actions.includes(action.key ?? "") ?? false;
            return (
              <div
                key={action.key}
                style={{
                  position: "absolute",
                  top: 0,
                  left: 0,
                  width: "100%",
                  height: `${virtualItem.size}px`,
                  transform: `translateY(${virtualItem.start}px)`,
                }}
                onClick={() => onToggleAction(connId, action.key ?? "")}
                className={`group flex cursor-pointer items-center gap-3 border-b border-border px-3 transition-colors last:border-b-0 ${
                  isChecked
                    ? "bg-primary/[0.04] hover:bg-primary/[0.08]"
                    : "hover:bg-secondary/50"
                }`}
              >
                <Checkbox
                  checked={isChecked}
                  className="size-4 shrink-0"
                />
                <div className="flex min-w-0 flex-1 flex-col py-2">
                  <span className="truncate text-[12px] font-medium text-foreground">
                    {action.display_name}
                  </span>
                  {action.description && (
                    <span className="truncate text-[11px] text-dim">
                      {action.description}
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function ScopeSelector({
  scopes,
  onScopesChange,
}: {
  scopes: ScopeSelection[];
  onScopesChange: (scopes: ScopeSelection[]) => void;
}) {
  const { data: availableConnections = [] } = $api.useQuery("get", "/v1/connections/available-scopes");
  const [expandedConns, setExpandedConns] = useState<Set<string>>(new Set());
  const [searchQueries, setSearchQueries] = useState<Record<string, string>>({});

  const toggleConnection = (connId: string) => {
    const existing = scopes.find((s) => s.connectionId === connId);
    if (existing) {
      onScopesChange(scopes.filter((s) => s.connectionId !== connId));
    } else {
      const conn = availableConnections.find((c: AvailableScopeConnection) => c.connection_id === connId);
      if (conn) {
        onScopesChange([...scopes, {
          connectionId: connId,
          actions: (conn.actions ?? []).map((a) => a.key ?? ""),
          resources: {},
        }]);
      }
    }
  };

  const toggleExpanded = (connId: string) => {
    const next = new Set(expandedConns);
    if (next.has(connId)) next.delete(connId);
    else next.add(connId);
    setExpandedConns(next);
  };

  const toggleAction = (connId: string, actionKey: string) => {
    onScopesChange(scopes.map((s) => {
      if (s.connectionId !== connId) return s;
      const has = s.actions.includes(actionKey);
      return { ...s, actions: has ? s.actions.filter((a) => a !== actionKey) : [...s.actions, actionKey] };
    }));
  };

  const toggleResource = (connId: string, resourceType: string, resourceId: string) => {
    onScopesChange(scopes.map((s) => {
      if (s.connectionId !== connId) return s;
      const current = s.resources[resourceType] ?? [];
      const has = current.includes(resourceId);
      return {
        ...s,
        resources: {
          ...s.resources,
          [resourceType]: has ? current.filter((id) => id !== resourceId) : [...current, resourceId],
        },
      };
    }));
  };

  const handleSearchChange = (connId: string, value: string) => {
    setSearchQueries((prev) => ({ ...prev, [connId]: value }));
  };

  return (
    <div className="flex flex-col gap-2">
      {availableConnections.map((conn: AvailableScopeConnection) => {
        const connId = conn.connection_id ?? "";
        const isSelected = scopes.some((s) => s.connectionId === connId);
        const isExpanded = expandedConns.has(connId);
        const scopeEntry = scopes.find((s) => s.connectionId === connId);
        const searchQuery = searchQueries[connId] ?? "";

        return (
          <div key={connId} className="border border-border">
            <div className="flex items-center gap-2 px-3 py-2.5">
              <Checkbox
                checked={isSelected}
                onCheckedChange={() => toggleConnection(connId)}
                className="size-4"
              />
              <button
                onClick={() => toggleExpanded(connId)}
                className="flex flex-1 items-center gap-1.5 text-left"
              >
                {isExpanded
                  ? <ChevronDown className="size-3.5 text-dim" />
                  : <ChevronRight className="size-3.5 text-dim" />
                }
                <span className="text-[13px] font-medium text-foreground">{conn.display_name}</span>
                <span className="text-[11px] text-dim">{conn.provider}</span>
              </button>
              {isSelected && (
                <span className="text-[11px] text-muted-foreground">
                  {scopeEntry?.actions.length ?? 0} actions
                </span>
              )}
            </div>

            {isExpanded && isSelected && (
              <div className="border-t border-border bg-secondary/20 px-3 py-3">
                {/* Action List with Search */}
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-[11px] font-medium uppercase tracking-wider text-dim">Actions</span>
                  <span className="text-[10px] text-dim">{(conn.actions ?? []).length} total</span>
                </div>

                {/* Search Input */}
                <div className="relative mb-2">
                  <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
                  <Input
                    placeholder="Search actions..."
                    value={searchQuery}
                    onChange={(e) => handleSearchChange(connId, e.target.value)}
                    className="h-8 pl-8 text-[12px]"
                  />
                </div>

                {/* Virtualized Action List */}
                <ActionList
                  actions={conn.actions ?? []}
                  scopeEntry={scopeEntry}
                  connId={connId}
                  searchQuery={searchQuery}
                  onToggleAction={toggleAction}
                />

                {conn.resources && Object.entries(conn.resources).map(([type, res]) => (
                  <div key={type} className="mt-4 flex flex-col gap-1.5">
                    <span className="text-[11px] font-medium uppercase tracking-wider text-dim">{res.display_name}</span>
                    {(res.selected ?? []).length === 0 ? (
                      <span className="text-[11px] text-dim">No resources configured</span>
                    ) : (
                      <div className="flex flex-wrap gap-1.5">
                        {(res.selected ?? []).map((item) => {
                          const isActive = scopeEntry?.resources[type]?.includes(item.id ?? "") ?? false;
                          return (
                            <button
                              key={item.id}
                              onClick={() => toggleResource(connId, type, item.id ?? "")}
                              className={`px-2 py-1 text-[12px] transition-colors ${
                                isActive
                                  ? "border border-primary/30 bg-primary/8 text-foreground"
                                  : "border border-border text-muted-foreground hover:border-primary/20"
                              }`}
                            >
                              {item.name}
                            </button>
                          );
                        })}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}

      {availableConnections.length === 0 && (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <Shield className="mb-2 size-6 text-dim" />
          <p className="text-[13px] text-dim">No connections with available actions.</p>
          <p className="text-[11px] text-muted-foreground">Connect integrations to enable scoped token access.</p>
        </div>
      )}
    </div>
  );
}

// --- Mint Token Form ---

type CredentialOption = components["schemas"]["credentialResponse"];

function MintTokenForm({
  credentials,
  onCancel,
  onSuccess,
}: {
  credentials: CredentialOption[];
  onCancel: () => void;
  onSuccess: (result: MintResult) => void;
}) {
  const activeCredentials = credentials.filter((c) => !c.revoked_at);
  const [credential, setCredential] = useState(activeCredentials[0]?.id ?? "");
  const [ttl, setTtl] = useState("1h");
  const [remaining, setRemaining] = useState("");
  const [refillAmount, setRefillAmount] = useState("");
  const [refillInterval, setRefillInterval] = useState("");
  const [metadata, setMetadata] = useState("{ }");
  const [scopeSelections, setScopeSelections] = useState<ScopeSelection[]>([]);
  const [showScopes, setShowScopes] = useState(false);

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        credential_id: credential,
        ttl,
      };
      if (remaining) body.remaining = Number(remaining);
      if (refillAmount) body.refill_amount = Number(refillAmount);
      if (refillInterval) body.refill_interval = refillInterval;
      if (scopeSelections.length > 0) {
        body.scopes = scopeSelections.map((s) => ({
          connection_id: s.connectionId,
          actions: s.actions,
          resources: s.resources,
        }));
      }
      try {
        const parsed = JSON.parse(metadata);
        if (parsed && typeof parsed === "object" && Object.keys(parsed).length > 0) {
          body.meta = parsed;
        }
      } catch { /* ignore invalid JSON */ }

      const { data, error } = await fetchClient.POST("/v1/tokens", {
        body: body as components["schemas"]["mintTokenRequest"],
      });
      if (error) throw new Error((error as { error?: string }).error ?? "Failed to mint token");
      return data;
    },
    onSuccess: (data) => {
      onSuccess({
        token: data?.token ?? "",
        jti: data?.jti ?? "",
        expiresAt: data?.expires_at ?? "",
        mcpEndpoint: data?.mcp_endpoint ?? undefined,
        credentialId: credential,
        ttl,
      });
    },
  });

  return (
    <DialogContent className="sm:max-w-130 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <DialogTitle className="font-mono text-lg font-semibold">Mint Token</DialogTitle>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Create a short-lived proxy token scoped to a single credential. Tokens use the ptok_ prefix and authenticate proxy requests.
      </DialogDescription>

      {mutation.error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{mutation.error.message}</span>
        </div>
      )}

      <div className="flex flex-col gap-4.5">
        {/* Credential */}
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="credential" className="text-xs">
            Credential <span className="text-destructive">*</span>
          </Label>
          <Select value={credential} onValueChange={(v) => v && setCredential(v)}>
            <SelectTrigger className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {activeCredentials.map((c) => (
                <SelectItem key={c.id} value={c.id ?? ""}>
                  {c.label || c.id}{c.provider_id ? ` — ${c.provider_id}` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* TTL + Remaining */}
        <div className="flex gap-3">
          <div className="flex flex-1 flex-col gap-1.5">
            <Label htmlFor="ttl" className="text-xs">TTL</Label>
            <Input id="ttl" value={ttl} onChange={(e) => setTtl(e.target.value)} className="h-10 font-mono" placeholder="1h" />
            <span className="text-[11px] text-dim">Max 24h. Go duration format.</span>
          </div>
          <div className="flex flex-1 flex-col gap-1.5">
            <Label htmlFor="remaining" className="text-xs">Remaining</Label>
            <Input id="remaining" type="number" value={remaining} onChange={(e) => setRemaining(e.target.value)} className="h-10" placeholder="No limit" />
            <span className="text-[11px] text-dim">Optional request cap.</span>
          </div>
        </div>

        {/* Refill Amount + Refill Interval */}
        <div className="flex gap-3">
          <div className="flex flex-1 flex-col gap-1.5">
            <Label htmlFor="refillAmount" className="text-xs">Refill Amount</Label>
            <Input id="refillAmount" type="number" value={refillAmount} onChange={(e) => setRefillAmount(e.target.value)} className="h-10" placeholder="—" />
          </div>
          <div className="flex flex-1 flex-col gap-1.5">
            <Label htmlFor="refillInterval" className="text-xs">Refill Interval</Label>
            <Input id="refillInterval" value={refillInterval} onChange={(e) => setRefillInterval(e.target.value)} className="h-10" placeholder="—" />
          </div>
        </div>

        {/* Integration Access (Scopes) */}
        <div className="flex flex-col gap-1.5">
          <button
            onClick={() => setShowScopes(!showScopes)}
            className="flex items-center gap-1.5 text-left"
          >
            {showScopes
              ? <ChevronDown className="size-3.5 text-dim" />
              : <ChevronRight className="size-3.5 text-dim" />
            }
            <Label className="cursor-pointer text-xs">Integration Access</Label>
            {scopeSelections.length > 0 && (
              <Badge variant="outline" className="ml-1 text-[10px]">{scopeSelections.length} connections</Badge>
            )}
          </button>
          <span className="text-[11px] text-dim">
            Optional. Grant this token access to integration tools via MCP.
          </span>
          {showScopes && (
            <div className="mt-1">
              <ScopeSelector scopes={scopeSelections} onScopesChange={setScopeSelections} />
            </div>
          )}
        </div>

        {/* Metadata */}
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="metadata" className="text-xs">Metadata</Label>
          <Textarea id="metadata" value={metadata} onChange={(e) => setMetadata(e.target.value)} className="font-mono text-xs" placeholder="{ }" />
          <span className="text-[11px] text-dim">Optional JSON object.</span>
        </div>
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onCancel} disabled={mutation.isPending}>Cancel</Button>
        <Button onClick={() => mutation.mutate()} disabled={!credential} loading={mutation.isPending}>Mint Token</Button>
      </DialogFooter>
    </DialogContent>
  );
}

// --- Mint Success Content ---

function MintSuccessContent({ result, onClose }: { result: MintResult | null; onClose: () => void }) {
  const [copied, setCopied] = useState<string | null>(null);

  if (!result) return null;

  const baseUrl = "https://api.llmvault.dev/v1/proxy";
  const curlCommand = `curl ${baseUrl}/v1/chat/completions \\\n    -H "Authorization: Bearer ${result.token.slice(0, 20)}..." \\\n    -H "Content-Type: application/json" \\\n    -d '{"model":"gpt-4o","messages":[...]}'`;

  function handleCopy(text: string, key: string) {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  }

  return (
    <DialogContent className="sm:max-w-140 gap-6 p-7" showCloseButton={false}>
      <div className="flex items-start justify-between">
        <div className="flex items-start gap-3">
          <Badge variant="outline" className="flex size-8 items-center justify-center border-success/20 bg-success/10 p-0">
            <Check className="size-4 text-success-foreground" />
          </Badge>
          <DialogHeader className="space-y-0.5">
            <DialogTitle className="font-mono text-lg font-semibold">Token Minted</DialogTitle>
            <DialogDescription className="text-[13px]">
              Scoped to {truncateId(result.credentialId)} &middot; Expires in {result.ttl}
            </DialogDescription>
          </DialogHeader>
        </div>
        <button onClick={onClose} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </div>

      <div className="flex items-center gap-2 border border-warning/[0.13] bg-warning/5 px-3 py-2.5">
        <CircleAlert className="size-3.5 shrink-0 text-warning-foreground" />
        <span className="text-xs text-warning-foreground">This token is shown only once. Copy it now — you won&apos;t be able to see it again.</span>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label className="text-xs">Your Token</Label>
        <div className="flex items-center gap-2 border border-border bg-code px-3 py-3">
          <span className="flex-1 break-all font-mono text-xs leading-4 text-foreground">{result.token}</span>
          <Button size="sm" onClick={() => handleCopy(result.token, "token")} className="shrink-0 gap-1.5">
            {copied === "token" ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            {copied === "token" ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      {result.mcpEndpoint && (
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">MCP Endpoint</Label>
          <div className="flex items-center gap-2 border border-border bg-code px-3 py-3">
            <span className="flex-1 break-all font-mono text-xs leading-4 text-chart-2">{result.mcpEndpoint}</span>
            <Button size="sm" onClick={() => handleCopy(result.mcpEndpoint!, "mcp")} className="shrink-0 gap-1.5">
              {copied === "mcp" ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              {copied === "mcp" ? "Copied" : "Copy"}
            </Button>
          </div>
          <div className="mt-1 border border-border bg-code">
            <div className="flex items-center justify-between border-b border-border px-3 py-2">
              <span className="font-mono text-[11px] text-dim">claude_desktop_config.json</span>
              <button
                onClick={() => handleCopy(JSON.stringify({
                  mcpServers: {
                    llmvault: {
                      url: result.mcpEndpoint,
                      headers: { Authorization: `Bearer ${result.token}` },
                    },
                  },
                }, null, 2), "config")}
                className="text-dim hover:text-foreground"
              >
                {copied === "config" ? <Check className="size-3" /> : <Copy className="size-3" />}
              </button>
            </div>
            <div className="px-3 py-3">
              <pre className="font-mono text-xs leading-5 text-muted-foreground">
{JSON.stringify({
  mcpServers: {
    llmvault: {
      url: result.mcpEndpoint,
      headers: { Authorization: `Bearer ${result.token.slice(0, 20)}...` },
    },
  },
}, null, 2)}
              </pre>
            </div>
          </div>
        </div>
      )}

      {!result.mcpEndpoint && (
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">Quick Start</Label>
          <p className="text-[13px] leading-4.5 text-muted-foreground">
            Point your LLM client to the proxy endpoint and authenticate with your token:
          </p>
          <div className="border border-border bg-code">
            <div className="flex items-center justify-between border-b border-border px-3 py-2">
              <span className="font-mono text-[11px] text-dim">curl</span>
              <button onClick={() => handleCopy(curlCommand, "curl")} className="text-dim hover:text-foreground">
                {copied === "curl" ? <Check className="size-3" /> : <Copy className="size-3" />}
              </button>
            </div>
            <div className="px-3 py-3">
              <pre className="font-mono text-xs leading-5 text-muted-foreground">
{`curl ${baseUrl}/v1/chat/completions \\
    -H "Authorization: Bearer ${result.token.slice(0, 20)}..." \\
    -H "Content-Type: application/json" \\
    -d '{"model":"gpt-4o","messages":[...]}'`}
              </pre>
            </div>
          </div>

          <div className="mt-2 flex flex-col gap-1.5 border-t border-border bg-secondary/50 px-3 py-2.5">
            <div className="flex items-center gap-1.5">
              <span className="text-xs font-medium text-foreground">Base URL</span>
              <span className="text-[11px] text-dim">— set this in your SDK client</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="font-mono text-[13px] text-chart-2">{baseUrl}</span>
              <button onClick={() => handleCopy(baseUrl, "url")} className="text-dim hover:text-foreground">
                {copied === "url" ? <Check className="size-3" /> : <Copy className="size-3" />}
              </button>
            </div>
          </div>
        </div>
      )}

      <DialogFooter className="justify-end rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button onClick={onClose}>Done</Button>
      </DialogFooter>
    </DialogContent>
  );
}
