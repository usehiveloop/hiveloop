"use client"

import * as React from "react"
import Link from "next/link"
import { Controller, useForm, useWatch } from "react-hook-form"
import { useParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { FormSection } from "@/app/w/_components/form-section"
import { ImagePicker } from "@/components/image-picker"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { formatCategoryLabel } from "@/lib/format-label"
import { EmployeeAgentTemplatesSection } from "./employee-agent-templates-section"
import { EmployeeConnectionsSection } from "./employee-connections-section"
import { EmployeeRuntimeSection } from "./employee-runtime-section"
import { EmployeeSkillsSection } from "./employee-skills-section"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]

interface EmployeeDetailsFormValues {
  name: string
  description: string
  avatarUrl: string
  connectionIds: string[]
  skillIds: string[]
}

export function EmployeeDetailsForm({ employee }: { employee: Employee }) {
  const queryClient = useQueryClient()
  const { id } = useParams<{ id: string }>()
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}/connections/available",
    {
      params: { path: { id } },
    }
  )
  const skillsQuery = $api.useQuery("get", "/v1/skills", {
    params: { query: { limit: 100 } },
  })
  const updateEmployee = $api.useMutation("put", "/v1/employees/{id}")

  const form = useForm<EmployeeDetailsFormValues>({
    defaultValues: employeeFormValues(employee),
  })
  const [connectionsOpen, setConnectionsOpen] = React.useState(false)

  React.useEffect(() => {
    form.reset(employeeFormValues(employee))
  }, [employee, form])

  React.useEffect(() => {
    const allowedIDs = new Set(
      (connectionsQuery.data?.data ?? [])
        .map((connection) => connection.id)
        .filter((connectionID): connectionID is string => Boolean(connectionID))
    )
    if (allowedIDs.size === 0) return
    const currentIDs = form.getValues("connectionIds")
    const filteredIDs = currentIDs.filter((connectionID) =>
      allowedIDs.has(connectionID)
    )
    if (filteredIDs.length !== currentIDs.length) {
      form.setValue("connectionIds", filteredIDs)
    }
  }, [connectionsQuery.data?.data, form])

  const name = useWatch({ control: form.control, name: "name" }) ?? ""
  const description =
    useWatch({ control: form.control, name: "description" }) ?? ""
  const avatarUrl = useWatch({ control: form.control, name: "avatarUrl" }) ?? ""
  const connectionIds = useWatch({
    control: form.control,
    name: "connectionIds",
  })
  const skillIds = useWatch({ control: form.control, name: "skillIds" })

  const lockedSkillIDs = React.useMemo(() => {
    const out = new Set<string>()
    for (const skill of employee.attached_skills ?? []) {
      if (skill.id && (skill.locked || skill.required)) {
        out.add(skill.id)
      }
    }
    return out
  }, [employee.attached_skills])
  const selectedConnectionIDs = React.useMemo(
    () => new Set(connectionIds ?? []),
    [connectionIds]
  )
  const selectedSkillIDs = React.useMemo(
    () => new Set(skillIds ?? []),
    [skillIds]
  )
  const canSubmit =
    name.trim().length > 0 &&
    description.trim().length > 0 &&
    !updateEmployee.isPending

  function setConnectionIds(next: Set<string>) {
    form.setValue("connectionIds", Array.from(next), { shouldDirty: true })
  }

  function setSkillIds(next: Set<string>) {
    form.setValue("skillIds", Array.from(next), { shouldDirty: true })
  }

  function toggleConnection(connectionID: string) {
    const next = new Set(selectedConnectionIDs)
    if (next.has(connectionID)) {
      next.delete(connectionID)
    } else {
      next.add(connectionID)
    }
    setConnectionIds(next)
  }

  function toggleSkill(skillID: string) {
    if (lockedSkillIDs.has(skillID)) return
    const next = new Set(selectedSkillIDs)
    if (next.has(skillID)) {
      next.delete(skillID)
    } else {
      next.add(skillID)
    }
    setSkillIds(next)
  }

  function handleSave() {
    if (!canSubmit) return
    const values = form.getValues()
    updateEmployee.mutate(
      {
        params: { path: { id } },
        body: {
          name: values.name.trim(),
          description: values.description.trim(),
          avatar_url: values.avatarUrl.trim(),
          connection_ids: values.connectionIds,
          skill_ids: values.skillIds,
        },
      },
      {
        onSuccess: (data) => {
          toast.success(
            data.sync_status === "synced"
              ? "Employee updated and synced"
              : "Employee updated"
          )
          for (const warning of data.warnings ?? []) {
            toast.warning(warning)
          }
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/employees/{id}"],
          })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to update employee"))
        },
      }
    )
  }

  return (
    <>
      <PageHeader
        title={employee.name ?? "Employee"}
        breadcrumb="Details"
        sticky
        actions={
          <>
            <Button variant="ghost" render={<Link href="/w" />}>
              Cancel
            </Button>
            <Button
              onClick={handleSave}
              disabled={!canSubmit}
              loading={updateEmployee.isPending}
            >
              Save changes
            </Button>
          </>
        }
      />

      <main className="mx-auto w-full max-w-2xl px-6 pt-10 pb-20">
        <div className="divide-y divide-border/60 [&>section]:py-7 [&>section:first-child]:pt-0 [&>section:last-child]:pb-0">
          <FormSection
            title="Persona"
            description="Your employee's identity. When you change this, we recommend changing the agent's persona across your other services for consistency. This also affects your agent's behaviour."
            aside={
              <Controller
                control={form.control}
                name="avatarUrl"
                render={({ field }) => (
                  <ImagePicker
                    assetType="avatar"
                    value={field.value || undefined}
                    onChange={(url) => field.onChange(url ?? "")}
                    fallback={(name || employee.name || "E").charAt(0)}
                    ariaLabel={avatarUrl ? "Replace avatar" : "Upload avatar"}
                  />
                )}
              />
            }
          >
            <div className="flex flex-col gap-2">
              <Label
                htmlFor="employee-name"
                className="text-[13px] font-medium"
              >
                Name
              </Label>
              <Input
                id="employee-name"
                placeholder="Employee name"
                {...form.register("name")}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label
                htmlFor="employee-description"
                className="text-[13px] font-medium"
              >
                Description
              </Label>
              <Textarea
                id="employee-description"
                className="min-h-24"
                placeholder="What this employee does."
                {...form.register("description")}
              />
            </div>
          </FormSection>

          <FormSection
            title="Category"
            description="Employee category is fixed after creation."
          >
            <div className="flex h-10 items-center rounded-xl border border-border bg-muted/40 px-3 text-sm text-muted-foreground">
              {formatCategoryLabel(employee.category)}
            </div>
          </FormSection>

          <EmployeeConnectionsSection
            connections={connectionsQuery.data?.data ?? []}
            loading={connectionsQuery.isLoading}
            selectedIDs={selectedConnectionIDs}
            dialogOpen={connectionsOpen}
            onDialogOpenChange={setConnectionsOpen}
            onSelectionChange={setConnectionIds}
            onRemove={toggleConnection}
          />

          <EmployeeAgentTemplatesSection
            employeeID={id}
            employeeName={employee.name ?? "this employee"}
          />

          <EmployeeSkillsSection
            skills={skillsQuery.data?.data ?? []}
            loading={skillsQuery.isLoading}
            selectedIDs={selectedSkillIDs}
            lockedIDs={lockedSkillIDs}
            onToggle={toggleSkill}
          />

          <EmployeeRuntimeSection employee={employee} />
        </div>
      </main>
    </>
  )
}

function employeeFormValues(employee: Employee): EmployeeDetailsFormValues {
  return {
    name: employee.name ?? "",
    description: employee.description ?? "",
    avatarUrl: employee.avatar_url ?? "",
    connectionIds: connectionIDsFromEmployee(employee),
    skillIds: (employee.attached_skills ?? [])
      .map((skill) => skill.id)
      .filter((skillID): skillID is string => Boolean(skillID)),
  }
}

function connectionIDsFromEmployee(employee: Employee) {
  const integrations = employee.integrations
  if (!integrations || typeof integrations !== "object") return []
  return Object.keys(integrations)
}
