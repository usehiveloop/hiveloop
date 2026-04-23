"use client"

import { useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Tick02Icon,
  Cancel01Icon,
  AlertCircleIcon,
} from "@hugeicons/core-free-icons"
import { $api } from "@/lib/api/hooks"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "@/components/ui/tooltip"
import type { components } from "@/lib/api/schema"

type BuiltInTool = components["schemas"]["BuiltInToolDefinition"]
type PermissionLevel = "allow" | "deny" | "require_approval"

interface ToolPermissionsSectionProps {
  permissions: Record<string, PermissionLevel>
  onChange: (permissions: Record<string, PermissionLevel>) => void
}

const categoryLabels: Record<string, string> = {
  filesystem: "Filesystem",
  shell: "Shell",
  web: "Web",
  orchestration: "Orchestration",
  tasks: "Tasks",
  journal: "Journal",
  scheduling: "Scheduling",
  code_intelligence: "Code intelligence",
  memory: "Memory",
  codedb: "CodeDB",
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

interface ToolCardProps {
  tool: BuiltInTool
  permission: PermissionLevel
  onToggle: () => void
}

const permissionLabels: Record<PermissionLevel, string> = {
  allow: "Allowed",
  deny: "Denied",
  require_approval: "Requires approval",
}

function ToolCard({ tool, permission, onToggle }: ToolCardProps) {
  const locked = tool.locked ?? false

  const stateStyles = {
    allow: "border-emerald-500/30 bg-emerald-500/8 text-emerald-600 dark:text-emerald-400",
    deny: "border-border bg-muted/50 text-muted-foreground",
    require_approval: "border-amber-500/30 bg-amber-500/8 text-amber-600 dark:text-amber-400",
  }

  const stateIcons = {
    allow: Tick02Icon,
    deny: Cancel01Icon,
    require_approval: AlertCircleIcon,
  }

  const tooltipText = locked
    ? `${tool.name} — always allowed`
    : `${tool.name} — ${permissionLabels[permission]}`

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              onClick={locked ? undefined : onToggle}
              disabled={locked}
              className={`flex items-center gap-1.5 rounded-lg border px-2 py-1.5 text-left transition-colors ${
                stateStyles[permission]
              } ${locked ? "opacity-60 cursor-not-allowed" : "cursor-pointer hover:opacity-80"}`}
            />
          }
        >
          <HugeiconsIcon icon={stateIcons[permission]} size={12} className="shrink-0" />
          <span className="text-xs font-medium truncate">{tool.name}</span>
        </TooltipTrigger>
        <TooltipContent side="top">{tooltipText}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function ToolPermissionsSection({ permissions, onChange }: ToolPermissionsSectionProps) {
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

  function handleToggle(toolId: string, currentPermission: PermissionLevel) {
    const next = { ...permissions }
    const newPermission = nextPermission(currentPermission)
    if (newPermission === "allow") {
      delete next[toolId]
    } else {
      next[toolId] = newPermission
    }
    onChange(next)
  }

  function getPermission(toolId: string): PermissionLevel {
    return permissions[toolId] ?? "allow"
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <Skeleton key={index} className="h-10 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {categoryOrder
        .filter((category) => grouped.has(category))
        .map((category) => {
          const categoryTools = grouped.get(category)!
          return (
            <div key={category} className="flex flex-col gap-1.5">
              <p className="text-mini font-medium uppercase tracking-wider text-muted-foreground">
                {categoryLabels[category] ?? category}
              </p>
              <div className="flex flex-wrap gap-1.5">
                {categoryTools.map((tool) => (
                  <ToolCard
                    key={tool.id}
                    tool={tool}
                    permission={getPermission(tool.id!)}
                    onToggle={() => handleToggle(tool.id!, getPermission(tool.id!))}
                  />
                ))}
              </div>
            </div>
          )
        })}
    </div>
  )
}
