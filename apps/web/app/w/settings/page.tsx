"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowRight01Icon,
  Tick02Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { $api } from "@/lib/api/hooks"
import { EmployeeUpgradeDialog } from "../_components/employee-upgrade-dialog"

export default function SettingsPage() {
  const [upgradeOpen, setUpgradeOpen] = useState(false)

  const employeesQuery = $api.useQuery("get", "/v1/employees", {
    params: { query: { limit: 1 } },
  })

  const employee = employeesQuery.data?.data?.[0]
  const canUpgrade = employee?.upgrade_available && employee?.id
  const isUpgrading = employee?.sandbox?.status?.toLowerCase() === "upgrading"

  return (
    <div className="flex flex-1 flex-col gap-8">
      <div>
        <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground">
          Settings
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Manage your workspace preferences.
        </p>
      </div>

      <div className="flex flex-col gap-6 rounded-2xl border border-border bg-card p-6">
        <div>
          <h2 className="font-heading text-lg font-medium text-foreground">
            Sandbox runtime
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Your employee sandbox runs on a dedicated runtime image. Upgrades
            recreate the sandbox with the newest image while preserving the
            runtime database.
          </p>
        </div>

        <div className="flex items-center gap-3 rounded-xl border border-border bg-muted/30 p-4">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-muted">
            {isUpgrading ? (
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-5 animate-spin text-primary"
              />
            ) : canUpgrade ? (
              <HugeiconsIcon
                icon={Alert02Icon}
                className="size-5 text-primary"
              />
            ) : (
              <HugeiconsIcon
                icon={Tick02Icon}
                className="size-5 text-success"
              />
            )}
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <p className="text-sm font-medium text-foreground">
              {isUpgrading
                ? "Upgrade in progress"
                : canUpgrade
                  ? "Upgrade available"
                  : "Sandbox is up to date"}
            </p>
            <p className="text-xs text-muted-foreground">
              {isUpgrading
                ? "Your sandbox is being upgraded. This may take a few minutes."
                : canUpgrade
                  ? "A newer runtime image is available for your sandbox."
                  : "Your sandbox is running the latest runtime image."}
            </p>
          </div>
          {canUpgrade ? (
            <Button
              size="sm"
              onClick={() => setUpgradeOpen(true)}
              className="shrink-0 gap-1.5"
            >
              Upgrade
              <HugeiconsIcon icon={ArrowRight01Icon} className="size-3.5" />
            </Button>
          ) : null}
        </div>
      </div>

      {employee ? (
        <EmployeeUpgradeDialog
          employee={employee}
          open={upgradeOpen}
          onOpenChange={setUpgradeOpen}
        />
      ) : null}
    </div>
  )
}
