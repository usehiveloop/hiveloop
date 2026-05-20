"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  BookOpen01Icon,
  LockIcon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { CreateSkillDialog } from "@/app/w/settings/_components/create-skill-dialog"
import { SkillDetailDialog } from "@/app/w/settings/_components/skill-detail-dialog"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type Skill = components["schemas"]["skillResponse"]
type AttachedSkill = components["schemas"]["agentSkillResponse"]

export default function SkillsPage() {
  const queryClient = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [selectedSkill, setSelectedSkill] = useState<Skill | null>(null)
  const [pendingSkillId, setPendingSkillId] = useState<string | null>(null)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })
  const skillsQuery = $api.useQuery("get", "/v1/skills", {
    params: { query: { scope: "all", limit: 100 } },
  })

  const hivy = employeesQuery.data?.data?.[0]
  const employeeID = hivy?.id ?? ""
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

  const skills = skillsQuery.data?.data ?? []
  const attached = attachedQuery.data ?? []
  const attachedBySkillID = useMemo(() => {
    const map = new Map<string, AttachedSkill>()
    for (const row of attached) {
      if (row.skill_id) map.set(row.skill_id, row)
    }
    return map
  }, [attached])

  function refreshSkills() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/employees/{id}/skills"],
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
  }

  function handleAttach(skill: Skill) {
    if (!employeeID || !skill.id) return
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
    <>
      <PageHeader
        title="Skills"
        actions={
          <Button onClick={() => setCreating(true)}>
            <HugeiconsIcon icon={Add01Icon} size={16} data-icon="inline-start" />
            New skill
          </Button>
        }
      />

      <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-6 px-6 py-10">
        <section className="border border-border bg-card p-6">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div>
              <p className="text-sm font-medium text-foreground">
                Hivy skill library
              </p>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                Install global and workspace skills on Hivy. Provider-managed
                skills stay locked while their connection is active.
              </p>
            </div>
            <Badge variant="outline">
              {attached.length} installed
            </Badge>
          </div>
        </section>

        {loading ? (
          <div className="grid gap-3 md:grid-cols-2">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-32 w-full" />
            ))}
          </div>
        ) : skills.length === 0 ? (
          <div className="flex min-h-72 flex-col items-center justify-center border border-border text-center">
            <HugeiconsIcon
              icon={BookOpen01Icon}
              className="size-8 text-muted-foreground"
            />
            <p className="mt-4 text-sm font-medium">No skills available</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Create a custom skill for this workspace.
            </p>
          </div>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {skills.map((skill) => {
              const attachment = skill.id
                ? attachedBySkillID.get(skill.id)
                : undefined
              const installed = Boolean(attachment)
              const locked = Boolean(attachment?.locked || attachment?.required)
              const pending = pendingSkillId === skill.id

              return (
                <article
                  key={skill.id}
                  className="flex min-h-36 flex-col justify-between border border-border bg-background p-4"
                >
                  <button
                    type="button"
                    className="min-w-0 text-left"
                    onClick={() => setSelectedSkill(skill)}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <h2 className="truncate text-sm font-medium">
                          {skill.name}
                        </h2>
                        <p className="mt-2 line-clamp-2 text-sm leading-6 text-muted-foreground">
                          {skill.description ?? "No description"}
                        </p>
                      </div>
                      <Badge variant={installed ? "default" : "outline"}>
                        {installed ? "Installed" : skill.source_type ?? "Skill"}
                      </Badge>
                    </div>
                  </button>

                  <div className="mt-5 flex items-center justify-between gap-3">
                    <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
                      {locked ? (
                        <>
                          <HugeiconsIcon icon={LockIcon} size={14} />
                          Provider managed
                        </>
                      ) : (
                        skill.slug ?? skill.source_type ?? "workspace"
                      )}
                    </div>
                    {installed ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={locked || pending}
                        onClick={() => handleDetach(skill)}
                      >
                        {pending ? (
                          <HugeiconsIcon
                            icon={Loading03Icon}
                            className="size-4 animate-spin"
                          />
                        ) : null}
                        Detach
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        disabled={pending}
                        onClick={() => handleAttach(skill)}
                      >
                        {pending ? (
                          <HugeiconsIcon
                            icon={Loading03Icon}
                            className="size-4 animate-spin"
                          />
                        ) : null}
                        Install
                      </Button>
                    )}
                  </div>
                </article>
              )
            })}
          </div>
        )}
      </div>

      <CreateSkillDialog
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => refreshSkills()}
      />
      {selectedSkill ? (
        <SkillDetailDialog
          skill={selectedSkill}
          open
          onOpenChange={(open) => {
            if (!open) setSelectedSkill(null)
          }}
        />
      ) : null}
    </>
  )
}
