"use client"

import { useEffect, useMemo, useRef } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  LockIcon,
  FolderLibraryIcon,
  TerminalIcon,
  FlowIcon,
  TaskDone01Icon,
  Note01Icon,
  Calendar01Icon,
  CodeSquareIcon,
  Brain01Icon,
  Database01Icon,
  CubeIcon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { Skeleton } from "@/components/ui/skeleton"
import type { components } from "@/lib/api/schema"

type BuiltInTool = components["schemas"]["BuiltInToolDefinition"]
type PermissionLevel = "allow" | "deny" | "require_approval"

// Tools allowed by default for a brand-new agent. Everything else starts denied.
// IDs match the backend catalogue in internal/handler/agents_tools.go.
const DEFAULT_ALLOWED_TOOLS = new Set<string>([
  "skill",
  "bash",
  "agent",
  "sub_agent",
  "Read",
  "write",
  "apply_patch",
  "RipGrep",
  "web_search",
  "web_fetch",
  "todoread",
  "todowrite",
])

interface ToolPermissionsSectionProps {
  permissions: Record<string, PermissionLevel>
  onChange: (permissions: Record<string, PermissionLevel>) => void
  /**
   * "create" seeds every non-locked tool with "deny" once tools load, so the
   * default for a brand-new agent is no permissions granted. "edit" leaves
   * existing agents untouched.
   */
  mode: "create" | "edit"
}

const PERMISSION_OPTIONS: Array<{
  value: PermissionLevel
  label: string
  activeClass: string
}> = [
  {
    value: "allow",
    label: "Allow",
    activeClass:
      "bg-emerald-500/12 text-emerald-700 dark:text-emerald-400 ring-1 ring-emerald-500/30",
  },
  {
    value: "require_approval",
    label: "Approval",
    activeClass:
      "bg-amber-500/12 text-amber-700 dark:text-amber-400 ring-1 ring-amber-500/30",
  },
  {
    value: "deny",
    label: "Deny",
    activeClass: "bg-foreground/8 text-foreground ring-1 ring-foreground/15",
  },
]

const CATEGORIES: Record<
  string,
  { label: string; description: string }
> = {
  filesystem: {
    label: "Filesystem",
    description: "Read, write, and inspect files in the workspace.",
  },
  shell: {
    label: "Shell",
    description: "Run commands inside the agent's sandbox.",
  },
  web: {
    label: "Web",
    description: "Browse, fetch, and search the internet.",
  },
  orchestration: {
    label: "Orchestration",
    description: "Spawn sub-agents and coordinate multi-step work.",
  },
  tasks: {
    label: "Tasks",
    description: "Track plans, todos, and progress mid-run.",
  },
  journal: {
    label: "Journal",
    description: "Log decisions and notes the agent can revisit.",
  },
  scheduling: {
    label: "Scheduling",
    description: "Schedule future runs and recurring jobs.",
  },
  code_intelligence: {
    label: "Code intelligence",
    description: "Navigate code structurally — symbols, types, references.",
  },
  memory: {
    label: "Memory",
    description: "Persist long-term knowledge across runs.",
  },
  codedb: {
    label: "CodeDB",
    description: "Query the indexed code knowledge graph.",
  },
}

const categoryOrder = [
  "filesystem",
  "shell",
  "web",
  "orchestration",
  "tasks",
  "journal",
  "scheduling",
  "code_intelligence",
  "memory",
  "codedb",
]

function nextPermission(current: PermissionLevel): PermissionLevel {
  if (current === "allow") return "require_approval"
  if (current === "require_approval") return "deny"
  return "allow"
}

function ChromeLogo({ className }: { className?: string }) {
  return (
    <svg
      preserveAspectRatio="xMidYMid"
      viewBox="0 0 190.5 190.5"
      className={className}
      aria-hidden
    >
      <path
        fill="#fff"
        d="M95.252 142.873c26.304 0 47.627-21.324 47.627-47.628s-21.323-47.628-47.627-47.628-47.627 21.324-47.627 47.628 21.323 47.628 47.627 47.628z"
      />
      <path
        fill="#229342"
        d="m54.005 119.07-41.24-71.43a95.227 95.227 0 0 0-.003 95.25 95.234 95.234 0 0 0 82.496 47.61l41.24-71.43v-.011a47.613 47.613 0 0 1-17.428 17.443 47.62 47.62 0 0 1-47.632.007 47.62 47.62 0 0 1-17.433-17.437z"
      />
      <path
        fill="#fbc116"
        d="m136.495 119.067-41.239 71.43a95.229 95.229 0 0 0 82.489-47.622A95.24 95.24 0 0 0 190.5 95.248a95.237 95.237 0 0 0-12.772-47.623H95.249l-.01.007a47.62 47.62 0 0 1 23.819 6.372 47.618 47.618 0 0 1 17.439 17.431 47.62 47.62 0 0 1-.001 47.633z"
      />
      <path
        fill="#1a73e8"
        d="M95.252 132.961c20.824 0 37.705-16.881 37.705-37.706S116.076 57.55 95.252 57.55 57.547 74.431 57.547 95.255s16.881 37.706 37.705 37.706z"
      />
      <path
        fill="#e33b2e"
        d="M95.252 47.628h82.479A95.237 95.237 0 0 0 142.87 12.76 95.23 95.23 0 0 0 95.245 0a95.222 95.222 0 0 0-47.623 12.767 95.23 95.23 0 0 0-34.856 34.872l41.24 71.43.011.006a47.62 47.62 0 0 1-.015-47.633 47.61 47.61 0 0 1 41.252-23.815z"
      />
    </svg>
  )
}

function CategoryLogo({ category }: { category: string }) {
  if (category === "web") {
    return (
      <div className="flex size-9 items-center justify-center rounded-lg bg-background ring-1 ring-border">
        <ChromeLogo className="size-5" />
      </div>
    )
  }

  const iconMap: Record<string, typeof FolderLibraryIcon> = {
    filesystem: FolderLibraryIcon,
    shell: TerminalIcon,
    orchestration: FlowIcon,
    tasks: TaskDone01Icon,
    journal: Note01Icon,
    scheduling: Calendar01Icon,
    code_intelligence: CodeSquareIcon,
    memory: Brain01Icon,
    codedb: Database01Icon,
  }

  const tints: Record<string, string> = {
    filesystem: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
    shell: "bg-zinc-500/12 text-zinc-700 dark:text-zinc-300",
    orchestration: "bg-violet-500/10 text-violet-600 dark:text-violet-400",
    tasks: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
    journal: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
    scheduling: "bg-rose-500/10 text-rose-600 dark:text-rose-400",
    code_intelligence: "bg-indigo-500/10 text-indigo-600 dark:text-indigo-400",
    memory: "bg-fuchsia-500/10 text-fuchsia-600 dark:text-fuchsia-400",
    codedb: "bg-teal-500/10 text-teal-600 dark:text-teal-400",
  }

  const icon = iconMap[category] ?? CubeIcon
  const tint = tints[category] ?? "bg-muted text-muted-foreground"

  return (
    <div
      className={`flex size-9 items-center justify-center rounded-lg ${tint}`}
    >
      <HugeiconsIcon icon={icon} size={18} strokeWidth={2} />
    </div>
  )
}

interface PermissionSelectorProps {
  value: PermissionLevel
  onChange: (next: PermissionLevel) => void
  locked: boolean
}

function PermissionSelector({
  value,
  onChange,
  locked,
}: PermissionSelectorProps) {
  if (locked) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-[11px] font-medium text-muted-foreground">
        <HugeiconsIcon icon={LockIcon} size={11} />
        Always allowed
      </span>
    )
  }

  return (
    <div className="inline-flex shrink-0 items-center gap-0.5 rounded-lg bg-muted/60 p-0.5">
      {PERMISSION_OPTIONS.map((option) => {
        const active = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            className={
              "rounded-md px-2.5 py-1 text-[11px] font-medium transition-colors " +
              (active
                ? option.activeClass
                : "text-muted-foreground hover:text-foreground")
            }
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

export function ToolPermissionsSection({
  permissions,
  onChange,
  mode,
}: ToolPermissionsSectionProps) {
  const { data, isLoading } = $api.useQuery("get", "/v1/agents/built-in-tools")
  const tools: BuiltInTool[] = (data as BuiltInTool[] | undefined) ?? []

  const grouped = useMemo(() => {
    const groups = new Map<string, BuiltInTool[]>()
    for (const tool of tools) {
      const category = tool.category ?? "other"
      const list = groups.get(category) ?? []
      list.push(tool)
      groups.set(category, list)
    }
    return groups
  }, [tools])

  const seededRef = useRef(false)
  useEffect(() => {
    if (seededRef.current) return
    if (mode !== "create") return
    if (tools.length === 0) return
    if (Object.keys(permissions).length > 0) return
    const seeded: Record<string, PermissionLevel> = {}
    for (const tool of tools) {
      if (!tool.id) continue
      if (tool.locked) continue
      seeded[tool.id] = DEFAULT_ALLOWED_TOOLS.has(tool.id) ? "allow" : "deny"
    }
    seededRef.current = true
    onChange(seeded)
  }, [mode, tools, permissions, onChange])

  function setPermission(toolId: string, next: PermissionLevel) {
    onChange({ ...permissions, [toolId]: next })
  }

  function getPermission(toolId: string): PermissionLevel {
    if (permissions[toolId]) return permissions[toolId]
    if (mode === "create") {
      return DEFAULT_ALLOWED_TOOLS.has(toolId) ? "allow" : "deny"
    }
    return "allow"
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <Skeleton key={index} className="h-32 w-full rounded-2xl" />
        ))}
      </div>
    )
  }

  const visibleCategories = categoryOrder.filter((c) => grouped.has(c))
  const remainingCategories = Array.from(grouped.keys()).filter(
    (c) => !categoryOrder.includes(c),
  )
  const allCategories = [...visibleCategories, ...remainingCategories]

  return (
    <div className="flex flex-col gap-4">
      {allCategories.map((category) => {
        const categoryTools = grouped.get(category)!
        const meta = CATEGORIES[category] ?? {
          label: category,
          description: "",
        }
        return (
          <div
            key={category}
            className="overflow-hidden rounded-2xl border border-border bg-card"
          >
            <div className="flex items-start gap-3 border-b border-border bg-muted/30 px-4 py-3">
              <CategoryLogo category={category} />
              <div className="min-w-0 flex-1 pt-0.5">
                <h3 className="text-[14px] font-medium text-foreground">
                  {meta.label}
                </h3>
                {meta.description ? (
                  <p className="mt-0.5 text-[12px] text-muted-foreground">
                    {meta.description}
                  </p>
                ) : null}
              </div>
            </div>
            <ul className="divide-y divide-border/60">
              {categoryTools.map((tool) => {
                const id = tool.id ?? ""
                const permission = getPermission(id)
                const locked = tool.locked ?? false
                return (
                  <li
                    key={id}
                    onClick={
                      locked
                        ? undefined
                        : () =>
                            setPermission(id, nextPermission(permission))
                    }
                    className={
                      "flex items-center gap-4 px-4 py-2.5 transition-colors " +
                      (locked
                        ? "cursor-not-allowed"
                        : "cursor-pointer hover:bg-muted/40")
                    }
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-[13px] font-medium text-foreground">
                        {tool.name}
                      </p>
                      {tool.description ? (
                        <p className="mt-0.5 line-clamp-1 text-[12px] text-muted-foreground">
                          {tool.description}
                        </p>
                      ) : null}
                    </div>
                    <div onClick={(e) => e.stopPropagation()}>
                      <PermissionSelector
                        value={permission}
                        locked={locked}
                        onChange={(next) => setPermission(id, next)}
                      />
                    </div>
                  </li>
                )
              })}
            </ul>
          </div>
        )
      })}
    </div>
  )
}
