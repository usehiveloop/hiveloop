"use client"

import { useParams } from "next/navigation"
import { PageLoader } from "@/components/page-loader"
import { $api } from "@/lib/api/hooks"
import { EmployeeDetailsForm } from "./_components/employee-details-form"

export default function EmployeeDetailsPage() {
  const { id } = useParams<{ id: string }>()
  const employeeQuery = $api.useQuery("get", "/v1/employees/{id}", {
    params: { path: { id } },
  })

  if (employeeQuery.isLoading || !employeeQuery.data) {
    return <PageLoader description="Loading employee" />
  }

  return <EmployeeDetailsForm employee={employeeQuery.data} />
}
