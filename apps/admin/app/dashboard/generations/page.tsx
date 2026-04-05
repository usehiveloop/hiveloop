"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { PageHeader } from "@/components/admin/page-header"
import { StatCard } from "@/components/admin/stat-card"
import { TimeAgo } from "@/components/admin/time-ago"
import { StatusBadge } from "@/components/admin/status-badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

function formatCurrency(value: number | undefined) {
  if (value == null) return "$0.00"
  return `$${value.toFixed(4)}`
}

function formatTokens(value: number | undefined) {
  if (value == null) return "0"
  return value.toLocaleString()
}

export default function GenerationsPage() {
  const [orgId, setOrgId] = useState("")
  const [providerId, setProviderId] = useState("")
  const [model, setModel] = useState("")

  const { data: statsData, isLoading: statsLoading } = $api.useQuery(
    "get",
    "/admin/v1/generations/stats",
    {
      params: { query: { org_id: orgId || undefined } },
    }
  )

  const { data: listData, isLoading: listLoading } = $api.useQuery(
    "get",
    "/admin/v1/generations",
    {
      params: {
        query: {
          org_id: orgId || undefined,
          provider_id: providerId || undefined,
          model: model || undefined,
        },
      },
    }
  )

  const stats = statsData as {
    total_generations?: number
    total_cost?: number
    total_input_tokens?: number
    total_output_tokens?: number
    by_provider?: { provider_id?: string; count?: number; cost?: number; input_tokens?: number; output_tokens?: number }[]
    by_model?: { model?: string; count?: number; cost?: number; input_tokens?: number; output_tokens?: number }[]
  } | undefined

  const generations = ((listData as { data?: Record<string, unknown>[] })?.data ?? []) as {
    id?: string
    org_id?: string
    provider_id?: string
    model?: string
    input_tokens?: number
    output_tokens?: number
    cost?: number
    status?: string
    created_at?: string
  }[]

  return (
    <div className="space-y-6">
      <PageHeader
        title="Generations"
        description="LLM generation logs and statistics."
      />

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter by Org ID..."
          value={orgId}
          onChange={(e) => setOrgId(e.target.value)}
          className="max-w-xs"
        />
        <Input
          placeholder="Filter by Provider ID..."
          value={providerId}
          onChange={(e) => setProviderId(e.target.value)}
          className="max-w-xs"
        />
        <Input
          placeholder="Filter by Model..."
          value={model}
          onChange={(e) => setModel(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {/* Stats Cards */}
      {statsLoading ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            title="Total Generations"
            value={formatTokens(stats?.total_generations)}
          />
          <StatCard
            title="Total Cost"
            value={formatCurrency(stats?.total_cost)}
          />
          <StatCard
            title="Input Tokens"
            value={formatTokens(stats?.total_input_tokens)}
          />
          <StatCard
            title="Output Tokens"
            value={formatTokens(stats?.total_output_tokens)}
          />
        </div>
      )}

      {/* By Provider Table */}
      {stats?.by_provider && stats.by_provider.length > 0 && (
        <div className="space-y-3">
          <h2 className="text-lg font-semibold">By Provider</h2>
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Provider</TableHead>
                  <TableHead className="text-right">Count</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                  <TableHead className="text-right">Input Tokens</TableHead>
                  <TableHead className="text-right">Output Tokens</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {stats.by_provider.map((entry) => (
                  <TableRow key={entry.provider_id}>
                    <TableCell className="font-medium">
                      {entry.provider_id ?? "--"}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.count)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatCurrency(entry.cost)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.input_tokens)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.output_tokens)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      )}

      {/* By Model Table */}
      {stats?.by_model && stats.by_model.length > 0 && (
        <div className="space-y-3">
          <h2 className="text-lg font-semibold">By Model</h2>
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right">Count</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                  <TableHead className="text-right">Input Tokens</TableHead>
                  <TableHead className="text-right">Output Tokens</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {stats.by_model.map((entry) => (
                  <TableRow key={entry.model}>
                    <TableCell className="font-medium">
                      {entry.model ?? "--"}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.count)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatCurrency(entry.cost)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.input_tokens)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(entry.output_tokens)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      )}

      {/* Recent Generations Table */}
      <div className="space-y-3">
        <h2 className="text-lg font-semibold">Recent Generations</h2>
        {listLoading ? (
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-12 w-full" />
            ))}
          </div>
        ) : generations.length === 0 ? (
          <div className="flex h-48 items-center justify-center rounded-lg border border-border">
            <p className="text-sm text-muted-foreground">No generations found.</p>
          </div>
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>ID</TableHead>
                  <TableHead>Org ID</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right">Input Tokens</TableHead>
                  <TableHead className="text-right">Output Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {generations.map((gen) => (
                  <TableRow key={gen.id}>
                    <TableCell className="font-mono text-xs">
                      {gen.id ?? "--"}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {gen.org_id ?? "--"}
                    </TableCell>
                    <TableCell>{gen.provider_id ?? "--"}</TableCell>
                    <TableCell>{gen.model ?? "--"}</TableCell>
                    <TableCell className="text-right">
                      {formatTokens(gen.input_tokens)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatTokens(gen.output_tokens)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatCurrency(gen.cost)}
                    </TableCell>
                    <TableCell>
                      {gen.status ? <StatusBadge status={gen.status} /> : "--"}
                    </TableCell>
                    <TableCell>
                      <TimeAgo date={gen.created_at} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </div>
  )
}
