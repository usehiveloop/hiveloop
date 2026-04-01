"use client";

import { useState } from "react";
import Link from "next/link";
import { $api } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { Bot, Plus, Search, ChevronLeft, ChevronRight } from "lucide-react";
import { CreateAgentDialog } from "./create-agent-dialog";

const PAGE_SIZE = 20;

type AgentRow = {
  id?: string;
  name?: string;
  description?: string;
  model?: string;
  provider_id?: string;
  identity_id?: string;
  sandbox_type?: string;
  status?: string;
  created_at?: string;
};

function truncateId(id: string) {
  return id.length > 12 ? `${id.slice(0, 6)}...${id.slice(-4)}` : id;
}

function formatRelativeDate(dateStr?: string) {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

const columns: DataTableColumn<AgentRow>[] = [
  {
    id: "agent",
    header: "Agent",
    width: "25%",
    cell: (row) => (
      <div className="flex flex-col">
        <span className="text-[13px] font-medium leading-4 text-foreground">
          {row.name || "Untitled"}
        </span>
        <span className="font-mono text-[11px] leading-3.5 text-dim">
          {truncateId(row.id ?? "")}
        </span>
      </div>
    ),
  },
  {
    id: "model",
    header: "Model",
    width: "20%",
    cellClassName: "font-mono text-[12px] text-foreground",
    cell: (row) => row.model ?? "",
  },
  {
    id: "provider",
    header: "Provider",
    width: "13%",
    cell: (row) => (
      <Badge
        variant="outline"
        className="h-auto border-border bg-secondary px-2 py-0.5 font-mono text-[11px] font-normal text-muted-foreground"
      >
        {row.provider_id ?? ""}
      </Badge>
    ),
  },
  {
    id: "sandbox",
    header: "Sandbox",
    width: "12%",
    cell: (row) => (
      <Badge
        variant="outline"
        className={`h-auto px-2 py-0.5 text-[11px] font-normal ${
          row.sandbox_type === "dedicated"
            ? "border-blue-500/30 text-blue-400"
            : "border-primary/20 bg-primary/8 text-chart-2"
        }`}
      >
        {row.sandbox_type ?? ""}
      </Badge>
    ),
  },
  {
    id: "status",
    header: "Status",
    width: "10%",
    cell: (row) => (
      <Badge
        variant="outline"
        className={`h-auto px-2 py-0.5 text-[11px] font-normal ${
          row.status === "active"
            ? "border-green-500/30 text-green-500"
            : "border-yellow-500/30 text-yellow-500"
        }`}
      >
        {row.status ?? ""}
      </Badge>
    ),
  },
  {
    id: "created",
    header: "Created",
    width: "20%",
    cellClassName: "text-[13px] text-muted-foreground",
    cell: (row) => formatRelativeDate(row.created_at),
  },
];

function AgentMobileCard({ agent }: { agent: AgentRow }) {
  return (
    <div className="flex flex-col gap-2 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <span className="text-[13px] font-medium text-foreground">{agent.name}</span>
        <Badge
          variant="outline"
          className={`h-auto px-2 py-0.5 text-[11px] ${
            agent.status === "active"
              ? "border-green-500/30 text-green-500"
              : "border-yellow-500/30 text-yellow-500"
          }`}
        >
          {agent.status}
        </Badge>
      </div>
      <div className="flex items-center gap-2">
        <span className="font-mono text-[11px] text-muted-foreground">{agent.model}</span>
        <Badge variant="outline" className="h-auto px-1.5 py-0 text-[10px] text-dim">
          {agent.sandbox_type}
        </Badge>
      </div>
    </div>
  );
}

export default function AgentsPage() {
  const [search, setSearch] = useState("");
  const [modal, setModal] = useState<"closed" | "create">("closed");
  const [cursors, setCursors] = useState<string[]>([]);

  const currentCursor = cursors[cursors.length - 1];

  const { data, isLoading } = $api.useQuery("get", "/v1/agents", {
    params: {
      query: {
        limit: PAGE_SIZE,
        ...(currentCursor ? { cursor: currentCursor } : {}),
      },
    },
  });

  const agents: AgentRow[] = (data?.data ?? []) as AgentRow[];
  const hasMore = (data?.data?.length ?? 0) >= PAGE_SIZE;
  const pageNumber = cursors.length + 1;

  const filtered = search
    ? agents.filter(
        (a) =>
          (a.name ?? "").toLowerCase().includes(search.toLowerCase()) ||
          (a.model ?? "").toLowerCase().includes(search.toLowerCase())
      )
    : agents;

  return (
    <>
      {/* Header */}
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-5 sm:px-6 lg:px-8">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Agents
        </h1>
        <div className="flex items-center gap-3">
          <div className="relative hidden md:block">
            <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search agents..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8 w-56 pl-8 text-[13px]"
            />
          </div>
          <Button size="sm" onClick={() => setModal("create")}>
            <Plus className="mr-1.5 size-3.5" />
            New Agent
          </Button>
        </div>
      </header>

      {/* Mobile search */}
      <div className="border-b border-border px-4 py-3 md:hidden">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search agents..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-8 pl-8 text-[13px]"
          />
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto px-4 py-4 sm:px-6 lg:px-8">
        {isLoading ? (
          <div className="flex items-center justify-center py-20 text-[13px] text-muted-foreground">
            Loading agents...
          </div>
        ) : agents.length === 0 && cursors.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-20">
            <Bot className="size-10 text-muted-foreground/40" />
            <p className="text-[13px] text-muted-foreground">No agents yet</p>
            <Button size="sm" onClick={() => setModal("create")}>
              <Plus className="mr-1.5 size-3.5" />
              Create your first agent
            </Button>
          </div>
        ) : (
          <>
            <DataTable
              columns={columns}
              data={filtered}
              keyExtractor={(row) => row.id ?? ""}
              mobileCard={(row) => <AgentMobileCard agent={row} />}
            />

            {/* Pagination */}
            <div className="mt-4 flex items-center justify-between border-t border-border pt-4">
              <span className="text-[13px] text-muted-foreground">Page {pageNumber}</span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={cursors.length === 0}
                  onClick={() => setCursors((prev) => prev.slice(0, -1))}
                >
                  <ChevronLeft className="mr-1 size-3.5" />
                  Previous
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!hasMore}
                  onClick={() => {
                    const lastItem = agents[agents.length - 1];
                    if (lastItem?.id) setCursors((prev) => [...prev, lastItem.id!]);
                  }}
                >
                  Next
                  <ChevronRight className="ml-1 size-3.5" />
                </Button>
              </div>
            </div>
          </>
        )}
      </div>

      <CreateAgentDialog open={modal === "create"} onOpenChange={(o) => setModal(o ? "create" : "closed")} />
    </>
  );
}
