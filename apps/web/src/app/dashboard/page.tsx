"use client";

import {
  KeyRound,
  Coins,
  Users,
  BarChart3,
  TrendingUp,
  TrendingDown,
  Minus,
  Unplug,
  Sparkles,
  Cable,
  DollarSign,
  Activity,
} from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { $api } from "@/api/client";
import { useDashboardMode } from "@/hooks/use-dashboard-mode";

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function formatCost(n: number): string {
  if (n >= 1_000) return `$${(n / 1_000).toFixed(1)}k`;
  if (n >= 1) return `$${n.toFixed(2)}`;
  if (n > 0) return `$${n.toFixed(4)}`;
  return "$0.00";
}

function computeChange(today: number, yesterday: number): { value: string; positive: boolean | null } {
  if (yesterday === 0 && today === 0) return { value: "0%", positive: null };
  if (yesterday === 0) return { value: "+100%", positive: true };
  const pct = Math.round(((today - yesterday) / yesterday) * 100);
  return {
    value: `${pct >= 0 ? "+" : ""}${pct}%`,
    positive: pct > 0 ? true : pct < 0 ? false : null,
  };
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

function SpendChart({ data }: { data: { date: string; cost: number }[] }) {
  if (data.length === 0) {
    return <span className="py-8 text-center text-sm text-dim">No spend data yet</span>;
  }

  const max = Math.max(...data.map((d) => d.cost), 0.01);

  return (
    <div className="flex items-end gap-[3px]" style={{ height: 120 }}>
      {data.map((d) => {
        const height = Math.max((d.cost / max) * 100, 2);
        return (
          <div
            key={d.date}
            className="flex-1 bg-chart-5/60 transition-all hover:bg-chart-5"
            style={{ height: `${height}%` }}
            title={`${d.date}: ${formatCost(d.cost)}`}
          />
        );
      })}
    </div>
  );
}

function RankedRow({
  rank,
  label,
  sublabel,
  value,
}: {
  rank: number;
  label: string;
  sublabel?: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-4 border-b border-border px-4 py-3 last:border-b-0">
      <span className="w-5 shrink-0 font-mono text-xs text-dim">{rank}</span>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[13px] font-medium text-foreground">{label}</span>
        {sublabel && <span className="text-[11px] text-dim">{sublabel}</span>}
      </div>
      <span className="shrink-0 font-mono text-[13px] text-foreground">{value}</span>
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Home</h1>
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

/* ── App Mode (static) ── */

const staticRecentActivity = [
  { time: "2m ago", source: "gpt-4o", detail: "1,247 tokens", cost: "$0.04", icon: Sparkles },
  { time: "5m ago", source: "GitHub", detail: "create_issue", cost: "200", icon: Unplug },
  { time: "8m ago", source: "claude-4", detail: "3,891 tokens", cost: "$0.12", icon: Sparkles },
  { time: "12m ago", source: "Slack", detail: "send_message", cost: "200", icon: Unplug },
];

function AppHomeContent() {
  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Home</h1>
        <Link href="/dashboard/connections">
          <Button size="lg">New Connection</Button>
        </Link>
      </header>

      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:grid-cols-3 sm:gap-4 sm:px-6 sm:pt-6 lg:px-8">
        <StatCard label="Connections" value="3" icon={Unplug} />
        <StatCard label="LLM Keys" value="2" icon={KeyRound} />
        <StatCard
          label="Requests Today"
          value="1,247"
          icon={BarChart3}
          change={{ value: "+18%", positive: true }}
        />
      </section>

      <section className="px-4 pt-4 pb-6 sm:px-6 sm:pt-6 sm:pb-8 lg:px-8">
        <div className="flex flex-col border border-border bg-card">
          <div className="px-4 py-3">
            <span className="text-sm font-medium text-foreground">Recent Activity</span>
          </div>
          {staticRecentActivity.map((item, i) => (
            <div key={i} className="flex items-center gap-4 border-t border-border px-4 py-3">
              <span className="w-16 shrink-0 text-xs text-dim">{item.time}</span>
              <item.icon className="size-4 shrink-0 text-dim" />
              <span className="flex-1 text-[13px] font-medium text-foreground">{item.source}</span>
              <span className="font-mono text-[13px] text-muted-foreground">{item.detail}</span>
              <span className="w-14 shrink-0 text-right font-mono text-[13px] text-foreground">{item.cost}</span>
            </div>
          ))}
        </div>
      </section>
    </>
  );
}

/* ── Platform Mode (dynamic) ── */

function PlatformHomeContent() {
  const { data: usage, isLoading } = $api.useQuery("get", "/v1/usage");

  if (isLoading || !usage) return <LoadingSkeleton />;

  const identities = usage.identities;
  const requests = usage.requests;
  const todayCount = requests?.today ?? 0;
  const yesterdayCount = requests?.yesterday ?? 0;
  const change = computeChange(todayCount, yesterdayCount);

  const totalSpend = (usage.spend_over_time ?? []).reduce((sum, d) => sum + (d.total_cost ?? 0), 0);

  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">Home</h1>
      </header>

      {/* Top stats row */}
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:grid-cols-2 sm:gap-4 sm:px-6 sm:pt-6 lg:px-8 xl:grid-cols-4">
        <StatCard
          label="Apps"
          value="—"
          subtitle="Not tracked yet"
          icon={Cable}
        />
        <StatCard
          label="Connections"
          value="—"
          subtitle="Not tracked yet"
          icon={Unplug}
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
          change={change}
        />
      </section>

      {/* Charts: Requests + Spend */}
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 sm:gap-4 sm:px-6 sm:pt-6 lg:grid-cols-2 lg:px-8">
        <div className="flex flex-col gap-4 border border-border bg-card p-4 sm:p-5">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">Requests — Last 30 Days</span>
            <span className="font-mono text-xs text-dim">{formatNumber(requests?.last_30d ?? 0)} total</span>
          </div>
          <DailyChart data={(usage.daily_requests ?? []).map(d => ({ date: d.date ?? "", count: d.count ?? 0 }))} />
        </div>

        <div className="flex flex-col gap-4 border border-border bg-card p-4 sm:p-5">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-foreground">Spend — Last 30 Days</span>
            <span className="font-mono text-xs text-dim">{formatCost(totalSpend)} total</span>
          </div>
          <SpendChart data={(usage.spend_over_time ?? []).map(d => ({ date: d.date ?? "", cost: d.total_cost ?? 0 }))} />
        </div>
      </section>

      {/* Tables: Top Models + Top Customers + Latency/Errors */}
      <section className="grid shrink-0 grid-cols-1 gap-3 px-4 pt-4 pb-6 sm:gap-4 sm:px-6 sm:pt-6 sm:pb-8 lg:grid-cols-3 lg:px-8">
        {/* Top Models */}
        <div className="flex flex-col border border-border bg-card">
          <div className="px-4 py-3">
            <span className="text-sm font-medium text-foreground">Top Models</span>
          </div>
          {(usage.top_models ?? []).length === 0 ? (
            <span className="px-4 py-8 text-center text-sm text-dim">No data yet</span>
          ) : (
            (usage.top_models ?? []).slice(0, 5).map((m, i) => (
              <RankedRow
                key={m.model}
                rank={i + 1}
                label={m.model ?? ""}
                sublabel={m.provider_id ?? ""}
                value={formatNumber(m.request_count ?? 0)}
              />
            ))
          )}
        </div>

        {/* Top Customers */}
        <div className="flex flex-col border border-border bg-card">
          <div className="px-4 py-3">
            <span className="text-sm font-medium text-foreground">Top Customers</span>
          </div>
          {(usage.top_users ?? []).length === 0 ? (
            <span className="px-4 py-8 text-center text-sm text-dim">No data yet</span>
          ) : (
            (usage.top_users ?? []).slice(0, 5).map((u, i) => (
              <RankedRow
                key={u.user_id}
                rank={i + 1}
                label={u.user_id ?? "—"}
                value={formatCost(u.total_cost ?? 0)}
              />
            ))
          )}
        </div>

        {/* Summary stats */}
        <div className="flex flex-col gap-3 border border-border bg-card p-4 sm:p-5">
          <span className="text-sm font-medium text-foreground">Summary</span>
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">Requests (7d)</span>
              <span className="font-mono text-[13px] font-medium text-foreground">{formatNumber(requests?.last_7d ?? 0)}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">Requests (All Time)</span>
              <span className="font-mono text-[13px] font-medium text-foreground">{formatNumber(requests?.total ?? 0)}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">Active Credentials</span>
              <span className="font-mono text-[13px] font-medium text-foreground">{usage.credentials?.active ?? 0}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">Active Tokens</span>
              <span className="font-mono text-[13px] font-medium text-foreground">{usage.tokens?.active ?? 0}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">API Keys</span>
              <span className="font-mono text-[13px] font-medium text-foreground">{usage.api_keys?.active ?? 0}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-xs text-dim">Error Rate (30d)</span>
              <span className="font-mono text-[13px] font-medium text-foreground">
                {(() => {
                  const rates = usage.error_rates ?? [];
                  const totalReqs = rates.reduce((s, r) => s + (r.total ?? 0), 0);
                  const totalErrs = rates.reduce((s, r) => s + (r.error_count ?? 0), 0);
                  if (totalReqs === 0) return "0%";
                  return `${((totalErrs / totalReqs) * 100).toFixed(1)}%`;
                })()}
              </span>
            </div>
          </div>
        </div>
      </section>
    </>
  );
}

export default function DashboardPage() {
  const { mode } = useDashboardMode();
  return mode === "app" ? <AppHomeContent /> : <PlatformHomeContent />;
}
