"use client"

import { Area, AreaChart, CartesianGrid, XAxis } from "recharts"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowUp01Icon, ArrowDown01Icon } from "@hugeicons/core-free-icons"
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"

type StatCardProps = {
  label: string
  value: string
  trend?: number
  accent?: boolean
  chartData?: { label: string; value: number }[]
  chartColor?: string
  children?: React.ReactNode
}

function StatCard({ label, value, trend, accent, chartData, chartColor, children }: StatCardProps) {
  const isPositive = trend !== undefined && trend >= 0
  const isNeutral = trend === undefined
  const color = chartColor ?? (isPositive ? "var(--color-chart-positive)" : "var(--color-chart-negative)")

  const chartConfig: ChartConfig = {
    value: {
      label,
      color,
    },
  }

  return (
    <div className="flex flex-col rounded-xl border border-border overflow-hidden">
      <div className="flex flex-col gap-1 p-4 pb-2">
        <span className="text-xs text-muted-foreground">{label}</span>
        <div className="flex items-end justify-between">
          <span className={`font-mono text-2xl font-semibold tabular-nums ${accent ? "text-primary" : "text-foreground"}`}>
            {value}
          </span>
          {!isNeutral && (
            <span className={`flex items-center gap-0.5 text-xs ${isPositive ? "text-green-500" : "text-destructive"}`}>
              <HugeiconsIcon icon={isPositive ? ArrowUp01Icon : ArrowDown01Icon} size={12} />
              {Math.abs(trend!)}%
            </span>
          )}
          {children}
        </div>
      </div>
      {chartData && (
        <div className="px-1 pb-1">
          <ChartContainer config={chartConfig} className="h-[48px] w-full">
            <AreaChart data={chartData} margin={{ top: 4, right: 4, bottom: 0, left: 4 }}>
              <defs>
                <linearGradient id={`fill-${label.replace(/\s/g, "")}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={color} stopOpacity={0.2} />
                  <stop offset="100%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid vertical={false} horizontal={false} />
              <XAxis dataKey="label" hide />
              <ChartTooltip
                cursor={false}
                content={<ChartTooltipContent indicator="line" />}
              />
              <Area
                type="monotone"
                dataKey="value"
                stroke={color}
                strokeWidth={1.5}
                fill={`url(#fill-${label.replace(/\s/g, "")})`}
                dot={false}
                activeDot={{ r: 3, strokeWidth: 0 }}
              />
            </AreaChart>
          </ChartContainer>
        </div>
      )}
    </div>
  )
}

type Stats = {
  totalRuns: number
  totalRunsTrend: number
  activeNow: number
  spendThisMonth: number
  spendTrend: number
  tokensThisMonth: number
  tokensTrend: number
  avgCostPerRun: number
  avgCostTrend: number
}

function formatTokens(n: number) {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}k`
  return n.toString()
}

const runsData = [
  { label: "Jun 1", value: 18 },
  { label: "Jun 3", value: 24 },
  { label: "Jun 5", value: 20 },
  { label: "Jun 7", value: 32 },
  { label: "Jun 9", value: 28 },
  { label: "Jun 11", value: 35 },
  { label: "Jun 13", value: 42 },
  { label: "Jun 15", value: 38 },
  { label: "Jun 17", value: 45 },
  { label: "Jun 19", value: 40 },
  { label: "Jun 21", value: 48 },
  { label: "Jun 23", value: 52 },
  { label: "Jun 25", value: 46 },
  { label: "Jun 27", value: 55 },
]

const spendData = [
  { label: "Jun 1", value: 2.1 },
  { label: "Jun 3", value: 3.4 },
  { label: "Jun 5", value: 2.8 },
  { label: "Jun 7", value: 4.2 },
  { label: "Jun 9", value: 3.6 },
  { label: "Jun 11", value: 5.1 },
  { label: "Jun 13", value: 4.8 },
  { label: "Jun 15", value: 6.2 },
  { label: "Jun 17", value: 5.5 },
  { label: "Jun 19", value: 7.1 },
  { label: "Jun 21", value: 6.8 },
  { label: "Jun 23", value: 8.2 },
  { label: "Jun 25", value: 7.4 },
  { label: "Jun 27", value: 9.0 },
]

const tokensData = [
  { label: "Jun 1", value: 120000 },
  { label: "Jun 3", value: 180000 },
  { label: "Jun 5", value: 150000 },
  { label: "Jun 7", value: 220000 },
  { label: "Jun 9", value: 190000 },
  { label: "Jun 11", value: 280000 },
  { label: "Jun 13", value: 250000 },
  { label: "Jun 15", value: 320000 },
  { label: "Jun 17", value: 290000 },
  { label: "Jun 19", value: 380000 },
  { label: "Jun 21", value: 340000 },
  { label: "Jun 23", value: 420000 },
  { label: "Jun 25", value: 380000 },
  { label: "Jun 27", value: 460000 },
]

const avgCostData = [
  { label: "Jun 1", value: 0.058 },
  { label: "Jun 3", value: 0.052 },
  { label: "Jun 5", value: 0.061 },
  { label: "Jun 7", value: 0.048 },
  { label: "Jun 9", value: 0.055 },
  { label: "Jun 11", value: 0.044 },
  { label: "Jun 13", value: 0.046 },
  { label: "Jun 15", value: 0.042 },
  { label: "Jun 17", value: 0.048 },
  { label: "Jun 19", value: 0.040 },
  { label: "Jun 21", value: 0.043 },
  { label: "Jun 23", value: 0.038 },
  { label: "Jun 25", value: 0.041 },
  { label: "Jun 27", value: 0.036 },
]

export function StatCards({ stats }: { stats: Stats }) {
  return (
    <div className="grid grid-cols-2 lg:grid-cols-5 gap-3 mb-8">
      <StatCard
        label="Total runs"
        value={stats.totalRuns.toLocaleString()}
        trend={stats.totalRunsTrend}
        chartData={runsData}
        chartColor="rgb(34, 197, 94)"
      />
      <StatCard label="Active now" value={stats.activeNow.toString()} accent>
        <div className="flex items-center gap-1">
          {Array.from({ length: stats.activeNow }).map((_, i) => (
            <span key={i} className="h-1.5 w-1.5 rounded-full bg-green-500 animate-pulse" />
          ))}
        </div>
      </StatCard>
      <StatCard
        label="Spend this month"
        value={`$${stats.spendThisMonth.toFixed(2)}`}
        trend={stats.spendTrend}
        chartData={spendData}
        chartColor="rgb(34, 197, 94)"
      />
      <StatCard
        label="Tokens this month"
        value={formatTokens(stats.tokensThisMonth)}
        trend={stats.tokensTrend}
        chartData={tokensData}
        chartColor="rgb(34, 197, 94)"
      />
      <div className="col-span-2 lg:col-span-1">
        <StatCard
          label="Avg cost / run"
          value={`$${stats.avgCostPerRun.toFixed(3)}`}
          trend={stats.avgCostTrend}
          chartData={avgCostData}
          chartColor={stats.avgCostTrend <= 0 ? "rgb(34, 197, 94)" : "rgb(239, 68, 68)"}
        />
      </div>
    </div>
  )
}
