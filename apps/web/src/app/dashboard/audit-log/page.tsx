"use client";

import { useState } from "react";
import { Search, ChevronDown, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DataTable, type DataTableColumn } from "@/components/data-table";

type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

type AuditEvent = {
  id: string;
  timestamp: string;
  credential: string | null;
  method: HttpMethod;
  path: string;
  status: number;
  latency: string;
  ipAddress: string;
  category: "proxy" | "management";
};

const events: AuditEvent[] = [
  { id: "1", timestamp: "Mar 9, 14:23:41", credential: "prod-openai-main", method: "POST", path: "/v1/chat/completions", status: 200, latency: "234ms", ipAddress: "192.168.1.42", category: "proxy" },
  { id: "2", timestamp: "Mar 9, 14:22:18", credential: null, method: "POST", path: "/v1/credentials", status: 201, latency: "45ms", ipAddress: "10.0.3.15", category: "management" },
  { id: "3", timestamp: "Mar 9, 14:21:05", credential: "staging-anthropic", method: "POST", path: "/v1/chat/completions", status: 429, latency: "12ms", ipAddress: "203.0.113.7", category: "proxy" },
  { id: "4", timestamp: "Mar 9, 14:19:33", credential: "prod-openai-main", method: "GET", path: "/v1/models", status: 200, latency: "45ms", ipAddress: "192.168.1.42", category: "proxy" },
  { id: "5", timestamp: "Mar 9, 14:17:52", credential: "prod-gemini-flash", method: "POST", path: "/v1/chat/completions", status: 500, latency: "1,203ms", ipAddress: "172.16.0.89", category: "proxy" },
  { id: "6", timestamp: "Mar 9, 14:15:09", credential: null, method: "DELETE", path: "/v1/tokens/ptok_3b7a…c812", status: 200, latency: "23ms", ipAddress: "10.0.3.15", category: "management" },
  { id: "7", timestamp: "Mar 9, 14:12:44", credential: "cohere-embed-v3", method: "POST", path: "/v1/embeddings", status: 200, latency: "89ms", ipAddress: "10.0.3.15", category: "proxy" },
  { id: "8", timestamp: "Mar 9, 14:10:21", credential: null, method: "GET", path: "/v1/identities", status: 200, latency: "31ms", ipAddress: "10.0.3.15", category: "management" },
];

type EventFilter = "All" | "Proxy" | "Management";
const filterCounts: Record<EventFilter, number> = { All: 2847, Proxy: 2691, Management: 156 };

const methodConfig: Record<HttpMethod, { bg: string; text: string }> = {
  GET: { bg: "#3B82F614", text: "#3B82F6" },
  POST: { bg: "#22C55E14", text: "#22C55E" },
  PUT: { bg: "#F59E0B14", text: "#F59E0B" },
  PATCH: { bg: "#F59E0B14", text: "#F59E0B" },
  DELETE: { bg: "#EF444414", text: "#EF4444" },
};

function MethodBadge({ method }: { method: HttpMethod }) {
  const config = methodConfig[method];
  return (
    <span
      className="inline-block rounded-lg px-2.5 py-0.75 text-[11px] font-semibold"
      style={{ backgroundColor: config.bg, color: config.text }}
    >
      {method}
    </span>
  );
}

function statusColor(status: number): string {
  if (status >= 200 && status < 300) return "#22C55E";
  if (status >= 400 && status < 500) return "#F59E0B";
  return "#EF4444";
}

const columns: DataTableColumn<AuditEvent>[] = [
  {
    id: "timestamp",
    header: "Timestamp",
    width: "14%",
    cellClassName: "text-[13px] text-foreground",
    cell: (row) => row.timestamp,
  },
  {
    id: "credential",
    header: "Credential",
    width: "14%",
    cellClassName: "font-mono text-xs text-muted-foreground",
    cell: (row) => row.credential ?? "—",
  },
  {
    id: "method",
    header: "Method",
    width: "8%",
    cell: (row) => <MethodBadge method={row.method} />,
  },
  {
    id: "path",
    header: "Path",
    width: "26%",
    cellClassName: "font-mono text-[13px] text-foreground",
    cell: (row) => row.path,
  },
  {
    id: "status",
    header: "Status",
    width: "8%",
    cell: (row) => (
      <span className="font-mono text-[13px]" style={{ color: statusColor(row.status) }}>
        {row.status}
      </span>
    ),
  },
  {
    id: "latency",
    header: "Latency",
    width: "10%",
    cellClassName: "font-mono text-[13px] text-muted-foreground",
    cell: (row) => row.latency,
  },
  {
    id: "ipAddress",
    header: "IP Address",
    width: "20%",
    cellClassName: "font-mono text-[13px] text-dim",
    cell: (row) => row.ipAddress,
  },
];

function MobileCard({ event }: { event: AuditEvent }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-1">
          <span className="text-[13px] text-foreground">{event.timestamp}</span>
          <span className="font-mono text-[11px] text-muted-foreground">
            {event.credential ?? "—"}
          </span>
        </div>
        <MethodBadge method={event.method} />
      </div>
      <span className="font-mono text-[13px] text-foreground">{event.path}</span>
      <div className="flex items-center justify-between text-xs">
        <span className="font-mono" style={{ color: statusColor(event.status) }}>
          {event.status}
        </span>
        <span className="font-mono text-muted-foreground">{event.latency}</span>
        <span className="font-mono text-dim">{event.ipAddress}</span>
      </div>
    </div>
  );
}

export default function AuditLogPage() {
  const [filter, setFilter] = useState<EventFilter>("All");
  const [search, setSearch] = useState("");

  const filtered = events.filter((e) => {
    if (filter === "Proxy" && e.category !== "proxy") return false;
    if (filter === "Management" && e.category !== "management") return false;
    if (
      search &&
      !e.path.toLowerCase().includes(search.toLowerCase()) &&
      !e.credential?.toLowerCase().includes(search.toLowerCase()) &&
      !e.ipAddress.includes(search)
    )
      return false;
    return true;
  });

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-5 sm:px-6 lg:px-8">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Audit Log
        </h1>
        <div className="flex items-center gap-2">
          <div className="relative hidden sm:block">
            <Search className="absolute left-3.5 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
            <Input
              placeholder="Search events..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-[145px] pl-9 font-mono text-[13px]"
            />
          </div>
          <Button size="lg">
            <Download className="size-3.5" />
            Export
          </Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="px-4 pt-4 sm:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-dim" />
          <Input
            placeholder="Search events..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9 font-mono text-[13px]"
          />
        </div>
      </div>

      {/* Filters */}
      <section className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-4 sm:px-6 lg:px-8">
        {(["All", "Proxy", "Management"] as EventFilter[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setFilter(tab)}
            className={`px-3 py-1.5 text-xs font-medium transition-colors ${
              filter === tab
                ? "bg-primary/8 text-chart-2"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab} ({filterCounts[tab].toLocaleString()})
          </button>
        ))}
        <div className="mx-1 h-4 w-px shrink-0 bg-border" />
        <button className="flex items-center gap-1.5 border border-border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground">
          Last 7 days
          <ChevronDown className="size-2.5" />
        </button>
      </section>

      {/* Table */}
      <section className="flex shrink-0 flex-col px-4 pb-6 sm:px-6 sm:pb-8 lg:px-8">
        <DataTable
          columns={columns}
          data={filtered}
          keyExtractor={(row) => row.id}
          mobileCard={(row) => <MobileCard event={row} />}
          minWidth={1048}
        />
      </section>
    </>
  );
}
