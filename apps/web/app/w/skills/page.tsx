"use client"

import { useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  BookOpen01Icon,
  CheckmarkCircle02Icon,
  CodeIcon,
  CommandIcon,
  File01Icon,
  Folder01Icon,
  Loading03Icon,
  SearchIcon,
  TextIcon,
} from "@hugeicons/core-free-icons"
import { CreateSkillDialog } from "@/app/w-old/settings/_components/create-skill-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Skill = components["schemas"]["skillResponse"]
type SkillDetail = components["schemas"]["skillDetailResponse"]
type AttachedSkill = components["schemas"]["employeeSkillResponse"]

type SkillFile = {
  path: string
  body: string
}

const allCategories = "All categories"

export default function SkillsPage() {
  const queryClient = useQueryClient()
  const [query, setQuery] = useState("")
  const [category, setCategory] = useState(allCategories)
  const [creating, setCreating] = useState(false)
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null)
  const [pendingSkillId, setPendingSkillId] = useState<string | null>(null)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const skillsQuery = $api.useQuery("get", "/v1/skills", {
    params: { query: { scope: "all", limit: 100 } },
  })

  const employeeID = employeesQuery.data?.data?.[0]?.id ?? ""
  const attachedQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}/skills",
    { params: { path: { id: employeeID } } },
    { enabled: Boolean(employeeID) }
  )

  const attachSkill = $api.useMutation("post", "/v1/employees/{id}/skills")
  const detachSkill = $api.useMutation(
    "delete",
    "/v1/employees/{id}/skills/{skillID}"
  )

  const skills = (skillsQuery.data?.data ?? []).filter((skill) => !skill.hidden)
  const attached = attachedQuery.data ?? []

  const attachedBySkillID = useMemo(() => {
    const map = new Map<string, AttachedSkill>()
    for (const row of attached) {
      if (row.skill_id) map.set(row.skill_id, row)
    }
    return map
  }, [attached])

  const categories = useMemo(() => {
    const values = new Set<string>()
    for (const skill of skills) {
      const value = skill.category?.trim()
      if (value) values.add(value)
    }
    return [allCategories, ...Array.from(values).sort()]
  }, [skills])

  const filteredSkills = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()

    return skills.filter((skill) => {
      const tags = skill.tags ?? []
      const matchesCategory =
        category === allCategories || skill.category === category
      const matchesQuery =
        normalizedQuery.length === 0 ||
        [
          skill.name,
          skill.description,
          skill.category,
          skill.slug,
          skill.source_type,
          ...tags,
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase()
          .includes(normalizedQuery)

      return matchesCategory && matchesQuery
    })
  }, [category, query, skills])

  function refreshSkills() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/employees/{id}/skills"],
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
  }

  function handleAttach(skill: Skill) {
    if (!employeeID || !skill.id) {
      toast.error("No Hivy employee is available for this workspace")
      return
    }
    setPendingSkillId(skill.id)
    attachSkill.mutate(
      {
        params: { path: { id: employeeID } },
        body: { skill_id: skill.id } as never,
      },
      {
        onSuccess: () => {
          toast.success("Skill installed for Hivy")
          refreshSkills()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to install skill"))
        },
        onSettled: () => setPendingSkillId(null),
      }
    )
  }

  function handleDetach(skill: Skill) {
    if (!employeeID || !skill.id) return
    setPendingSkillId(skill.id)
    detachSkill.mutate(
      { params: { path: { id: employeeID, skillID: skill.id } } },
      {
        onSuccess: () => {
          toast.success("Skill removed from Hivy")
          refreshSkills()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to remove skill"))
        },
        onSettled: () => setPendingSkillId(null),
      }
    )
  }

  const loading =
    employeesQuery.isLoading || skillsQuery.isLoading || attachedQuery.isLoading

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-7">
      <div className="flex flex-col gap-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="max-w-2xl">
            <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
              Skills
            </h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Install focused capabilities that help Hivy research, write,
              review, and operate across your workspace.
            </p>
          </div>

          <Button
            type="button"
            className="w-full sm:w-auto"
            onClick={() => setCreating(true)}
          >
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            Create skill
          </Button>
        </div>

        <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
          <div className="relative min-w-0 flex-1">
            <HugeiconsIcon
              icon={SearchIcon}
              size={16}
              className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search skills"
              className="h-11 rounded-md bg-card pl-9"
            />
          </div>

          <Select
            value={category}
            onValueChange={(value) => {
              if (value) setCategory(value)
            }}
          >
            <SelectTrigger className="h-11 w-full rounded-md border-border bg-card px-3 sm:w-56">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              {categories.map((item) => (
                <SelectItem key={item} value={item}>
                  {item}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {loading ? (
        <SkillSkeletons />
      ) : filteredSkills.length === 0 ? (
        <EmptyState hasSkills={skills.length > 0} />
      ) : (
        <div className="flex flex-col gap-4">
          {filteredSkills.map((skill) => {
            const attachment = skill.id
              ? attachedBySkillID.get(skill.id)
              : undefined
            const installed = Boolean(attachment)
            const locked = Boolean(attachment?.locked || attachment?.required)
            const pending = pendingSkillId === skill.id

            return (
              <SkillRow
                key={skill.id}
                skill={skill}
                installed={installed}
                locked={locked}
                pending={pending}
                onOpen={() => setSelectedSkill(skill)}
                onAttach={() => handleAttach(skill)}
                onDetach={() => handleDetach(skill)}
              />
            )
          })}
        </div>
      )}

      <CreateSkillDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={refreshSkills}
      />
      <SkillDialog
        skill={selectedSkill}
        attachment={
          selectedSkill?.id ? attachedBySkillID.get(selectedSkill.id) : undefined
        }
        pending={selectedSkill?.id ? pendingSkillId === selectedSkill.id : false}
        onOpenChange={(open) => {
          if (!open) setSelectedSkill(null)
        }}
        onAttach={() => {
          if (selectedSkill) handleAttach(selectedSkill)
        }}
        onDetach={() => {
          if (selectedSkill) handleDetach(selectedSkill)
        }}
      />
    </div>
  )
}

function SkillRow({
  skill,
  installed,
  locked,
  pending,
  onOpen,
  onAttach,
  onDetach,
}: {
  skill: Skill
  installed: boolean
  locked: boolean
  pending: boolean
  onOpen: () => void
  onAttach: () => void
  onDetach: () => void
}) {
  const tags = skill.tags ?? []

  return (
    <article
      role="button"
      tabIndex={0}
      className="flex cursor-pointer gap-4 rounded-md border border-border bg-card p-5 text-left transition-colors hover:border-muted-foreground/25 hover:bg-muted/20 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 focus-visible:outline-none"
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          onOpen()
        }
      }}
    >
      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
        <HugeiconsIcon icon={CommandIcon} size={20} />
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
          <h2 className="text-base font-semibold text-foreground">
            {skill.name}
          </h2>
          {skill.category ? (
            <Badge variant="outline" className="w-fit">
              {skill.category}
            </Badge>
          ) : null}
        </div>

        <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
          {skill.description ?? "No description"}
        </p>

        <div className="mt-4 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="flex min-w-0 flex-wrap gap-2">
            {tags.length > 0 ? (
              tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded-full border border-border bg-background px-2.5 py-1 text-xs font-medium text-muted-foreground"
                >
                  #{tag}
                </span>
              ))
            ) : (
              <span className="text-xs text-muted-foreground">
                {skill.slug ?? skill.source_type ?? "workspace"}
              </span>
            )}
          </div>

          {installed ? (
            <Button
              type="button"
              variant="outline"
              className={cn("w-full shrink-0 sm:w-32", locked && "text-primary")}
              loading={pending}
              disabled={locked}
              onClick={(event) => {
                event.stopPropagation()
                if (!locked) onDetach()
              }}
            >
              {!pending && locked ? (
                <HugeiconsIcon
                  icon={CheckmarkCircle02Icon}
                  size={16}
                  data-icon="inline-start"
                />
              ) : null}
              {locked ? "Required" : "Installed"}
            </Button>
          ) : (
            <Button
              type="button"
              variant="secondary"
              className="w-full shrink-0 sm:w-32"
              loading={pending}
              onClick={(event) => {
                event.stopPropagation()
                onAttach()
              }}
            >
              Install
            </Button>
          )}
        </div>
      </div>
    </article>
  )
}

function SkillDialog({
  skill,
  attachment,
  pending,
  onOpenChange,
  onAttach,
  onDetach,
}: {
  skill: Skill | null
  attachment?: AttachedSkill
  pending: boolean
  onOpenChange: (open: boolean) => void
  onAttach: () => void
  onDetach: () => void
}) {
  const [selectedPath, setSelectedPath] = useState("SKILL.md")
  const [viewMode, setViewMode] = useState<"preview" | "source">("preview")
  const installed = Boolean(attachment)
  const locked = Boolean(attachment?.locked || attachment?.required)

  const detailQuery = $api.useQuery(
    "get",
    "/v1/skills/{id}",
    { params: { path: { id: skill?.id ?? "" } } },
    { enabled: Boolean(skill?.id) }
  )

  const detail = detailQuery.data as SkillDetail | undefined
  const files = useMemo(() => buildSkillFiles(detail), [detail])
  const selectedFile =
    files.find((file) => file.path === selectedPath) ?? files[0]
  const isMarkdown = selectedFile ? isMarkdownFile(selectedFile.path) : false

  useEffect(() => {
    setSelectedPath("SKILL.md")
    setViewMode("preview")
  }, [skill?.id])

  return (
    <Dialog open={Boolean(skill)} onOpenChange={onOpenChange}>
      <DialogContent className="h-[88dvh] max-h-[88dvh] overflow-hidden rounded-md p-0 sm:max-w-5xl">
        {skill ? (
          <div className="flex h-full min-h-0 flex-col">
            <DialogHeader className="border-b border-border px-5 py-4 pr-14">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <DialogTitle className="truncate text-xl">
                  {skill.name}
                </DialogTitle>
                {skill.category ? (
                  <Badge variant="outline">{skill.category}</Badge>
                ) : null}
              </div>
              <DialogDescription className="line-clamp-2">
                {skill.description ?? "No description"}
              </DialogDescription>
            </DialogHeader>

            {detailQuery.isLoading ? (
              <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[220px_minmax(0,1fr)]">
                <div className="hidden border-r border-border p-3 md:block">
                  <Skeleton className="h-8 w-full" />
                  <Skeleton className="mt-2 h-8 w-4/5" />
                  <Skeleton className="mt-2 h-8 w-3/5" />
                </div>
                <div className="space-y-3 p-5">
                  <Skeleton className="h-8 w-40" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-4/5" />
                  <Skeleton className="h-48 w-full" />
                </div>
              </div>
            ) : files.length === 0 ? (
              <div className="flex min-h-0 flex-1 items-center justify-center p-8 text-center">
                <div>
                  <HugeiconsIcon
                    icon={BookOpen01Icon}
                    className="mx-auto size-8 text-muted-foreground"
                  />
                  <p className="mt-4 text-sm font-medium text-foreground">
                    No hydrated content
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    This skill does not have files available yet.
                  </p>
                </div>
              </div>
            ) : (
              <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[220px_minmax(0,1fr)]">
                <div className="min-h-36 overflow-y-auto border-b border-border p-2 md:border-r md:border-b-0">
                  <div className="mb-2 px-2 py-1 text-xs font-medium text-muted-foreground">
                    Files
                  </div>
                  <div className="flex gap-1 overflow-x-auto pb-1 md:block md:overflow-x-visible md:pb-0">
                    {files.map((file) => (
                      <FileButton
                        key={file.path}
                        file={file}
                        selected={file.path === selectedFile?.path}
                        onSelect={() => {
                          setSelectedPath(file.path)
                          setViewMode(isMarkdownFile(file.path) ? "preview" : "source")
                        }}
                      />
                    ))}
                  </div>
                </div>

                <div className="flex min-w-0 flex-col overflow-hidden">
                  <div className="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-border px-4">
                    <p className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                      {selectedFile?.path}
                    </p>
                    {selectedFile && isMarkdown ? (
                      <div className="flex shrink-0 items-center gap-1 rounded-full bg-muted p-0.5">
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          className={cn(
                            "rounded-full px-2.5",
                            viewMode === "preview" && "bg-background text-foreground shadow-sm"
                          )}
                          onClick={() => setViewMode("preview")}
                        >
                          <HugeiconsIcon icon={TextIcon} size={12} />
                          Preview
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          className={cn(
                            "rounded-full px-2.5",
                            viewMode === "source" && "bg-background text-foreground shadow-sm"
                          )}
                          onClick={() => setViewMode("source")}
                        >
                          <HugeiconsIcon icon={CodeIcon} size={12} />
                          Source
                        </Button>
                      </div>
                    ) : null}
                  </div>

                  <div className="min-h-0 flex-1 overflow-auto p-5">
                    {selectedFile && isMarkdown && viewMode === "preview" ? (
                      <MarkdownContent content={selectedFile.body} />
                    ) : selectedFile ? (
                      <RawContent content={selectedFile.body} />
                    ) : null}
                  </div>
                </div>
              </div>
            )}

            <DialogFooter className="border-t border-border px-5 py-4">
              {installed ? (
                <Button
                  type="button"
                  variant="outline"
                  className={cn("w-full sm:w-auto", locked && "text-primary")}
                  loading={pending}
                  disabled={locked}
                  onClick={onDetach}
                >
                  {!pending && locked ? (
                    <HugeiconsIcon
                      icon={CheckmarkCircle02Icon}
                      size={16}
                      data-icon="inline-start"
                    />
                  ) : null}
                  {locked ? "Required by connection" : "Installed"}
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="secondary"
                  className="w-full sm:w-auto"
                  loading={pending}
                  onClick={onAttach}
                >
                  Install
                </Button>
              )}
            </DialogFooter>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function FileButton({
  file,
  selected,
  onSelect,
}: {
  file: SkillFile
  selected: boolean
  onSelect: () => void
}) {
  const parts = file.path.split("/")
  const name = parts.at(-1) ?? file.path
  const inFolder = parts.length > 1

  return (
    <button
      type="button"
      className={cn(
        "flex h-9 w-44 shrink-0 items-center gap-2 rounded-md px-2 text-left text-xs transition-colors md:w-full",
        selected
          ? "bg-primary/10 text-foreground"
          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
      )}
      onClick={onSelect}
    >
      <HugeiconsIcon
        icon={inFolder ? Folder01Icon : File01Icon}
        size={14}
        className="shrink-0"
      />
      <span className="truncate">{name}</span>
    </button>
  )
}

function SkillSkeletons() {
  return (
    <div className="flex flex-col gap-4">
      {Array.from({ length: 5 }).map((_, index) => (
        <div key={index} className="flex gap-4 rounded-md border border-border bg-card p-5">
          <Skeleton className="h-10 w-10 shrink-0 rounded-md" />
          <div className="min-w-0 flex-1">
            <Skeleton className="h-5 w-48" />
            <Skeleton className="mt-3 h-4 w-full max-w-2xl" />
            <Skeleton className="mt-2 h-4 w-2/3" />
            <div className="mt-4 flex gap-2">
              <Skeleton className="h-7 w-20 rounded-full" />
              <Skeleton className="h-7 w-24 rounded-full" />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function EmptyState({ hasSkills }: { hasSkills: boolean }) {
  return (
    <div className="flex min-h-72 flex-col items-center justify-center rounded-md border border-border bg-card px-6 text-center">
      <HugeiconsIcon
        icon={BookOpen01Icon}
        className="size-8 text-muted-foreground"
      />
      <p className="mt-4 text-sm font-medium text-foreground">
        {hasSkills ? "No matching skills" : "No skills available"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {hasSkills
          ? "Try a different search or category."
          : "Create a custom skill for this workspace."}
      </p>
    </div>
  )
}

function buildSkillFiles(detail?: SkillDetail): SkillFile[] {
  if (!detail?.bundle) return []
  const byPath = new Map<string, SkillFile>()

  if (detail.bundle.content) {
    byPath.set("SKILL.md", { path: "SKILL.md", body: detail.bundle.content })
  }

  for (const reference of detail.bundle.references ?? []) {
    if (reference.path && reference.body !== undefined) {
      byPath.set(reference.path, {
        path: reference.path,
        body: reference.body ?? "",
      })
    }
  }

  for (const [path, body] of Object.entries(detail.bundle.files ?? {})) {
    if (path) {
      byPath.set(path, { path, body })
    }
  }

  return Array.from(byPath.values()).sort((first, second) => {
    if (first.path === "SKILL.md") return -1
    if (second.path === "SKILL.md") return 1
    return first.path.localeCompare(second.path)
  })
}

function isMarkdownFile(path: string): boolean {
  return path.endsWith(".md") || path.endsWith(".mdx")
}

function RawContent({ content }: { content: string }) {
  return (
    <pre className="min-h-full overflow-auto rounded-md bg-muted p-4 font-mono text-xs leading-5 text-foreground">
      <code>{content}</code>
    </pre>
  )
}

function MarkdownContent({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: ({ children }) => (
          <h1 className="mt-8 first:mt-0 font-heading text-2xl font-medium tracking-[-0.01em] text-foreground">
            {children}
          </h1>
        ),
        h2: ({ children }) => (
          <h2 className="mt-6 first:mt-0 font-heading text-lg font-medium text-foreground">
            {children}
          </h2>
        ),
        h3: ({ children }) => (
          <h3 className="mt-5 text-base font-medium text-foreground">
            {children}
          </h3>
        ),
        p: ({ children }) => (
          <p className="mt-3 max-w-3xl text-sm leading-6 text-muted-foreground">
            {children}
          </p>
        ),
        ul: ({ children }) => (
          <ul className="mt-3 max-w-3xl list-disc space-y-1.5 pl-5 text-sm leading-6 text-muted-foreground">
            {children}
          </ul>
        ),
        ol: ({ children }) => (
          <ol className="mt-3 max-w-3xl list-decimal space-y-1.5 pl-5 text-sm leading-6 text-muted-foreground">
            {children}
          </ol>
        ),
        pre: ({ children }) => (
          <pre className="mt-4 overflow-auto rounded-md bg-muted p-4 text-xs leading-5 text-foreground">
            {children}
          </pre>
        ),
        code: ({ children }) => (
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
            {children}
          </code>
        ),
      }}
    >
      {content}
    </ReactMarkdown>
  )
}
