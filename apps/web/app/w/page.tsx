"use client"

import Link from "next/link"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"

export default function WorkspaceHome() {
  const { data, isLoading } = $api.useQuery("get", "/v1/employees")
  const hivy = data?.data?.[0]
  const specialistCount = hivy?.specialists?.length ?? 0

  return (
    <>
      <PageHeader title="Hivy" />
      <main className="mx-auto w-full max-w-4xl px-6 py-10">
        {isLoading ? (
          <div className="space-y-4">
            <Skeleton className="h-10 w-48" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : !hivy ? (
          <section className="border border-border p-6">
            <h2 className="text-lg font-medium">Hivy is being prepared</h2>
            <p className="mt-2 text-sm text-muted-foreground">
              Connect Slack in settings to finish workspace setup.
            </p>
            <Button className="mt-5" render={<Link href="/w/settings/connections" />}>
              Open connections
            </Button>
          </section>
        ) : (
          <section className="border border-border p-6">
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h2 className="text-2xl font-semibold">{hivy.name ?? "Hivy"}</h2>
                <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
                  {hivy.description ??
                    "Hivy is the managed employee for this workspace."}
                </p>
              </div>
              <Badge variant="secondary">{hivy.status ?? "active"}</Badge>
            </div>
            <div className="mt-8 grid gap-4 sm:grid-cols-3">
              <Metric label="Specialists" value={specialistCount} />
              <Metric label="Skills" value={hivy.attached_skills?.length ?? 0} />
              <Metric label="Triggers" value={hivy.triggers?.length ?? 0} />
            </div>
            <div className="mt-8 flex flex-wrap gap-3">
              <Button render={<Link href={`/w/employees/${hivy.id}`} />}>
                View Hivy
              </Button>
              <Button
                variant="outline"
                render={<Link href="/w/settings/connections" />}
              >
                Connections
              </Button>
            </div>
          </section>
        )}
      </main>
    </>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="border border-border p-4">
      <div className="text-2xl font-semibold">{value}</div>
      <div className="mt-1 text-sm text-muted-foreground">{label}</div>
    </div>
  )
}
