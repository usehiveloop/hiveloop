"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  Loading03Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Progress } from "@/components/ui/progress"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Employee = components["schemas"]["employeeListItem"]
type EmployeeSandboxUpgrade =
  components["schemas"]["employeeSandboxUpgradeResponse"]

type UpgradeStep = {
  phase: string
  label: string
  description: string
}

const UPGRADE_STEPS: UpgradeStep[] = [
  {
    phase: "queued",
    label: "Queue upgrade",
    description: "Create a single upgrade operation for this employee.",
  },
  {
    phase: "creating_new",
    label: "Create replacement sandbox",
    description: "Provision a fresh sandbox from the current employee image.",
  },
  {
    phase: "sync",
    label: "Sync config and readiness",
    description: "Push the current employee config and require readiness.",
  },
  {
    phase: "pausing_old",
    label: "Pause current sandbox",
    description: "Stop the current runtime now that the replacement is ready.",
  },
  {
    phase: "cleanup_old",
    label: "Schedule old sandbox removal",
    description: "Keep the previous sandbox stopped for 24 hours before deletion.",
  },
  {
    phase: "completed",
    label: "Employee back online",
    description: "The upgraded runtime is healthy and ready for work.",
  },
]

export function EmployeeUpgradeDialog({
  employee,
  open,
  onOpenChange,
}: {
  employee: Employee
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [hasStarted, setHasStarted] = useState(false)
  const [upgradeID, setUpgradeID] = useState<string | null>(null)
  const notifiedStatusRef = useRef<string | null>(null)
  const employeeID = employee.id ?? ""
  const employeeName = employee.name ?? "this employee"

  const startUpgrade = $api.useMutation(
    "post",
    "/v1/employees/{id}/sandbox/upgrade"
  )
  const upgradeQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}/sandbox/upgrades/{upgradeID}",
    {
      params: {
        path: {
          id: employeeID,
          upgradeID: upgradeID ?? "",
        },
      },
    },
    {
      enabled: open && Boolean(employeeID && upgradeID),
      refetchInterval: (query) => {
        const data = query.state.data as EmployeeSandboxUpgrade | undefined
        if (!data) return 1500
        return isUpgradeActive(data) ? 2000 : false
      },
    }
  )

  const upgrade = upgradeQuery.data ?? startUpgrade.data
  const status = upgrade?.status ?? (hasStarted ? "queued" : undefined)
  const isActive = status === "queued" || status === "running"
  const isSucceeded = status === "succeeded"
  const isFailed = status === "failed"
  const progress = useMemo(
    () => upgradeProgress(upgrade, hasStarted),
    [hasStarted, upgrade]
  )
  const startError = startUpgrade.isError
    ? extractErrorMessage(startUpgrade.error, "Failed to start upgrade")
    : null
  const statusError = upgradeQuery.isError
    ? extractErrorMessage(
        upgradeQuery.error,
        "Failed to refresh upgrade status"
      )
    : null

  useEffect(() => {
    if (!upgrade?.upgrade_id || !upgrade.status) return
    const notificationKey = `${upgrade.upgrade_id}:${upgrade.status}`
    if (notifiedStatusRef.current === notificationKey) return

    if (upgrade.status === "succeeded") {
      notifiedStatusRef.current = notificationKey
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
      toast.success(`${employeeName} is back online`)
    }

    if (upgrade.status === "failed") {
      notifiedStatusRef.current = notificationKey
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
      toast.error(upgrade.error_message ?? "Employee sandbox upgrade failed")
    }
  }, [employeeName, queryClient, upgrade])

  function handleStart() {
    if (!employeeID) return
    setHasStarted(true)
    notifiedStatusRef.current = null
    startUpgrade.mutate(
      {
        params: { path: { id: employeeID } },
        body: { smoke_test: false },
      },
      {
        onSuccess: (data) => {
          if (data.upgrade_id) {
            setUpgradeID(data.upgrade_id)
          }
        },
        onError: () => {
          setHasStarted(false)
        },
      }
    )
  }

  function handleRetry() {
    setHasStarted(false)
    setUpgradeID(null)
    notifiedStatusRef.current = null
    startUpgrade.reset()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {!hasStarted ? (
          <>
            <DialogHeader>
              <DialogTitle>Upgrade {employeeName}&apos;s sandbox</DialogTitle>
              <DialogDescription>
                This recreates the employee sandbox on the newest runtime image
                and restores the runtime database before the employee comes back
                online.
              </DialogDescription>
            </DialogHeader>

            <div className="rounded-2xl border border-primary/20 bg-primary/10 p-4 text-sm">
              <div className="flex gap-3">
                <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
                  <HugeiconsIcon
                    icon={Alert02Icon}
                    className="size-4"
                    strokeWidth={2}
                  />
                </span>
                <div className="flex flex-col gap-2">
                  <p className="font-medium text-foreground">
                    Pick a quiet time before starting.
                  </p>
                  <p className="leading-relaxed text-muted-foreground">
                    The current sandbox is stopped during cutover. Any Slack or
                    HTTP requests sent during that window may need to be resent
                    after the employee is back online.
                  </p>
                </div>
              </div>
            </div>

            <ul className="space-y-2 text-sm text-muted-foreground">
              <li>Back up and verify the current SQLite runtime database.</li>
              <li>Create a fresh sandbox from the current employee image.</li>
              <li>
                Restore the database, sync config, and wait for readiness.
              </li>
            </ul>

            {startError ? (
              <p className="rounded-2xl bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {startError}
              </p>
            ) : null}

            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                onClick={handleStart}
                loading={startUpgrade.isPending}
                disabled={!employeeID}
              >
                Start upgrade
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>
                {isSucceeded
                  ? `${employeeName} is back online`
                  : isFailed
                    ? `Upgrade failed for ${employeeName}`
                    : `Upgrading ${employeeName}`}
              </DialogTitle>
              <DialogDescription>
                {isSucceeded
                  ? "The new sandbox is healthy, configured, and ready."
                  : isFailed
                    ? "The old sandbox was restored where rollback was needed. Review the error before retrying."
                    : "Keep this open to watch progress. The operation continues even if you close the dialog."}
              </DialogDescription>
            </DialogHeader>

            <div className="flex flex-col gap-5">
              <Progress value={progress} />
              <ol className="space-y-3">
                {UPGRADE_STEPS.map((step) => (
                  <UpgradeStepRow
                    key={step.phase}
                    step={step}
                    state={stepState(step.phase, upgrade, hasStarted)}
                  />
                ))}
              </ol>
            </div>

            {upgrade?.error_message ? (
              <p className="rounded-2xl bg-destructive/10 px-4 py-3 text-sm text-destructive">
                {upgrade.error_message}
              </p>
            ) : statusError && isActive ? (
              <p className="rounded-2xl bg-muted px-4 py-3 text-sm text-muted-foreground">
                {statusError}
              </p>
            ) : null}

            <DialogFooter className="sm:items-center sm:justify-between">
              <p className="text-xs text-muted-foreground">
                {upgradeID ? `Upgrade ${upgradeID.slice(0, 8)}` : "Starting"}
              </p>
              <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                <Button variant="outline" onClick={() => onOpenChange(false)}>
                  {isSucceeded ? "Done" : "Close"}
                </Button>
                {isFailed ? (
                  <Button onClick={handleRetry}>Try again</Button>
                ) : null}
              </div>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function UpgradeStepRow({
  step,
  state,
}: {
  step: UpgradeStep
  state: "done" | "active" | "pending" | "failed"
}) {
  return (
    <li className="flex gap-3">
      <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center">
        {state === "done" ? (
          <span className="flex size-5 items-center justify-center rounded-full bg-success/15 text-success">
            <HugeiconsIcon
              icon={Tick02Icon}
              className="size-3"
              strokeWidth={2.75}
            />
          </span>
        ) : state === "active" ? (
          <HugeiconsIcon
            icon={Loading03Icon}
            className="size-4 animate-spin text-primary"
            strokeWidth={2}
          />
        ) : state === "failed" ? (
          <span className="flex size-5 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <HugeiconsIcon
              icon={Alert02Icon}
              className="size-3"
              strokeWidth={2.25}
            />
          </span>
        ) : (
          <span className="size-1.5 rounded-full bg-muted-foreground/30" />
        )}
      </span>
      <span className="flex min-w-0 flex-col gap-0.5">
        <span
          className={cn(
            "text-sm font-medium",
            state === "pending"
              ? "text-muted-foreground/70"
              : state === "failed"
                ? "text-destructive"
                : "text-foreground"
          )}
        >
          {step.label}
        </span>
        <span className="text-xs leading-relaxed text-muted-foreground">
          {step.description}
        </span>
      </span>
    </li>
  )
}

function isUpgradeActive(upgrade: EmployeeSandboxUpgrade | undefined) {
  return upgrade?.status === "queued" || upgrade?.status === "running"
}

function upgradeProgress(
  upgrade: EmployeeSandboxUpgrade | undefined,
  hasStarted: boolean
) {
  if (upgrade?.status === "succeeded") return 100
  if (upgrade?.status === "failed") {
    return Math.max(8, phaseProgress(upgrade.phase))
  }
  if (!hasStarted) return 0
  return Math.min(95, phaseProgress(upgrade?.phase ?? "queued"))
}

function phaseProgress(phase: string | undefined) {
  const idx = phaseIndex(phase)
  if (idx < 0) return 8
  return Math.round(((idx + 1) / UPGRADE_STEPS.length) * 100)
}

function stepState(
  phase: string,
  upgrade: EmployeeSandboxUpgrade | undefined,
  hasStarted: boolean
): "done" | "active" | "pending" | "failed" {
  if (upgrade?.status === "succeeded") return "done"
  const currentPhase = upgrade?.phase ?? (hasStarted ? "queued" : undefined)
  const currentIdx = phaseIndex(currentPhase)
  const stepIdx = phaseIndex(phase)

  if (upgrade?.status === "failed") {
    if (stepIdx < currentIdx) return "done"
    if (stepIdx === currentIdx) return "failed"
    return "pending"
  }
  if (stepIdx < currentIdx) return "done"
  if (stepIdx === currentIdx) return "active"
  return "pending"
}

function phaseIndex(phase: string | undefined) {
  return UPGRADE_STEPS.findIndex((step) => step.phase === phase)
}
