"use client"

import { $api } from "@/lib/api/hooks"
import { PageHeader } from "@/components/admin/page-header"
import { StatCard } from "@/components/admin/stat-card"
import { Skeleton } from "@/components/ui/skeleton"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

function formatCurrency(value: number | undefined): string {
  if (value == null) return "$0.00"
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(value)
}

function formatNumber(value: number | undefined): string {
  if (value == null) return "0"
  return new Intl.NumberFormat("en-US").format(value)
}

function StatCardSkeleton() {
  return (
    <Card>
      <CardHeader className="pb-2">
        <Skeleton className="h-4 w-24" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-8 w-16" />
        <Skeleton className="mt-1 h-3 w-32" />
      </CardContent>
    </Card>
  )
}

function TableSkeleton({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {Array.from({ length: cols }).map((_, i) => (
            <TableHead key={i}>
              <Skeleton className="h-4 w-20" />
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {Array.from({ length: rows }).map((_, i) => (
          <TableRow key={i}>
            {Array.from({ length: cols }).map((_, j) => (
              <TableCell key={j}>
                <Skeleton className="h-4 w-24" />
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

export default function DashboardPage() {
  const { data: stats, isLoading: statsLoading, error: statsError } = $api.useQuery(
    "get",
    "/admin/v1/stats"
  )

  const { data: genStats, isLoading: genStatsLoading, error: genStatsError } = $api.useQuery(
    "get",
    "/admin/v1/generations/stats"
  )

  return (
    <div className="space-y-8">
      <PageHeader
        title="Overview"
        description="Platform-wide statistics and generation metrics."
      />

      {/* Stats grid */}
      {statsLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <StatCardSkeleton key={i} />
          ))}
        </div>
      ) : statsError ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-destructive">
            Failed to load platform statistics.
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard title="Total Users" value={formatNumber(stats?.total_users)} />
          <StatCard title="Total Organizations" value={formatNumber(stats?.total_orgs)} />
          <StatCard title="Total Agents" value={formatNumber(stats?.total_agents)} />
          <StatCard title="Total Credentials" value={formatNumber(stats?.total_credentials)} />
          <StatCard
            title="Sandboxes Running"
            value={formatNumber(stats?.total_sandboxes_running)}
            subtitle={`${formatNumber(stats?.total_sandboxes_stopped)} stopped, ${formatNumber(stats?.total_sandboxes_error)} error`}
          />
          <StatCard
            title="Active Conversations"
            value={formatNumber(stats?.total_conversations_active)}
          />
          <StatCard
            title="Total Generations"
            value={formatNumber(stats?.total_generations)}
          />
          <StatCard
            title="Total Cost"
            value={formatCurrency(stats?.total_cost)}
          />
        </div>
      )}

      {/* Generation stats */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* By Provider */}
        <Card>
          <CardHeader>
            <CardTitle>Generations by Provider</CardTitle>
          </CardHeader>
          <CardContent>
            {genStatsLoading ? (
              <TableSkeleton rows={4} cols={4} />
            ) : genStatsError ? (
              <p className="py-4 text-center text-sm text-destructive">
                Failed to load provider statistics.
              </p>
            ) : !genStats?.by_provider?.length ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                No generation data yet.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Provider</TableHead>
                    <TableHead className="text-right">Requests</TableHead>
                    <TableHead className="text-right">Tokens (In/Out)</TableHead>
                    <TableHead className="text-right">Cost</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {genStats.by_provider.map((entry) => (
                    <TableRow key={entry.provider_id}>
                      <TableCell className="font-medium">{entry.provider_id}</TableCell>
                      <TableCell className="text-right">
                        {formatNumber(entry.count)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatNumber(entry.input_tokens)} / {formatNumber(entry.output_tokens)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatCurrency(entry.cost)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* By Model */}
        <Card>
          <CardHeader>
            <CardTitle>Generations by Model</CardTitle>
          </CardHeader>
          <CardContent>
            {genStatsLoading ? (
              <TableSkeleton rows={4} cols={4} />
            ) : genStatsError ? (
              <p className="py-4 text-center text-sm text-destructive">
                Failed to load model statistics.
              </p>
            ) : !genStats?.by_model?.length ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                No generation data yet.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Model</TableHead>
                    <TableHead className="text-right">Requests</TableHead>
                    <TableHead className="text-right">Tokens (In/Out)</TableHead>
                    <TableHead className="text-right">Cost</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {genStats.by_model.map((entry) => (
                    <TableRow key={entry.model}>
                      <TableCell className="font-medium">{entry.model}</TableCell>
                      <TableCell className="text-right">
                        {formatNumber(entry.count)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatNumber(entry.input_tokens)} / {formatNumber(entry.output_tokens)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatCurrency(entry.cost)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
