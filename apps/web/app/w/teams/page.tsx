"use client"

import { useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon } from "@hugeicons/core-free-icons"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Team = components["schemas"]["teamResponse"]
type Employee = components["schemas"]["employeeListItem"]

const TEAM_COLUMN_GRID = "grid-cols-[1.4fr_1.8fr_0.8fr_0.9fr]"

export default function TeamsPage() {
  const { activeOrg } = useAuth()
  const [filter, setFilter] = useState("")
  const { data, isLoading } = $api.useQuery("get", "/v1/teams")
  const { data: employeesData } = $api.useQuery("get", "/v1/employees")
  const teams = useMemo(() => data?.data ?? [], [data])
  const employees = useMemo(() => employeesData?.data ?? [], [employeesData])
  const countsByTeam = useMemo(() => countEmployeesByTeam(employees), [employees])

  const filtered = useMemo(() => {
    const query = filter.trim().toLowerCase()
    if (!query) return teams
    return teams.filter((team) =>
      [
        team.name,
        team.description,
        String(countsByTeam.get(team.name ?? "") ?? 0),
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(query)
    )
  }, [countsByTeam, filter, teams])

  const totalEmployees = useMemo(
    () =>
      teams.reduce(
        (total, team) => total + (countsByTeam.get(team.name ?? "") ?? 0),
        0
      ),
    [countsByTeam, teams]
  )

  return (
    <main className="mx-auto w-full max-w-6xl px-6 pb-32 pt-14 sm:px-10">
      <TeamsHeader
        orgName={activeOrg?.name ?? "Workspace"}
        total={teams.length}
        employeeTotal={totalEmployees}
        filter={filter}
        onFilterChange={setFilter}
      />

      <div className="mt-14 flex flex-col gap-12">
        {isLoading ? (
          <TeamsTableSkeleton />
        ) : teams.length === 0 ? (
          <TeamsEmpty />
        ) : filtered.length === 0 ? (
          <div className="rounded-2xl border border-border px-5 py-12 text-center text-sm text-muted-foreground">
            No teams match your filter.
          </div>
        ) : (
          <TeamSection teams={filtered} countsByTeam={countsByTeam} />
        )}
      </div>
    </main>
  )
}

function TeamsHeader({
  orgName,
  total,
  employeeTotal,
  filter,
  onFilterChange,
}: {
  orgName: string
  total: number
  employeeTotal: number
  filter: string
  onFilterChange: (value: string) => void
}) {
  return (
    <section className="flex flex-col gap-8 lg:flex-row lg:items-end lg:justify-between">
      <div className="flex flex-col gap-3">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          {orgName} Workforce
        </p>
        <h1 className="font-display text-4xl font-medium tracking-tight">
          All teams
        </h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-semibold text-foreground">{total}</span>{" "}
          {total === 1 ? "team" : "teams"},{" "}
          <span className="font-semibold text-foreground">
            {employeeTotal}
          </span>{" "}
          {employeeTotal === 1 ? "employee" : "employees"} assigned.
        </p>
      </div>
      <FilterField value={filter} onChange={onFilterChange} />
    </section>
  )
}

function FilterField({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="relative w-full sm:w-60">
      <HugeiconsIcon
        icon={Search01Icon}
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        strokeWidth={2}
      />
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Filter teams"
        className="pl-9"
      />
    </div>
  )
}

function TeamSection({
  teams,
  countsByTeam,
}: {
  teams: Team[]
  countsByTeam: Map<string, number>
}) {
  return (
    <section className="flex flex-col gap-4">
      <header className="flex items-baseline justify-between gap-6">
        <div className="flex items-baseline gap-3">
          <h2 className="text-base font-semibold tracking-tight">Teams</h2>
          <p className="text-xs text-muted-foreground">
            {teams.length} {teams.length === 1 ? "team" : "teams"}
          </p>
        </div>
      </header>

      <TeamsTable teams={teams} countsByTeam={countsByTeam} />
    </section>
  )
}

function TeamsTable({
  teams,
  countsByTeam,
}: {
  teams: Team[]
  countsByTeam: Map<string, number>
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-card">
      <div
        className={cn(
          "hidden items-center border-b border-border px-5 py-3 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground lg:grid",
          TEAM_COLUMN_GRID
        )}
      >
        <span>Team</span>
        <span>Description</span>
        <span>Employees</span>
        <span className="text-right">Created</span>
      </div>
      <ul className="divide-y divide-border">
        {teams.map((team) => (
          <TeamRow
            key={team.id ?? team.name}
            team={team}
            employeeCount={countsByTeam.get(team.name ?? "") ?? 0}
          />
        ))}
      </ul>
    </div>
  )
}

function TeamRow({
  team,
  employeeCount,
}: {
  team: Team
  employeeCount: number
}) {
  const name = team.name ?? "Untitled team"
  const description = team.description || "No description yet."

  return (
    <li
      className={cn(
        "grid gap-3 px-5 py-4 text-sm transition-colors hover:bg-muted/40 lg:items-center lg:gap-0 lg:py-3.5",
        "grid-cols-1 lg:grid",
        TEAM_COLUMN_GRID
      )}
    >
      <span className="truncate font-medium">{name}</span>
      <span className="truncate text-foreground/90">{description}</span>
      <span className="font-medium tabular-nums">
        {employeeCount} {employeeCount === 1 ? "employee" : "employees"}
      </span>
      <span className="text-muted-foreground lg:text-right">
        {formatDate(team.created_at)}
      </span>
    </li>
  )
}

function TeamsTableSkeleton() {
  return (
    <section className="flex flex-col gap-4" aria-busy="true">
      <div className="flex items-center gap-3">
        <Skeleton className="h-5 w-44 rounded-md" />
        <Skeleton className="h-3 w-24 rounded-md" />
      </div>
      <div className="overflow-hidden rounded-2xl border border-border">
        <div
          className={cn(
            "hidden items-center border-b border-border px-5 py-3 lg:grid",
            TEAM_COLUMN_GRID
          )}
        >
          {Array.from({ length: 4 }).map((_, headerIndex) => (
            <Skeleton key={headerIndex} className="h-3 w-20 rounded-md" />
          ))}
        </div>
        {Array.from({ length: 4 }).map((_, rowIndex) => (
          <div
            key={rowIndex}
            className={cn(
              "grid gap-3 border-b border-border px-5 py-4 last:border-b-0 lg:gap-0",
              TEAM_COLUMN_GRID
            )}
          >
            {Array.from({ length: 4 }).map((__, cellIndex) => (
              <Skeleton key={cellIndex} className="h-5 w-24 rounded-md" />
            ))}
          </div>
        ))}
      </div>
    </section>
  )
}

function TeamsEmpty() {
  return (
    <div className="rounded-2xl border border-border px-5 py-16 text-center">
      <h3 className="text-sm font-semibold text-foreground">No teams yet</h3>
      <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">
        Teams will appear here once they are created for this workspace.
      </p>
    </div>
  )
}

function countEmployeesByTeam(employees: Employee[]) {
  const counts = new Map<string, number>()
  for (const employee of employees) {
    const team = employee.team?.trim()
    if (!team) continue
    counts.set(team, (counts.get(team) ?? 0) + 1)
  }
  return counts
}

function formatDate(value?: string) {
  if (!value) return "Unknown"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "Unknown"
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date)
}
