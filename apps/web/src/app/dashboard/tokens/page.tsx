"use client";

import { useState } from "react";
import { Search, X, Copy, Check, CircleAlert } from "lucide-react";
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

export default function TokensPage() {
  const [filter, setFilter] = useState<StatusFilter>("All");
  const [search, setSearch] = useState("");
  const [credentialFilter, setCredentialFilter] = useState<string | null>(null);
  const [modal, setModal] = useState<ModalState>("closed");

  const credentialNames = [...new Set(tokens.map((t) => t.credential.name))];

  const filtered = tokens.filter((tok) => {
    if (filter !== "All" && tok.status !== filter) return false;
    if (credentialFilter && tok.credential.name !== credentialFilter) return false;
    if (search && !tok.jti.toLowerCase().includes(search.toLowerCase()) && !tok.credential.name.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

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
        <MintTokenForm onCancel={() => setModal("closed")} onSuccess={() => setModal("success")} />
      </Dialog>

      {/* Mint Success Dialog */}
      <Dialog open={modal === "success"} onOpenChange={(open) => !open && setModal("closed")}>
        <MintSuccessContent onClose={() => setModal("closed")} />
      </Dialog>
    </>
  );
}

function MintTokenForm({ onCancel, onSuccess }: { onCancel: () => void; onSuccess: () => void }) {
  const [credential, setCredential] = useState(credentialOptions[0].id);
  const [ttl, setTtl] = useState("1h");
  const [remaining, setRemaining] = useState("");
  const [refillAmount, setRefillAmount] = useState("");
  const [refillInterval, setRefillInterval] = useState("");
  const [metadata, setMetadata] = useState("{ }");

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

        {/* Metadata */}
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="metadata" className="text-xs">Metadata</Label>
          <Textarea id="metadata" value={metadata} onChange={(e) => setMetadata(e.target.value)} className="font-mono text-xs" placeholder="{ }" />
          <span className="text-[11px] text-dim">Optional JSON object.</span>
        </div>
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onCancel}>Cancel</Button>
        <Button onClick={onSuccess}>Mint Token</Button>
      </DialogFooter>
    </DialogContent>
  );
}

function MintSuccessContent({ onClose }: { onClose: () => void }) {
  const [copied, setCopied] = useState<string | null>(null);

  const tokenValue = "ptok_eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJvcmdfaWQiOiI5ZjJhLi4uYjRjMSIsImlhdCI6MTcxMDk1NzIwMH0.aI5ZjJh…";
  const baseUrl = "https://api.llmvault.dev/v1/proxy";
  const curlCommand = `curl ${baseUrl}/v1/chat/completions \\\n    -H "Authorization: Bearer ptok_eyJhbG..." \\\n    -H "Content-Type: application/json" \\\n    -d '{"model":"gpt-4o","messages":[...]}'`;

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
              Scoped to prod-openai-main &middot; Expires in 1 hour
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
          <span className="flex-1 break-all font-mono text-xs leading-4 text-foreground">{tokenValue}</span>
          <Button size="sm" onClick={() => handleCopy(tokenValue, "token")} className="shrink-0 gap-1.5">
            {copied === "token" ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            {copied === "token" ? "Copied" : "Copy"}
          </Button>
        </div>
      </div>

      {/* Quick Start */}
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
    -H "Authorization: Bearer ptok_eyJhbG..." \\
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

      <DialogFooter className="justify-end rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button onClick={onClose}>Done</Button>
      </DialogFooter>
    </DialogContent>
  );
}
