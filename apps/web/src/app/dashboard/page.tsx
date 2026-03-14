"use client";

import { KeyRound, Coins, Users, BarChart3, TrendingUp, TrendingDown, Minus } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { $api } from "@/api/client";

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function StatCard({
  label,
  value,
  subtitle,
  icon: Icon,
  change,
}: {
  label: string;
  value: string;
  subtitle?: string;
  icon: typeof KeyRound;
  change?: { value: string; positive: boolean | null };
}) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4 sm:p-5">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wider text-dim">{label}</span>
        <Icon className="size-4 text-dim" />
      </div>
      <span className="font-mono text-2xl font-medium leading-8.5 tracking-tight text-foreground sm:text-[28px]">
        {value}
      </span>
      {subtitle && <span className="text-xs text-dim">{subtitle}</span>}
      {change && (
        <div className="flex items-center gap-1">
          {change.positive === true && <TrendingUp className="size-3 text-success-foreground" />}
          {change.positive === false && <TrendingDown className="size-3 text-destructive" />}
          {change.positive === null && <Minus className="size-3 text-dim" />}
          <span className={`text-xs ${change.positive === true ? "text-success-foreground" : change.positive === false ? "text-destructive" : "text-dim"}`}>
            {change.value}
          </span>
          <span className="text-xs text-dim">vs yesterday</span>
        </div>
      )}
    </div>
  );
}

function TopCredentialRow({ label, provider, count, rank }: { label: string; provider: string; count: number; rank: number }) {
  return (
    <div className="flex items-center gap-4 border-b border-border px-4 py-3 last:border-b-0">
      <span className="w-5 shrink-0 font-mono text-xs text-dim">{rank}</span>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[13px] font-medium text-foreground">{label}</span>
        <span className="text-[11px] text-dim">{provider}</span>
      </div>
      <span className="shrink-0 font-mono text-[13px] text-foreground">{formatNumber(count)}</span>
    </div>
  );
}

function DailyChart({ data }: { data: { date: string; count: number }[] }) {
  if (data.length === 0) {
    return <span className="py-8 text-center text-sm text-dim">No request data yet</span>;
  }

  const max = Math.max(...data.map((d) => d.count), 1);

  return (
    <div className="flex items-end gap-[3px]" style={{ height: 120 }}>
      {data.map((d) => {
        const height = Math.max((d.count / max) * 100, 2);
        return (
          <div
            key={d.date}
            className="flex-1 bg-primary/60 transition-all hover:bg-primary"
            style={{ height: `${height}%` }}
            title={`${d.date}: ${d.count.toLocaleString()} requests`}
          />
        );
      })}
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Dashboard</h1>
      </header>
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:grid-cols-2 sm:gap-4 sm:px-6 sm:pt-6 lg:px-8 xl:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex h-[130px] animate-pulse flex-col gap-3 border border-border bg-card p-4 sm:p-5">
            <div className="h-3 w-24 bg-secondary" />
            <div className="h-8 w-16 bg-secondary" />
            <div className="h-3 w-20 bg-secondary" />
          </div>
        ))}
      </section>
    </>
  );
}

export default function DashboardPage() {
  const { data: usage, isLoading } = $api.useQuery("get", "/v1/usage");

  if (isLoading || !usage) return <LoadingSkeleton />;

  const creds = usage.credentials;
  const tokens = usage.tokens;
  const identities = usage.identities;
  const requests = usage.requests;

  // Compute change vs yesterday
  const todayCount = requests?.today ?? 0;
  const yesterdayCount = requests?.yesterday ?? 0;
  let changeValue: string;
  let changePositive: boolean | null;
  if (yesterdayCount === 0 && todayCount === 0) {
    changeValue = "0%";
    changePositive = null;
  } else if (yesterdayCount === 0) {
    changeValue = "+100%";
    changePositive = true;
  } else {
    const pct = Math.round(((todayCount - yesterdayCount) / yesterdayCount) * 100);
    changeValue = `${pct >= 0 ? "+" : ""}${pct}%`;
    changePositive = pct > 0 ? true : pct < 0 ? false : null;
  }

  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Dashboard</h1>
        <Link href="/dashboard/credentials">
          <Button size="lg">New Credential</Button>
        </Link>
      </header>

      {/* Stats */}
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:grid-cols-2 sm:gap-4 sm:px-6 sm:pt-6 lg:px-8 xl:grid-cols-4">
        <StatCard
          label="Active Credentials"
          value={String(creds?.active ?? 0)}
          subtitle={creds?.revoked ? `${creds.revoked} revoked` : undefined}
          icon={KeyRound}
        />
        <StatCard
          label="Active Tokens"
          value={String(tokens?.active ?? 0)}
          subtitle={tokens?.expired ? `${tokens.expired} expired` : undefined}
          icon={Coins}
        />
        <StatCard
          label="Identities"
          value={String(identities?.total ?? 0)}
          icon={Users}
        />
        <StatCard
          label="Requests Today"
          value={formatNumber(todayCount)}
          icon={BarChart3}
          change={{ value: changeValue, positive: changePositive }}
        />
      </section>

      {/* Request Chart + Top Credentials */}
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:gap-4 sm:px-6 sm:pt-6 lg:grid-cols-5 lg:px-8">
        {/* Daily chart */}
        <div className="flex flex-col gap-4 border border-border bg-card p-4 sm:p-5 lg:col-span-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">Requests — Last 30 Days</span>
            <span className="font-mono text-xs text-dim">{formatNumber(requests?.last_30d ?? 0)} total</span>
          </div>
          <DailyChart data={(usage.daily_requests ?? []).map(d => ({ date: d.date ?? '', count: d.count ?? 0 }))} />
        </div>

        {/* Top credentials */}
        <div className="flex flex-col border border-border bg-card lg:col-span-2">
          <div className="flex items-center justify-between px-4 py-3">
            <span className="text-sm font-medium text-foreground">Top Credentials</span>
            <Link href="/dashboard/credentials" className="text-[13px] text-chart-2">View all</Link>
          </div>
          {(usage.top_credentials ?? []).length === 0 ? (
            <span className="px-4 py-8 text-center text-sm text-dim">No request data yet</span>
          ) : (
            (usage.top_credentials ?? []).map((cred, i) => (
              <TopCredentialRow
                key={cred.id}
                label={cred.label ?? ""}
                provider={cred.provider_id ?? ""}
                count={cred.request_count ?? 0}
                rank={i + 1}
              />
            ))
          )}
        </div>
      </section>

      {/* Summary row */}
      <section className="grid shrink-0 grid-cols-2 gap-3 px-4 pt-4 pb-6 sm:grid-cols-4 sm:gap-4 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        <div className="flex flex-col gap-1 border border-border bg-card p-4">
          <span className="text-xs text-dim">Total Credentials</span>
          <span className="font-mono text-lg font-medium text-foreground">{creds?.total ?? 0}</span>
        </div>
        <div className="flex flex-col gap-1 border border-border bg-card p-4">
          <span className="text-xs text-dim">Total Tokens</span>
          <span className="font-mono text-lg font-medium text-foreground">{tokens?.total ?? 0}</span>
        </div>
        <div className="flex flex-col gap-1 border border-border bg-card p-4">
          <span className="text-xs text-dim">Requests (7d)</span>
          <span className="font-mono text-lg font-medium text-foreground">{formatNumber(requests?.last_7d ?? 0)}</span>
        </div>
        <div className="flex flex-col gap-1 border border-border bg-card p-4">
          <span className="text-xs text-dim">Requests (All Time)</span>
          <span className="font-mono text-lg font-medium text-foreground">{formatNumber(requests?.total ?? 0)}</span>
        </div>
      </section>
    </>
  );
}
