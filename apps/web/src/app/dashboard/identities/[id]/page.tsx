"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { StatusBadge } from "@/components/status-badge";
import { ProviderBadge } from "@/components/provider-badge";
import { RemainingBar, RemainingBarCompact } from "@/components/remaining-bar";

type LinkedCredential = {
  name: string;
  id: string;
  provider: string;
  status: "Active" | "Expiring" | "Revoked";
  remaining: { current: string; max: string; percent: number } | null;
  created: string;
};

const identity = {
  externalId: "customer_42",
  credentials: 3,
  created: "Mar 1, 2026 at 09:14 UTC",
  id: "a1b2c3d4-5678-4321-abcd-e41b12345678",
  rateLimits: [
    { type: "requests", value: 100, window: "60s", description: "100 requests per 60 second window" },
    { type: "tokens", value: 50000, displayValue: "50,000", window: "60s", description: "50,000 tokens per 60 second window" },
  ],
  linkedCredentials: [
    { name: "prod-openai-main", id: "9f2a…b4c1", provider: "openai", status: "Active" as const, remaining: { current: "4,500", max: "10,000", percent: 45 }, created: "Feb 12, 2026" },
    { name: "staging-anthropic", id: "4b1c…e7a2", provider: "anthropic", status: "Active" as const, remaining: null, created: "Feb 20, 2026" },
    { name: "prod-gemini-flash", id: "d83f…1a9e", provider: "google", status: "Active" as const, remaining: { current: "4,500", max: "5,000", percent: 90 }, created: "Mar 2, 2026" },
  ] as LinkedCredential[],
  metadata: { plan: "pro", region: "us", company: "Acme Corp", billing_email: "billing@acme.co" },
};

function ConfigRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border py-3 last:border-b-0 last:pb-0">
      <span className="text-[13px] text-dim">{label}</span>
      {children}
    </div>
  );
}

const credentialColumns: DataTableColumn<LinkedCredential>[] = [
  {
    id: "label",
    header: "Label",
    width: "22%",
    cell: (row) => (
      <div className="flex flex-col">
        <span className="text-[13px] font-medium leading-4 text-foreground">{row.name}</span>
        <span className="font-mono text-[11px] leading-3.5 text-dim">{row.id}</span>
      </div>
    ),
  },
  {
    id: "provider",
    header: "Provider",
    width: "12%",
    cell: (row) => <ProviderBadge provider={row.provider} />,
  },
  {
    id: "status",
    header: "Status",
    width: "10%",
    cell: (row) => <StatusBadge status={row.status} />,
  },
  {
    id: "remaining",
    header: "Remaining",
    width: "40%",
    cell: (row) =>
      row.remaining ? (
        <RemainingBar {...row.remaining} />
      ) : (
        <span className="text-xs text-dim">Unlimited</span>
      ),
  },
  {
    id: "created",
    header: "Created",
    width: "16%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => row.created,
  },
];

function CredentialMobileCard({ cred }: { cred: LinkedCredential }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="text-[13px] font-medium leading-4 text-foreground">{cred.name}</span>
          <span className="font-mono text-[11px] leading-3.5 text-dim">{cred.id}</span>
        </div>
        <StatusBadge status={cred.status} />
      </div>
      <div className="flex items-center gap-3">
        <ProviderBadge provider={cred.provider} />
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>{cred.created}</span>
        {cred.remaining ? (
          <RemainingBarCompact {...cred.remaining} />
        ) : (
          <span className="text-dim">Unlimited</span>
        )}
      </div>
    </div>
  );
}

export default function IdentityDetailPage() {
  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <div className="flex items-center gap-1.5">
          <Link href="/dashboard/identities" className="text-[13px] text-dim hover:text-foreground">
            Identities
          </Link>
          <span className="text-[13px] text-dim">/</span>
          <span className="text-[13px] text-muted-foreground">{identity.externalId}</span>
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="font-mono text-lg font-semibold tracking-tight text-foreground sm:text-[22px]">
            {identity.externalId}
          </h1>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="lg">Edit</Button>
            <Button variant="destructive" size="lg">Delete</Button>
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
            <div className="flex flex-col">
              <ConfigRow label="External ID">
                <span className="font-mono text-[13px] text-foreground">{identity.externalId}</span>
              </ConfigRow>
              <ConfigRow label="Credentials">
                <span className="font-mono text-[13px] text-foreground">{identity.credentials}</span>
              </ConfigRow>
              <ConfigRow label="Created">
                <span className="font-mono text-[13px] text-muted-foreground">{identity.created}</span>
              </ConfigRow>
              <ConfigRow label="ID">
                <span className="font-mono text-[13px] text-muted-foreground">{identity.id}</span>
              </ConfigRow>
            </div>
          </div>

          {/* Rate Limits Card */}
          <div className="flex w-full flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:w-85 lg:shrink-0">
            <span className="text-[13px] font-semibold uppercase tracking-wider text-dim">Rate Limits</span>
            <div className="flex flex-col gap-0">
              {identity.rateLimits.map((rl, i) => (
                <div key={rl.type} className={`flex flex-col gap-1.5 py-3 ${i < identity.rateLimits.length - 1 ? "border-b border-border" : ""}`}>
                  <div className="flex items-center justify-between">
                    <span className="text-[13px] font-medium text-foreground">{rl.type}</span>
                    <span className="font-mono text-[13px] text-chart-2">
                      {rl.displayValue ?? rl.value} / {rl.window}
                    </span>
                  </div>
                  <span className="text-[11px] text-dim">{rl.description}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Linked Credentials */}
        <div className="flex flex-col">
          <div className="flex items-center justify-between pb-4">
            <span className="text-sm font-medium text-foreground">Linked Credentials</span>
            <Link href="/dashboard/credentials" className="text-[13px] text-chart-2">View all credentials</Link>
          </div>
          <DataTable
            columns={credentialColumns}
            data={identity.linkedCredentials}
            keyExtractor={(row) => row.id}
            minWidth={700}
            mobileCard={(row) => <CredentialMobileCard cred={row} />}
          />
        </div>

        {/* Metadata */}
        <div className="flex flex-col">
          <div className="pb-4">
            <span className="text-sm font-medium text-foreground">Metadata</span>
          </div>
          <div className="border border-border bg-code p-4">
            <pre className="font-mono text-xs leading-5 text-muted-foreground">
              {JSON.stringify(identity.metadata, null, 2)}
            </pre>
          </div>
        </div>
      </div>
    </>
  );
}
