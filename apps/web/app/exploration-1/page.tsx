"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  CommandIcon,
  Search01Icon,
  Settings01Icon,
} from "@hugeicons/core-free-icons"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

type Status = "active" | "paused" | "error" | "draft"

interface Employee {
  initial: string
  name: string
  role: string
  model: string
  status: Status
  tasks7d: number | null
  successPct: number | null
  lastActive: string
  spark: number[]
}

interface Team {
  name: string
  weeklyTasks: number
  employees: Employee[]
}

const TEAMS: Team[] = [
  {
    name: "Platform Engineering",
    weeklyTasks: 243,
    employees: [
      {
        initial: "A",
        name: "Atlas",
        role: "Bug Triage Lead",
        model: "Claude Opus 4.7",
        status: "active",
        tasks7d: 184,
        successPct: 96,
        lastActive: "2 min ago",
        spark: [12, 18, 11, 24, 9, 22, 15, 28, 13, 19, 26, 17],
      },
      {
        initial: "B",
        name: "Boreas",
        role: "Incident Responder",
        model: "Claude Sonnet 4.7",
        status: "active",
        tasks7d: 47,
        successPct: 94,
        lastActive: "11 min ago",
        spark: [4, 8, 3, 12, 6, 9, 5, 14, 7, 4, 11, 8],
      },
      {
        initial: "C",
        name: "Ceres",
        role: "Release Notes Author",
        model: "Claude Haiku 4.7",
        status: "paused",
        tasks7d: 12,
        successPct: 100,
        lastActive: "3 days ago",
        spark: [2, 1, 4, 2, 5, 3, 4, 1, 0, 3, 2, 1],
      },
    ],
  },
  {
    name: "Customer Success",
    weeklyTasks: 130,
    employees: [
      {
        initial: "D",
        name: "Delphi",
        role: "Onboarding Specialist",
        model: "Claude Sonnet 4.7",
        status: "active",
        tasks7d: 92,
        successPct: 91,
        lastActive: "5 min ago",
        spark: [6, 10, 8, 14, 11, 9, 13, 16, 12, 18, 15, 17],
      },
      {
        initial: "E",
        name: "Echo",
        role: "Health Score Analyst",
        model: "Claude Opus 4.7",
        status: "active",
        tasks7d: 38,
        successPct: 88,
        lastActive: "1 hr ago",
        spark: [3, 6, 4, 9, 5, 8, 4, 11, 6, 9, 7, 10],
      },
    ],
  },
  {
    name: "Growth",
    weeklyTasks: 79,
    employees: [
      {
        initial: "F",
        name: "Fable",
        role: "Lifecycle Email Writer",
        model: "Claude Sonnet 4.7",
        status: "active",
        tasks7d: 61,
        successPct: 97,
        lastActive: "27 min ago",
        spark: [5, 4, 7, 6, 8, 5, 9, 7, 10, 6, 11, 8],
      },
      {
        initial: "G",
        name: "Gaia",
        role: "Experiment Analyst",
        model: "Claude Opus 4.7",
        status: "error",
        tasks7d: 18,
        successPct: 72,
        lastActive: "8 hr ago",
        spark: [3, 2, 4, 1, 3, 5, 2, 4, 1, 3, 2, 4],
      },
    ],
  },
  {
    name: "Sales",
    weeklyTasks: 396,
    employees: [
      {
        initial: "H",
        name: "Helios",
        role: "Sales Development Rep",
        model: "Claude Sonnet 4.7",
        status: "active",
        tasks7d: 240,
        successPct: 81,
        lastActive: "just now",
        spark: [18, 24, 16, 32, 20, 28, 22, 36, 26, 30, 34, 28],
      },
      {
        initial: "I",
        name: "Iris",
        role: "Demo Scheduler",
        model: "Claude Haiku 4.7",
        status: "active",
        tasks7d: 156,
        successPct: 99,
        lastActive: "4 min ago",
        spark: [10, 16, 9, 22, 13, 18, 14, 24, 15, 19, 21, 17],
      },
      {
        initial: "J",
        name: "Juno",
        role: "Pipeline Hygiene",
        model: "Claude Sonnet 4.7",
        status: "draft",
        tasks7d: null,
        successPct: null,
        lastActive: "never run",
        spark: [],
      },
    ],
  },
]

const COLUMN_GRID =
  "grid-cols-[1.4fr_1.4fr_1.3fr_0.9fr_1fr_0.7fr_0.9fr]"

export default function ExplorationOnePage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <TopBar />
      <main className="mx-auto w-full max-w-6xl px-10 pt-16 pb-32">
        <PageHeader />
        <div className="mt-14 flex flex-col gap-12">
          {TEAMS.map((team) => (
            <TeamSection key={team.name} team={team} />
          ))}
        </div>
      </main>
      <WorkspaceFloater />
    </div>
  )
}

function TopBar() {
  return (
    <header className="sticky top-0 z-20 flex items-center justify-between bg-background px-6 py-3.5">
      <div className="flex items-center gap-6">
        <BrandSwitcher />
        <span className="text-base text-muted-foreground/50">/</span>
        <OrgSwitcher />
        <nav className="ml-6 flex items-center gap-1">
          <TabButton active>Employees</TabButton>
          <TabButton>Teams</TabButton>
          <TabButton>Settings</TabButton>
        </nav>
      </div>
      <div className="flex items-center gap-2">
        <TopBarSearch />
        <Button variant="ghost" size="icon-sm" aria-label="Settings">
          <HugeiconsIcon icon={Settings01Icon} strokeWidth={1.75} />
        </Button>
        <Avatar size="sm">
          <AvatarFallback>BD</AvatarFallback>
        </Avatar>
      </div>
    </header>
  )
}

function BrandSwitcher() {
  return (
    <Button variant="ghost" size="sm" className="-ml-1 gap-2 pl-1">
      <Avatar size="sm">
        <AvatarFallback className="bg-foreground text-background">H</AvatarFallback>
      </Avatar>
      <span className="font-display text-sm font-medium tracking-tight">Hivy</span>
    </Button>
  )
}

function OrgSwitcher() {
  return (
    <Button variant="ghost" size="sm" className="-ml-1 gap-2 pl-1">
      <Avatar size="sm">
        <AvatarFallback className="bg-foreground text-background">N</AvatarFallback>
      </Avatar>
      <span className="text-sm font-medium">Northwind</span>
      <Badge variant="secondary" className="text-[10px] uppercase tracking-wider">
        Scale
      </Badge>
      <HugeiconsIcon
        icon={ArrowDown01Icon}
        className="size-3.5 text-muted-foreground"
        strokeWidth={1.75}
      />
    </Button>
  )
}

function TabButton({
  children,
  active,
}: {
  children: React.ReactNode
  active?: boolean
}) {
  return (
    <button
      type="button"
      className={cn(
        "relative px-3 py-2 text-sm font-medium transition-colors",
        active
          ? "text-foreground after:absolute after:right-3 after:bottom-0 after:left-3 after:h-px after:bg-foreground after:content-['']"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  )
}

function TopBarSearch() {
  const [value, setValue] = useState("")
  return (
    <label className="flex h-9 w-64 items-center gap-2 rounded-4xl bg-muted/60 px-3 text-sm text-foreground transition-colors hover:bg-muted">
      <HugeiconsIcon
        icon={Search01Icon}
        className="size-4 text-muted-foreground"
        strokeWidth={2}
      />
      <input
        value={value}
        onChange={(event) => setValue(event.target.value)}
        placeholder="Search"
        className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
      />
      <kbd className="flex h-5 items-center gap-0.5 rounded-md bg-background/60 px-1.5 font-mono text-xs font-medium text-muted-foreground">
        <HugeiconsIcon icon={CommandIcon} className="size-3" strokeWidth={2} />
        <span>K</span>
      </kbd>
    </label>
  )
}

function PageHeader() {
  return (
    <section className="flex items-end justify-between gap-12">
      <div className="flex flex-col gap-3">
        <p className="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          Northwind Workforce
        </p>
        <h1 className="font-display text-4xl font-medium tracking-tight">
          All employees
        </h1>
        <p className="text-sm text-muted-foreground">
          <span className="font-semibold text-foreground">10</span> employees,{" "}
          <span className="font-semibold text-foreground">7</span> active,{" "}
          <span className="font-semibold text-foreground">848</span> tasks completed in the
          last 7 days.{" "}
          <a
            href="#"
            className="text-destructive underline decoration-destructive/40 underline-offset-4 transition-colors hover:decoration-destructive"
          >
            1 need attention.
          </a>
        </p>
      </div>
      <div className="flex items-center gap-3">
        <FilterField />
        <Button>
          <HugeiconsIcon icon={Add01Icon} strokeWidth={2.25} data-icon="inline-start" />
          New employee
        </Button>
      </div>
    </section>
  )
}

function FilterField() {
  const [value, setValue] = useState("")
  return (
    <div className="relative w-60">
      <HugeiconsIcon
        icon={Search01Icon}
        className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        strokeWidth={2}
      />
      <Input
        value={value}
        onChange={(event) => setValue(event.target.value)}
        placeholder="Filter employees"
        className="pl-9"
      />
    </div>
  )
}

function TeamSection({ team }: { team: Team }) {
  return (
    <section className="flex flex-col gap-4">
      <header className="flex items-baseline justify-between gap-6">
        <div className="flex items-baseline gap-3">
          <h2 className="text-base font-semibold tracking-tight">{team.name}</h2>
          <p className="text-xs text-muted-foreground">
            {team.employees.length} {team.employees.length === 1 ? "employee" : "employees"} ·{" "}
            {team.weeklyTasks} tasks this week
          </p>
        </div>
        <a
          href="#"
          className="flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Open team
          <HugeiconsIcon icon={ArrowRight01Icon} className="size-3.5" strokeWidth={2} />
        </a>
      </header>

      <EmployeeTable employees={team.employees} />
    </section>
  )
}

function EmployeeTable({ employees }: { employees: Employee[] }) {
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-card">
      <div
        className={cn(
          "grid items-center border-b border-border px-5 py-3 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground",
          COLUMN_GRID,
        )}
      >
        <span>Employee</span>
        <span>Role</span>
        <span>Model</span>
        <span>Status</span>
        <span>Tasks / 7d</span>
        <span>Success</span>
        <span className="text-right">Last active</span>
      </div>
      <ul className="divide-y divide-border">
        {employees.map((employee) => (
          <EmployeeRow key={employee.name} employee={employee} />
        ))}
      </ul>
    </div>
  )
}

function EmployeeRow({ employee }: { employee: Employee }) {
  return (
    <li
      className={cn(
        "grid items-center px-5 py-3.5 text-sm transition-colors hover:bg-muted/40",
        COLUMN_GRID,
      )}
    >
      <div className="flex min-w-0 items-center gap-3">
        <Avatar size="sm">
          <AvatarFallback>{employee.initial}</AvatarFallback>
        </Avatar>
        <span className="truncate font-medium">{employee.name}</span>
      </div>
      <span className="truncate text-foreground/90">{employee.role}</span>
      <span className="truncate text-foreground/90">{employee.model}</span>
      <span>
        <StatusBadge status={employee.status} />
      </span>
      <div className="flex items-center gap-3">
        <Sparkline points={employee.spark} />
        <span className="font-medium tabular-nums">{employee.tasks7d ?? "—"}</span>
      </div>
      <span className="font-medium tabular-nums">
        {employee.successPct == null ? "—" : `${employee.successPct}%`}
      </span>
      <span className="text-right text-muted-foreground">{employee.lastActive}</span>
    </li>
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

function Sparkline({ points }: { points: number[] }) {
  const w = 80
  const h = 22
  if (points.length === 0) {
    return <span className="block h-[22px] w-20" aria-hidden />
  }
  const max = Math.max(...points, 1)
  const min = Math.min(...points)
  const range = Math.max(max - min, 1)
  const stepX = points.length > 1 ? w / (points.length - 1) : w
  const path = points
    .map((p, i) => {
      const x = i * stepX
      const y = h - ((p - min) / range) * (h - 2) - 1
      return `${i === 0 ? "M" : "L"}${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(" ")
  return (
    <svg
      width={w}
      height={h}
      viewBox={`0 0 ${w} ${h}`}
      className="shrink-0 text-primary"
      aria-hidden
    >
      <path
        d={path}
        fill="none"
        stroke="currentColor"
        strokeWidth={1.4}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function WorkspaceFloater() {
  return (
    <div className="fixed bottom-6 left-6 z-30">
      <Button
        variant="default"
        size="icon-sm"
        aria-label="Switch workspace"
        className="rounded-full bg-foreground text-background hover:bg-foreground/90"
      >
        N
      </Button>
    </div>
  )
}
