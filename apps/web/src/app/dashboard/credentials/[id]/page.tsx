"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge } from "@/components/status-badge";
import { ProviderBadge } from "@/components/provider-badge";

type Token = {
  jti: string;
  remaining: string;
  remainingPercent: number;
  expires: string;
  created: string;
};

const credential = {
  name: "prod-openai-main",
  id: "cred_9f2a…b4c1",
  provider: "openai",
  status: "Active" as const,
  baseUrl: "https://api.openai.com/v1",
  authScheme: "Bearer",
  identity: "user_83291",
  created: "2024-01-15 09:32:00 UTC",
  usage: {
    current: 7218,
    max: 10000,
    percent: 72,
    refillAmount: "10,000",
    refillInterval: "24h",
    totalRequests: "24,891",
  },
  tokens: [
    { jti: "tok_a1b2c3d4e5f6…", remaining: "482 / 1,000", remainingPercent: 48, expires: "2024-03-15 14:00 UTC", created: "2024-03-14 14:00 UTC" },
    { jti: "tok_g7h8i9j0k1l2…", remaining: "1,000 / 1,000", remainingPercent: 100, expires: "2024-03-16 09:00 UTC", created: "2024-03-15 09:00 UTC" },
    { jti: "tok_m3n4o5p6q7r8…", remaining: "23 / 500", remainingPercent: 5, expires: "2024-03-15 10:30 UTC", created: "2024-03-14 10:30 UTC" },
  ] as Token[],
  metadata: { team: "ml-platform", environment: "production", cost_center: "CC-4821", owner: "jane.doe@acme.corp" },
};

const tokenColumns: DataTableColumn<Token>[] = [
  {
    id: "jti",
    header: "JTI",
    width: "35%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => row.jti,
  },
  {
    id: "remaining",
    header: "Remaining",
    width: "22%",
    cell: (row) => (
      <div className="flex items-center gap-1.5 pr-4">
        <div className="h-1 flex-1 bg-secondary">
          <div className="h-full bg-primary" style={{ width: `${row.remainingPercent}%` }} />
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">{row.remaining}</span>
      </div>
    ),
  },
  {
    id: "expires",
    header: "Expires",
    width: "22%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.expires,
  },
  {
    id: "created",
    header: "Created",
    width: "21%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.created,
  },
];

function TokenMobileCard({ token }: { token: Token }) {
  return (
    <div className="flex flex-col gap-2 border border-border bg-card p-4">
      <span className="font-mono text-[13px] text-foreground">{token.jti}</span>
      <div className="flex items-center gap-2">
        <div className="h-1 w-16 bg-secondary">
          <div className="h-full bg-primary" style={{ width: `${token.remainingPercent}%` }} />
        </div>
        <span className="font-mono text-[11px] text-muted-foreground">{token.remaining}</span>
      </div>
      <span className="text-xs text-dim">Expires {token.expires}</span>
    </div>
  );
}

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <span className="text-[13px] text-dim">{label}</span>
      {children}
    </div>
  );
}

export default function CredentialDetailPage() {
  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link href="/dashboard/credentials" className="text-[13px] text-dim hover:text-foreground">
            Credentials
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">{credential.name}</span>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
              {credential.name}
            </h1>
            <StatusBadge status={credential.status} />
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="lg">Mint Token</Button>
            <Button variant="destructive" size="lg">Revoke</Button>
          </div>
        </div>
      </header>

      {/* Content */}
      <div className="flex flex-col gap-6 px-4 py-4 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
        {/* Info Cards */}
        <div className="flex flex-col gap-5 lg:flex-row">
          {/* Configuration Card */}
          <div className="flex flex-1 flex-col gap-4 border border-border bg-card p-4 sm:p-5">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">Configuration</span>
            <div className="flex flex-col gap-3.5">
              <ConfigRow label="Base URL">
                <span className="font-mono text-[13px] text-foreground">{credential.baseUrl}</span>
              </ConfigRow>
              <ConfigRow label="Auth Scheme">
                <span className="font-mono text-[13px] text-foreground">{credential.authScheme}</span>
              </ConfigRow>
              <ConfigRow label="Provider">
                <ProviderBadge provider={credential.provider} />
              </ConfigRow>
              <ConfigRow label="Identity">
                <span className="font-mono text-[13px] text-muted-foreground">{credential.identity}</span>
              </ConfigRow>
              <ConfigRow label="Created">
                <span className="font-mono text-[13px] text-muted-foreground">{credential.created}</span>
              </ConfigRow>
              <ConfigRow label="ID">
                <span className="font-mono text-[13px] text-muted-foreground">{credential.id}</span>
              </ConfigRow>
            </div>
          </div>

          {/* Usage Card */}
          <div className="flex w-full flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:w-85 lg:shrink-0">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">Usage</span>
            <div className="flex flex-col gap-2">
              <span className="font-mono text-[28px] font-medium leading-8.5 tracking-tight text-foreground">
                {credential.usage.current.toLocaleString()}
              </span>
              <span className="text-xs text-dim">of {credential.usage.max.toLocaleString()}</span>
              <div className="h-1.5 w-full bg-secondary">
                <div className="h-full bg-primary" style={{ width: `${credential.usage.percent}%` }} />
              </div>
              <span className="text-[11px] text-dim">remaining this period</span>
            </div>
            <div className="flex flex-col gap-3 border-t border-border pt-4">
              <ConfigRow label="Refill Amount">
                <span className="font-mono text-[13px] text-foreground">{credential.usage.refillAmount}</span>
              </ConfigRow>
              <ConfigRow label="Refill Interval">
                <span className="font-mono text-[13px] text-foreground">{credential.usage.refillInterval}</span>
              </ConfigRow>
              <ConfigRow label="Total Requests">
                <span className="font-mono text-[13px] text-foreground">{credential.usage.totalRequests}</span>
              </ConfigRow>
            </div>
          </div>
        </div>

        {/* Active Tokens */}
        <div className="flex flex-col">
          <div className="flex items-center justify-between pb-4">
            <span className="text-sm font-medium text-foreground">Active Tokens</span>
            <Link href="/dashboard/tokens" className="text-[13px] text-chart-2">View all tokens</Link>
          </div>
          <DataTable
            columns={tokenColumns}
            data={credential.tokens}
            keyExtractor={(row) => row.jti}
            minWidth={700}
            mobileCard={(row) => <TokenMobileCard token={row} />}
          />
        </div>

        {/* Metadata */}
        <div className="flex flex-col">
          <div className="pb-4">
            <span className="text-sm font-medium text-foreground">Metadata</span>
          </div>
          <div className="border border-border bg-code p-4">
            <pre className="font-mono text-xs leading-5 text-muted-foreground">
              {JSON.stringify(credential.metadata, null, 2)}
            </pre>
          </div>
        </div>
      </div>
    </>
  );
}
