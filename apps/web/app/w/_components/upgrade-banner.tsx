"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Alert02Icon, ArrowRight01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { $api } from "@/lib/api/hooks"
import { EmployeeUpgradeDialog } from "./employee-upgrade-dialog"

export function UpgradeBanner() {
  const [open, setOpen] = useState(false)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })

  const employee = employeesQuery.data?.data?.[0]
  const showUpgrade = employee?.upgrade_available && employee?.id

  if (!showUpgrade || !employee) return null

  return (
    <>
      <div className="flex items-center gap-3 rounded-2xl border border-primary/20 bg-primary/10 px-4 py-3">
        <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
          <HugeiconsIcon
            icon={Alert02Icon}
            className="size-4"
            strokeWidth={2}
          />
        </span>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <p className="text-sm font-medium text-foreground">
            A sandbox upgrade is available.
          </p>
          <p className="hidden text-sm text-muted-foreground sm:block">
            Upgrade to the newest runtime image.
          </p>
        </div>
        <Button
          size="sm"
          onClick={() => setOpen(true)}
          className="shrink-0 gap-1.5"
        >
          Upgrade
          <HugeiconsIcon icon={ArrowRight01Icon} className="size-3.5" />
        </Button>
      </div>

      <EmployeeUpgradeDialog
        employee={employee}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  )
}
