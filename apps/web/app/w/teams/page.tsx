"use client"

import { useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Clock01Icon,
  Search01Icon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type Team = components["schemas"]["teamResponse"]
type Employee = components["schemas"]["employeeListItem"]

export default function TeamsPage() {
  const [search, setSearch] = useState("")
  const { data, isLoading } = $api.useQuery("get", "/v1/teams")
  const { data: employeesData } = $api.useQuery("get", "/v1/employees")
  const teams = useMemo(() => data?.data ?? [], [data])
  const employees = useMemo(() => employeesData?.data ?? [], [employeesData])
  const countsByTeam = useMemo(() => countEmployeesByTeam(employees), [employees])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return teams
    return teams.filter((team) =>
      [team.name, team.description]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(query)
    )
  }, [search, teams])

  return (
    <>
      <PageHeader title="Teams" />
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-6 py-10">
        <div className="max-w-2xl">
          <h2 className="font-heading text-2xl font-medium tracking-tight text-foreground">
            Teams
          </h2>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            Organize employees, memory, and operating context around the groups
            in your company.
          </p>
        </div>

        <div className="relative max-w-sm">
          <HugeiconsIcon
            icon={Search01Icon}
            size={16}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search teams..."
            className="pl-9"
          />
        </div>

        {isLoading ? (
          <TeamsSkeleton />
        ) : teams.length === 0 ? (
          <TeamsEmpty />
        ) : filtered.length === 0 ? (
          <div className="rounded-2xl border border-border px-5 py-10 text-center text-sm text-muted-foreground">
            No teams match your search.
          </div>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {filtered.map((team) => (
              <TeamCard
                key={team.id ?? team.name}
                team={team}
                employeeCount={countsByTeam.get(team.name ?? "") ?? 0}
              />
            ))}
          </div>
        )}
      </div>
    </>
  )
}

function TeamCard({
  team,
  employeeCount,
}: {
  team: Team
  employeeCount: number
}) {
  return (
    <article className="rounded-2xl border border-border bg-background p-5 transition-colors hover:border-foreground/20">
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-muted text-muted-foreground">
            <HugeiconsIcon icon={UserGroupIcon} size={18} />
          </span>
          <div className="min-w-0">
            <h3 className="truncate text-sm font-semibold text-foreground">
              {team.name ?? "Untitled team"}
            </h3>
            {team.description ? (
              <p className="mt-1 line-clamp-3 text-sm leading-relaxed text-muted-foreground">
                {team.description}
              </p>
            ) : (
              <p className="mt-1 text-sm text-muted-foreground">
                No description yet.
              </p>
            )}
          </div>
        </div>
        <Badge variant="outline">
          {employeeCount} {employeeCount === 1 ? "employee" : "employees"}
        </Badge>
      </div>

      <div className="mt-5 grid grid-cols-2 gap-2 text-xs text-muted-foreground">
        <div className="rounded-xl bg-muted/35 px-3 py-2">
          <div className="flex items-center gap-1.5 text-[11px]">
            <HugeiconsIcon icon={UserGroupIcon} size={13} />
            Employees
          </div>
          <div className="mt-1 truncate text-[12px] font-medium text-foreground">
            {employeeCount}
          </div>
        </div>
        <div className="rounded-xl bg-muted/35 px-3 py-2">
          <div className="flex items-center gap-1.5 text-[11px]">
            <HugeiconsIcon icon={Clock01Icon} size={13} />
            Created
          </div>
          <div className="mt-1 truncate text-[12px] font-medium text-foreground">
            {formatDate(team.created_at)}
          </div>
        </div>
      </div>
    </article>
  )
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

function TeamsSkeleton() {
  return (
    <div className="grid gap-3 md:grid-cols-2" aria-busy="true">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="rounded-2xl border border-border p-5">
          <div className="flex items-start gap-3">
            <Skeleton className="size-9 rounded-xl" />
            <div className="flex-1 space-y-2">
              <Skeleton className="h-4 w-32 rounded-md" />
              <Skeleton className="h-3 w-full rounded-md" />
              <Skeleton className="h-3 w-2/3 rounded-md" />
            </div>
          </div>
          <div className="mt-5 grid grid-cols-2 gap-2">
            <Skeleton className="h-12 rounded-xl" />
            <Skeleton className="h-12 rounded-xl" />
          </div>
        </div>
      ))}
    </div>
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
