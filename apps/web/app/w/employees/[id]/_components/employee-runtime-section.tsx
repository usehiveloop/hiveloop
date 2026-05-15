import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"
import { FormSection } from "@/app/w/_components/form-section"
import { formatLabel } from "@/lib/format-label"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]

export function EmployeeRuntimeSection({ employee }: { employee: Employee }) {
  return (
    <FormSection title="Runtime">
      <div className="grid gap-3 rounded-xl border border-border p-4 sm:grid-cols-2">
        <RuntimeStat label="Status" value={employee.status ?? "draft"} />
        <RuntimeStat
          label="Sandbox"
          value={employee.sandbox?.status ?? "not provisioned"}
        />
      </div>
    </FormSection>
  )
}

function RuntimeStat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className="mt-1 flex items-center gap-1.5 text-sm font-medium">
        {value === "active" || value === "running" ? (
          <span className="size-1.5 rounded-full bg-success" />
        ) : value === "upgrading" ? (
          <HugeiconsIcon
            icon={Loading03Icon}
            className="size-3 animate-spin text-primary"
            strokeWidth={2}
          />
        ) : null}
        {formatLabel(value)}
      </p>
    </div>
  )
}
