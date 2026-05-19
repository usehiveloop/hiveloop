"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { EmployeeAgentTemplatesSection } from "./employee-agent-templates-section"
import { EmployeeRuntimeSection } from "./employee-runtime-section"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]

export function EmployeeDetailsForm({ employee }: { employee: Employee }) {
  const { id } = useParams<{ id: string }>()

  return (
    <>
      <PageHeader
        title={employee.name ?? "Hivy"}
        breadcrumb="Details"
        sticky
        actions={
          <Button variant="ghost" render={<Link href="/w" />}>
            Back
          </Button>
        }
      />

      <main className="mx-auto w-full max-w-2xl px-6 pt-10 pb-20">
        <div className="divide-y divide-border/60 [&>section]:py-7 [&>section:first-child]:pt-0 [&>section:last-child]:pb-0">
          <section className="space-y-2">
            <h2 className="text-sm font-medium">Employee</h2>
            <div className="rounded-lg border border-border bg-muted/30 p-4">
              <div className="text-base font-medium">{employee.name ?? "Hivy"}</div>
              {employee.description ? (
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  {employee.description}
                </p>
              ) : null}
            </div>
          </section>

          <EmployeeAgentTemplatesSection
            employeeID={id}
            employeeName={employee.name ?? "Hivy"}
          />

          <EmployeeRuntimeSection employee={employee} />
        </div>
      </main>
    </>
  )
}
