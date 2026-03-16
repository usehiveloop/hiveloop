"use client";

import { useState, useCallback } from "react";
import { Search, X, Copy, Check, CircleAlert, ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge, type Status } from "@/components/status-badge";
import { RemainingBar, RemainingBarCompact } from "@/components/remaining-bar";

type StatusFilter = "All" | "Active" | "Expiring" | "Revoked";

type Token = {
  jti: string;
  credential: { name: string; id: string };
  status: Status;
  remaining: { current: string; max: string; percent: number } | null;
  expires: string;
  created: string;
};

const tokens: Token[] = [
  { jti: "ptok_a8f2…3e91", credential: { name: "prod-openai-main", id: "9f2a…b4c1" }, status: "Active", remaining: { current: "450", max: "1,000", percent: 45 }, expires: "in 23 hours", created: "Mar 8, 2026" },
  { jti: "ptok_c7d1…9b42", credential: { name: "staging-anthropic", id: "4b1c…e7a2" }, status: "Active", remaining: null, expires: "in 6 days", created: "Mar 3, 2026" },
  { jti: "ptok_e4b8…1f73", credential: { name: "prod-openai-main", id: "9f2a…b4c1" }, status: "Expiring", remaining: { current: "12", max: "150", percent: 8 }, expires: "in 47 minutes", created: "Mar 9, 2026" },
  { jti: "ptok_f291…8d04", credential: { name: "prod-gemini-flash", id: "d83f…1a9e" }, status: "Active", remaining: null, expires: "in 12 days", created: "Feb 25, 2026" },
  { jti: "ptok_3b7a…c812", credential: { name: "staging-anthropic", id: "4b1c…e7a2" }, status: "Revoked", remaining: null, expires: "expired", created: "Feb 14, 2026" },
  { jti: "ptok_d5e3…2a67", credential: { name: "azure-openai-east", id: "5f18…e3d7" }, status: "Active", remaining: { current: "4.5k", max: "5,000", percent: 90 }, expires: "in 3 days", created: "Mar 6, 2026" },
];

const statusCounts: Record<StatusFilter, number> = { All: 47, Active: 38, Expiring: 4, Revoked: 5 };

const credentialOptions = [
  { name: "prod-openai-main", id: "cred_9f2a…b4c1", provider: "openai" },
  { name: "staging-anthropic", id: "cred_4b1c…e7a2", provider: "anthropic" },
  { name: "prod-gemini-flash", id: "cred_d83f…1a9e", provider: "google" },
  { name: "azure-openai-east", id: "cred_2f8d…a1b3", provider: "azure" },
  { name: "mistral-large-prod", id: "cred_5c4a…f8e7", provider: "mistral" },
];

// Mock available scopes data (will be fetched from API)
type AvailableScopeAction = {
  key: string;
  display_name: string;
  description: string;
  resource_type?: string;
};

type AvailableScopeResourceItem = { id: string; name: string };
type AvailableScopeResource = { display_name: string; selected: AvailableScopeResourceItem[] };

type AvailableScopeConnection = {
  connection_id: string;
  integration_id: string;
  provider: string;
  display_name: string;
  actions: AvailableScopeAction[];
  resources?: Record<string, AvailableScopeResource>;
};

const mockAvailableScopes: AvailableScopeConnection[] = [
  {
    connection_id: "conn-1",
    integration_id: "int-1",
    provider: "slack",
    display_name: "Slack (Engineering)",
    actions: [
      { key: "send_message", display_name: "Send Message", description: "Send a message to a channel", resource_type: "channel" },
      { key: "read_messages", display_name: "Read Messages", description: "Read messages from a channel", resource_type: "channel" },
      { key: "list_channels", display_name: "List Channels", description: "List all channels in the workspace" },
    ],
    resources: {
      channel: {
        display_name: "Channels",
        selected: [
          { id: "C123", name: "engineering" },
          { id: "C456", name: "general" },
          { id: "C789", name: "alerts" },
        ],
      },
    },
  },
  {
    connection_id: "conn-2",
    integration_id: "int-2",
    provider: "github-app",
    display_name: "GitHub (Org)",
    actions: [
      { key: "list_repos", display_name: "List Repositories", description: "List repositories" },
      { key: "list_issues", display_name: "List Issues", description: "List issues in a repository", resource_type: "repo" },
      { key: "create_issue", display_name: "Create Issue", description: "Create an issue", resource_type: "repo" },
    ],
    resources: {
      repo: {
        display_name: "Repositories",
        selected: [
          { id: "org/api", name: "api" },
          { id: "org/web", name: "web" },
        ],
      },
    },
  },
];

const columns: DataTableColumn<Token>[] = [
  {
    id: "jti",
    header: "JTI",
    width: "19%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => row.jti,
  },
  {
    id: "credential",
    header: "Credential",
    width: "17%",
    cell: (row) => (
      <div className="flex flex-col">
        <span className="text-[13px] font-medium leading-4 text-foreground">{row.credential.name}</span>
        <span className="font-mono text-[11px] leading-3.5 text-dim">{row.credential.id}</span>
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
    id: "remaining",
    header: "Remaining",
    width: "16%",
    cell: (row) =>
      row.remaining ? (
        <RemainingBar {...row.remaining} />
      ) : row.status === "Revoked" ? (
        <span className="text-xs text-dim">—</span>
      ) : (
        <span className="text-xs text-dim">Unlimited</span>
      ),
  },
  {
    id: "expires",
    header: "Expires",
    width: "14%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.expires,
  },
  {
    id: "created",
    header: "Created",
    width: "26%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.created,
  },
];

function TokenMobileCard({ token }: { token: Token }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="font-mono text-[13px] text-foreground">{token.jti}</span>
          <span className="text-[13px] text-muted-foreground">{token.credential.name}</span>
        </div>
        <StatusBadge status={token.status} />
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Expires {token.expires}</span>
        {token.remaining ? (
          <RemainingBarCompact {...token.remaining} />
        ) : token.status === "Revoked" ? (
          <span className="text-dim">—</span>
        ) : (
          <span className="text-dim">Unlimited</span>
        )}
      </div>
    </div>
  );
}

type ModalState = "closed" | "mint" | "success";

// Scope selection state
type ScopeSelection = {
  connectionId: string;
  actions: string[];
  resources: Record<string, string[]>;
};

// Minted token result
type MintResult = {
  token: string;
  jti: string;
  expiresAt: string;
  mcpEndpoint?: string;
  credentialName: string;
  ttl: string;
};

export default function TokensPage() {
  const [filter, setFilter] = useState<StatusFilter>("All");
  const [search, setSearch] = useState("");
  const [credentialFilter, setCredentialFilter] = useState<string | null>(null);
  const [modal, setModal] = useState<ModalState>("closed");
  const [mintResult, setMintResult] = useState<MintResult | null>(null);

  const credentialNames = [...new Set(tokens.map((t) => t.credential.name))];

  const filtered = tokens.filter((tok) => {
    if (filter !== "All" && tok.status !== filter) return false;
    if (credentialFilter && tok.credential.name !== credentialFilter) return false;
    if (search && !tok.jti.toLowerCase().includes(search.toLowerCase()) && !tok.credential.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  const handleMintSuccess = useCallback((result: MintResult) => {
    setMintResult(result);
    setModal("success");
  }, []);

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
        <div className="flex items-center gap-1">
          {(["All", "Active", "Expiring", "Revoked"] as StatusFilter[]).map((tab) => (
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
        <div className="hidden h-5 w-px bg-border sm:block" />
        <div className="hidden items-center gap-2 sm:flex">
          <Select value={credentialFilter ?? ""} onValueChange={(v) => setCredentialFilter(v || null)}>
            <SelectTrigger className="h-8 text-[13px] text-muted-foreground">
              <SelectValue placeholder="Credential" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">All Credentials</SelectItem>
              {credentialNames.map((name) => (
                <SelectItem key={name} value={name}>{name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        <DataTable
          columns={columns}
          data={filtered}
          keyExtractor={(row) => row.jti}
          mobileCard={(row) => <TokenMobileCard token={row} />}
        />
      </section>

      {/* Mint Token Dialog */}
      <Dialog open={modal === "mint"} onOpenChange={(open) => !open && setModal("closed")}>
        <MintTokenForm onCancel={() => setModal("closed")} onSuccess={handleMintSuccess} />
      </Dialog>

      {/* Mint Success Dialog */}
      <Dialog open={modal === "success"} onOpenChange={(open) => !open && setModal("closed")}>
        <MintSuccessContent result={mintResult} onClose={() => setModal("closed")} />
      </Dialog>
    </>
  );
}

// --- Scope Selection Component ---

function ScopeSelector({
  scopes,
  onScopesChange,
}: {
  scopes: ScopeSelection[];
  onScopesChange: (scopes: ScopeSelection[]) => void;
}) {
  const availableConnections = mockAvailableScopes;
  const [expandedConns, setExpandedConns] = useState<Set<string>>(new Set());

  const toggleConnection = (connId: string) => {
    const existing = scopes.find((s) => s.connectionId === connId);
    if (existing) {
      onScopesChange(scopes.filter((s) => s.connectionId !== connId));
    } else {
      const conn = availableConnections.find((c) => c.connection_id === connId);
      if (conn) {
        onScopesChange([...scopes, {
          connectionId: connId,
          actions: conn.actions.map((a) => a.key),
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

  return (
    <div className="flex flex-col gap-2">
      {availableConnections.map((conn) => {
        const isSelected = scopes.some((s) => s.connectionId === conn.connection_id);
        const isExpanded = expandedConns.has(conn.connection_id);
        const scopeEntry = scopes.find((s) => s.connectionId === conn.connection_id);

        return (
          <div key={conn.connection_id} className="border border-border">
            {/* Connection header */}
            <div className="flex items-center gap-2 px-3 py-2.5">
              <input
                type="checkbox"
                checked={isSelected}
                onChange={() => toggleConnection(conn.connection_id)}
                className="size-3.5 accent-primary"
              />
              <button
                onClick={() => toggleExpanded(conn.connection_id)}
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

            {/* Expanded: actions + resources */}
            {isExpanded && isSelected && (
              <div className="border-t border-border px-3 py-2.5">
                {/* Actions */}
                <div className="flex flex-col gap-1.5">
                  <span className="text-[11px] font-medium uppercase tracking-wider text-dim">Actions</span>
                  {conn.actions.map((action) => (
                    <label key={action.key} className="flex items-center gap-2 py-0.5">
                      <input
                        type="checkbox"
                        checked={scopeEntry?.actions.includes(action.key) ?? false}
                        onChange={() => toggleAction(conn.connection_id, action.key)}
                        className="size-3 accent-primary"
                      />
                      <span className="text-[13px] text-foreground">{action.display_name}</span>
                      <span className="text-[11px] text-dim">{action.description}</span>
                    </label>
                  ))}
                </div>

                {/* Resources */}
                {conn.resources && Object.entries(conn.resources).map(([type, res]) => (
                  <div key={type} className="mt-3 flex flex-col gap-1.5">
                    <span className="text-[11px] font-medium uppercase tracking-wider text-dim">{res.display_name}</span>
                    {res.selected.length === 0 ? (
                      <span className="text-[11px] text-dim">No resources configured</span>
                    ) : (
                      <div className="flex flex-wrap gap-1.5">
                        {res.selected.map((item) => {
                          const isActive = scopeEntry?.resources[type]?.includes(item.id) ?? false;
                          return (
                            <button
                              key={item.id}
                              onClick={() => toggleResource(conn.connection_id, type, item.id)}
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
        <p className="text-[13px] text-dim">No connections with available actions.</p>
      )}
    </div>
  );
}

// --- Mint Token Form ---

function MintTokenForm({ onCancel, onSuccess }: { onCancel: () => void; onSuccess: (result: MintResult) => void }) {
  const [credential, setCredential] = useState(credentialOptions[0].id);
  const [ttl, setTtl] = useState("1h");
  const [remaining, setRemaining] = useState("");
  const [refillAmount, setRefillAmount] = useState("");
  const [refillInterval, setRefillInterval] = useState("");
  const [metadata, setMetadata] = useState("{ }");
  const [scopeSelections, setScopeSelections] = useState<ScopeSelection[]>([]);
  const [showScopes, setShowScopes] = useState(false);

  const handleMint = () => {
    // TODO: Replace with real API call
    const credName = credentialOptions.find((c) => c.id === credential)?.name ?? "unknown";
    const hasMCP = scopeSelections.length > 0;
    onSuccess({
      token: "ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJvcmdfaWQiOiI5ZjJhLi4uYjRjMSIsImlhdCI6MTcxMDk1NzIwMH0.aI5ZjJh…",
      jti: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      expiresAt: new Date(Date.now() + 3600000).toISOString(),
      mcpEndpoint: hasMCP ? `https://mcp.llmvault.dev/a1b2c3d4-e5f6-7890-abcd-ef1234567890` : undefined,
      credentialName: credName,
      ttl,
    });
  };

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
              {credentialOptions.map((c) => (
                <SelectItem key={c.id} value={c.id}>{c.name} — {c.provider}</SelectItem>
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
            <Input id="remaining" value={remaining} onChange={(e) => setRemaining(e.target.value)} className="h-10" placeholder="No limit" />
            <span className="text-[11px] text-dim">Optional request cap.</span>
          </div>
        </div>

        {/* Refill Amount + Refill Interval */}
        <div className="flex gap-3">
          <div className="flex flex-1 flex-col gap-1.5">
            <Label htmlFor="refillAmount" className="text-xs">Refill Amount</Label>
            <Input id="refillAmount" value={refillAmount} onChange={(e) => setRefillAmount(e.target.value)} className="h-10" placeholder="—" />
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
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
        <Button onClick={handleMint}>Mint Token</Button>
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
      {/* Header */}
      <div className="flex items-start justify-between">
        <div className="flex items-start gap-3">
          <Badge variant="outline" className="flex size-8 items-center justify-center border-success/20 bg-success/10 p-0">
            <Check className="size-4 text-success-foreground" />
          </Badge>
          <DialogHeader className="space-y-0.5">
            <DialogTitle className="font-mono text-lg font-semibold">Token Minted</DialogTitle>
            <DialogDescription className="text-[13px]">
              Scoped to {result.credentialName} &middot; Expires in {result.ttl}
            </DialogDescription>
          </DialogHeader>
        </div>
        <button onClick={onClose} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </div>

      {/* Warning */}
      <div className="flex items-center gap-2 border border-warning/[0.13] bg-warning/5 px-3 py-2.5">
        <CircleAlert className="size-3.5 shrink-0 text-warning-foreground" />
        <span className="text-xs text-warning-foreground">This token is shown only once. Copy it now — you won&apos;t be able to see it again.</span>
      </div>

      {/* Your Token */}
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

      {/* MCP Endpoint (shown when scopes are present) */}
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

      {/* Quick Start (proxy) */}
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

          {/* Base URL */}
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
