"use client"

import Link from "next/link"
import { useEffect, useMemo, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  ArrowRight01Icon,
  Delete02Icon,
  MoreHorizontalIcon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/confirm-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]
type Status = "active" | "paused" | "error" | "draft"

interface EmployeeGroup {
  name: string
  employees: Employee[]
}

const COLUMN_GRID = "grid-cols-[1.6fr_1.9fr_0.9fr_0.9fr_2.5rem]"

export default function WorkspaceHome() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const queryClient = useQueryClient()
  const { activeOrg } = useAuth()
  const [filter, setFilter] = useState("")
  const [deleting, setDeleting] = useState<Employee | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/employees")
  const deleteEmployee = $api.useMutation("delete", "/v1/agents/{id}")
  const employees = useMemo(() => data?.data ?? [], [data])

  useEffect(() => {
    if (searchParams.get("checkout") === "success") {
      toast.success("Subscription activated! You're on the Pro plan.")
      router.replace("/w")
    }
  }, [searchParams, router])

  const filtered = useMemo(() => {
    const query = filter.trim().toLowerCase()
    if (!query) return employees
    return employees.filter((employee) =>
      [
        employee.name,
        employee.description,
        employee.category,
        employee.model,
        employee.team,
        employee.status,
        employee.sandbox?.status,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(query)
    )
  }, [employees, filter])

  const groups = useMemo(() => groupEmployees(filtered), [filtered])
  const activeCount = employees.filter(
    (employee) => normalizeStatus(employee.status) === "active"
  ).length
  const attentionCount = employees.filter((employee) =>
    ["error", "paused"].includes(normalizeStatus(employee.status))
  ).length

  function handleDeleteEmployee() {
    if (!deleting?.id) return

    deleteEmployee.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.name ?? "Employee"}" deleted`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to delete employee"))
          setDeleting(null)
        },
      }
    )
  }

  return (
    <>
      <main className="mx-auto w-full max-w-6xl px-6 pb-32 pt-14 sm:px-10">
        <EmployeesHeader
          orgName={activeOrg?.name ?? "Workspace"}
          total={employees.length}
          active={activeCount}
          attention={attentionCount}
          filter={filter}
          onFilterChange={setFilter}
        />

        <div className="mt-14 flex flex-col gap-12">
          {isLoading ? (
            <EmployeesTableSkeleton />
          ) : employees.length === 0 ? (
            <EmployeesEmpty />
          ) : groups.length === 0 ? (
            <div className="rounded-2xl border border-border px-5 py-12 text-center text-sm text-muted-foreground">
              No employees match your filter.
            </div>
          ) : (
            groups.map((group) => (
              <TeamSection
                key={group.name}
                group={group}
                onDeleteEmployee={setDeleting}
              />
            ))
          )}
        </div>
      </main>

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null)
        }}
        title="Delete employee"
        description={`This will permanently delete "${deleting?.name ?? "this employee"}" and all related employee data. This action cannot be undone.`}
        confirmText={deleting?.name ?? ""}
        confirmLabel="Delete employee"
        destructive
        loading={deleteEmployee.isPending}
        onConfirm={handleDeleteEmployee}
      />
    </>
  )
}

function EmployeeActions({ onDelete }: { onDelete: () => void }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex size-8 items-center justify-center rounded-lg outline-none transition-colors hover:bg-muted">
        <HugeiconsIcon
          icon={MoreHorizontalIcon}
          className="size-4 text-muted-foreground"
          strokeWidth={2}
        />
        <span className="sr-only">Open employee actions</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={4} className="w-44">
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} className="size-4" strokeWidth={2} />
            Delete employee
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function TeamSection({
  group,
  onDeleteEmployee,
}: {
  group: EmployeeGroup
  onDeleteEmployee: (employee: Employee) => void
}) {
  return (
    <section className="flex flex-col gap-4">
      <header className="flex items-baseline justify-between gap-6">
        <div className="flex items-baseline gap-3">
          <h2 className="text-base font-semibold tracking-tight">
            {group.name}
          </h2>
          <p className="text-xs text-muted-foreground">
            {group.employees.length}{" "}
            {group.employees.length === 1 ? "employee" : "employees"}
          </p>
        </div>
        <Link
          href="/w/teams"
          className="flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Open teams
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            className="size-3.5"
            strokeWidth={2}
          />
        </Link>
      </header>

      <EmployeeTable
        employees={group.employees}
        onDeleteEmployee={onDeleteEmployee}
      />
    </section>
  )
}

function EmployeeTable({
  employees,
  onDeleteEmployee,
}: {
  employees: Employee[]
  onDeleteEmployee: (employee: Employee) => void
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-card">
      <div
        className={cn(
          "hidden items-center border-b border-border px-5 py-3 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground lg:grid",
          COLUMN_GRID
        )}
      >
        <span>Employee</span>
        <span>Role</span>
        <span>Status</span>
        <span className="text-right">Last active</span>
        <span className="sr-only">Actions</span>
      </div>
      <ul className="divide-y divide-border">
        {employees.map((employee) => (
          <EmployeeRow
            key={employee.id ?? employee.name}
            employee={employee}
            onDelete={() => onDeleteEmployee(employee)}
          />
        ))}
      </ul>
    </div>
  )
}

function EmployeeRow({
  employee,
  onDelete,
}: {
  employee: Employee
  onDelete: () => void
}) {
  const name = employee.name ?? "Unnamed employee"
  const role = employee.category || employee.description || "Coordinator"
  const status = normalizeStatus(employee.status)
  const lastActive = formatLastActive(
    employee.sandbox?.last_active_at ?? employee.updated_at
  )

  return (
    <li
      className={cn(
        "grid gap-3 px-5 py-4 text-sm transition-colors hover:bg-muted/40 lg:items-center lg:gap-0 lg:py-3.5",
        "grid-cols-1 lg:grid",
        COLUMN_GRID
      )}
    >
      <div className="flex min-w-0 items-center gap-3">
        <Avatar size="sm">
          {employee.avatar_url ? (
            <AvatarImage src={employee.avatar_url} alt="" />
          ) : null}
          <AvatarFallback>{name.charAt(0).toUpperCase()}</AvatarFallback>
        </Avatar>
        <span className="truncate font-medium">{name}</span>
      </div>
      <span className="truncate text-foreground/90">{role}</span>
      <span>
        <StatusBadge status={status} />
      </span>
      <span className="text-muted-foreground lg:text-right">{lastActive}</span>
      <div className="flex justify-start lg:justify-end">
        <EmployeeActions onDelete={onDelete} />
      </div>
    </li>
  )
}

function EmployeesHeader({
  orgName,
  total,
  active,
  attention,
  filter,
  onFilterChange,
}: {
  orgName: string
  total: number
  active: number
  attention: number
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
          All employees
        </h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-semibold text-foreground">{total}</span>{" "}
          {total === 1 ? "employee" : "employees"},{" "}
          <span className="font-semibold text-foreground">{active}</span>{" "}
          active
          {attention > 0 ? (
            <>
              .{" "}
              <span className="text-destructive underline decoration-destructive/40 underline-offset-4">
                {attention} need attention.
              </span>
            </>
          ) : (
            "."
          )}
        </p>
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <FilterField value={filter} onChange={onFilterChange} />
        <Button render={<Link href="/w/employees/new" />}>
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2.25}
            data-icon="inline-start"
          />
          New employee
        </Button>
      </div>
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
        placeholder="Filter employees"
        className="pl-9"
      />
    </div>
  )
}


const STATUS_PRESETS: Record<
  Status,
  { label: string; chip: string; dot: string }
> = {
  active: {
    label: "Active",
    chip: "bg-success/15 text-success",
    dot: "bg-success",
  },
  paused: {
    label: "Paused",
    chip: "bg-muted text-muted-foreground",
    dot: "bg-muted-foreground/70",
  },
  error: {
    label: "Error",
    chip: "bg-destructive/15 text-destructive",
    dot: "bg-destructive",
  },
  draft: {
    label: "Draft",
    chip: "bg-muted/60 text-muted-foreground",
    dot: "bg-muted-foreground/50",
  },
}

function StatusBadge({ status }: { status: Status }) {
  const preset = STATUS_PRESETS[status]
  return (
    <Badge variant="ghost" className={cn("gap-1.5", preset.chip)}>
      <span className={cn("size-1.5 rounded-full", preset.dot)} />
      {preset.label}
    </Badge>
  )
}

function EmployeesTableSkeleton() {
  return (
    <div className="flex flex-col gap-12" aria-busy="true">
      {Array.from({ length: 2 }).map((_, sectionIndex) => (
        <section key={sectionIndex} className="flex flex-col gap-4">
          <div className="flex items-center gap-3">
            <Skeleton className="h-5 w-44 rounded-md" />
            <Skeleton className="h-3 w-24 rounded-md" />
          </div>
          <div className="overflow-hidden rounded-2xl border border-border">
            <div
              className={cn(
                "hidden items-center border-b border-border px-5 py-3 lg:grid",
                COLUMN_GRID
              )}
            >
              {Array.from({ length: 5 }).map((__, headerIndex) => (
                <Skeleton
                  key={headerIndex}
                  className="h-3 w-20 rounded-md"
                />
              ))}
            </div>
            {Array.from({ length: 3 }).map((__, rowIndex) => (
              <div
                key={rowIndex}
                className={cn(
                  "grid gap-3 border-b border-border px-5 py-4 last:border-b-0 lg:gap-0",
                  COLUMN_GRID
                )}
              >
                {Array.from({ length: 5 }).map((___, cellIndex) => (
                  <Skeleton
                    key={cellIndex}
                    className="h-5 w-24 rounded-md"
                  />
                ))}
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function EmployeesEmpty() {
  return (
    <div className="rounded-2xl border border-border px-5 py-16 text-center">
      <h3 className="text-sm font-semibold text-foreground">
        No employees yet
      </h3>
      <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">
        Complete onboarding to create your coordinator employee and connect it
        to a channel.
      </p>
    </div>
  )
}

function groupEmployees(employees: Employee[]): EmployeeGroup[] {
  const groups = new Map<string, Employee[]>()
  for (const employee of employees) {
    const key = employee.team?.trim() || "Unassigned"
    groups.set(key, [...(groups.get(key) ?? []), employee])
  }
  return Array.from(groups.entries())
    .sort(([a], [b]) => {
      if (a === "Unassigned") return 1
      if (b === "Unassigned") return -1
      return a.localeCompare(b)
    })
    .map(([name, groupedEmployees]) => ({
      name,
      employees: groupedEmployees,
    }))
}

function normalizeStatus(status?: string): Status {
  const normalized = status?.toLowerCase()
  if (normalized === "paused") return "paused"
  if (normalized === "error" || normalized === "failed") return "error"
  if (normalized === "draft") return "draft"
  return "active"
}

function formatLastActive(value?: string) {
  if (!value) return "never run"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "unknown"
  const diffMs = Date.now() - date.getTime()
  const diffMinutes = Math.max(0, Math.floor(diffMs / 60000))
  if (diffMinutes < 1) return "just now"
  if (diffMinutes < 60) return `${diffMinutes} min ago`
  const diffHours = Math.floor(diffMinutes / 60)
  if (diffHours < 24) return `${diffHours} hr ago`
  const diffDays = Math.floor(diffHours / 24)
  if (diffDays < 7) return `${diffDays} day${diffDays === 1 ? "" : "s"} ago`
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
  }).format(date)
}
