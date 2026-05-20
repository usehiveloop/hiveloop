"use client"

import Link from "next/link"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  Calendar03Icon,
  CreditCardIcon,
  Plug01Icon,
  Rocket01Icon,
} from "@hugeicons/core-free-icons"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { changelogAnnouncements } from "@/lib/changelog"

export default function WorkspaceHome() {
  const { data, isLoading } = $api.useQuery("get", "/v1/dashboard")

  const planDone = data?.onboarding?.plan_selected === true
  const toolsConnected = data?.onboarding?.extra_tools_connected ?? 0
  const toolsRequired = data?.onboarding?.extra_tools_required ?? 3
  const toolsDone = toolsConnected >= toolsRequired
  const progressTotal = 1 + toolsRequired
  const progressDone = (planDone ? 1 : 0) + Math.min(toolsConnected, toolsRequired)
  const progress = progressTotal > 0 ? progressDone / progressTotal : 0

  return (
    <>
      <PageHeader
        title="Dashboard"
        actions={
          <Button render={<Link href="/w/connections" />}>
            <HugeiconsIcon icon={Plug01Icon} size={16} data-icon="inline-start" />
            Add connection
          </Button>
        }
      />
      <main className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-6 py-8">
        <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {isLoading ? (
            Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="h-44 w-full rounded-lg" />
            ))
          ) : (
            <>
              <MetricCard
                icon={CreditCardIcon}
                label="Credits"
                value={formatCredits(data?.credits?.balance ?? 0)}
                detail={`${formatCredits(data?.credits?.spent_this_period ?? 0)} spent this period`}
                href="/w/settings/billing"
              />
              <MetricCard
                icon={Plug01Icon}
                label="Connections"
                value={String(data?.connections?.total ?? 0)}
                detail={
                  data?.connections?.slack_connected
                    ? `${data?.connections?.non_slack_connected ?? 0} tools beyond Slack`
                    : "Slack is not connected"
                }
                href="/w/connections"
              />
              <SetupCard
                progress={progress}
                planDone={planDone}
                toolsDone={toolsDone}
                toolsConnected={toolsConnected}
                toolsRequired={toolsRequired}
              />
              <MetricCard
                icon={Calendar03Icon}
                label="Scheduled tasks"
                value={String(data?.schedules?.total ?? 0)}
                detail="Cron jobs Hivy has scheduled"
                href="/w/sessions"
              />
            </>
          )}
        </section>

        <section className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
          <div className="rounded-lg border border-border bg-muted/15 p-6">
            <div className="flex items-start justify-between gap-4">
              <div>
                <h2 className="text-lg font-semibold tracking-tight">Hivy workspace</h2>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                  Hivy is ready to work from Slack, use connected tools, and call installed skills from your workspace.
                </p>
              </div>
              <Badge variant="secondary">Managed</Badge>
            </div>
            <div className="mt-6 grid gap-3 sm:grid-cols-3">
              <QuickLink href="/w/skills" label="Manage skills" />
              <QuickLink href="/w/knowledge" label="Knowledge base" />
              <QuickLink href="/w/settings/sandboxes" label="Sandbox setup" />
            </div>
          </div>

          <div className="rounded-lg border border-border p-6">
            <h2 className="text-lg font-semibold tracking-tight">Latest updates</h2>
            <div className="mt-5 flex flex-col gap-4">
              {changelogAnnouncements.slice(0, 3).map((item) => (
                <article key={item.title} className="border-b border-border/60 pb-4 last:border-0 last:pb-0">
                  <time className="font-mono text-[11px] text-muted-foreground">
                    {formatDate(item.date)}
                  </time>
                  <h3 className="mt-1 text-sm font-medium">{item.title}</h3>
                  <p className="mt-1 text-sm leading-5 text-muted-foreground">
                    {item.summary}
                  </p>
                </article>
              ))}
            </div>
          </div>
        </section>
      </main>
    </>
  )
}

function MetricCard({
  icon,
  label,
  value,
  detail,
  href,
}: {
  icon: typeof CreditCardIcon
  label: string
  value: string
  detail: string
  href: string
}) {
  return (
    <Link
      href={href}
      className="group rounded-lg border border-border bg-background p-5 transition-colors hover:bg-muted/25"
    >
      <div className="flex items-center justify-between gap-3">
        <span className="flex size-10 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <HugeiconsIcon icon={icon} size={18} />
        </span>
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          size={16}
          className="text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
        />
      </div>
      <p className="mt-5 text-sm text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold tracking-tight">{value}</p>
      <p className="mt-2 text-sm leading-5 text-muted-foreground">{detail}</p>
    </Link>
  )
}

function SetupCard({
  progress,
  planDone,
  toolsDone,
  toolsConnected,
  toolsRequired,
}: {
  progress: number
  planDone: boolean
  toolsDone: boolean
  toolsConnected: number
  toolsRequired: number
}) {
  return (
    <div className="rounded-lg border border-border bg-background p-5">
      <div className="flex items-center justify-between gap-3">
        <span className="flex size-10 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <HugeiconsIcon icon={Rocket01Icon} size={18} />
        </span>
        <Badge variant={progress >= 1 ? "secondary" : "outline"}>
          {Math.round(progress * 100)}%
        </Badge>
      </div>
      <p className="mt-5 text-sm text-muted-foreground">Onboarding progress</p>
      <Progress className="mt-3" value={progress * 100} />
      <div className="mt-4 space-y-2 text-sm">
        <SetupRow done={planDone} label="Pick a plan" href="/w/settings/billing" />
        <SetupRow
          done={toolsDone}
          label={`${Math.min(toolsConnected, toolsRequired)}/${toolsRequired} extra tools connected`}
          href="/w/connections"
        />
      </div>
    </div>
  )
}

function SetupRow({ done, label, href }: { done: boolean; label: string; href: string }) {
  return (
    <Link href={href} className="flex items-center justify-between gap-2 rounded-md py-1 text-muted-foreground hover:text-foreground">
      <span>{label}</span>
      <span className={done ? "text-foreground" : "text-muted-foreground"}>{done ? "Done" : "Open"}</span>
    </Link>
  )
}

function QuickLink({ href, label }: { href: string; label: string }) {
  return (
    <Button variant="outline" render={<Link href={href} />}>
      {label}
      <HugeiconsIcon icon={ArrowRight01Icon} size={14} data-icon="inline-end" />
    </Button>
  )
}

function formatCredits(value: number) {
  return `${new Intl.NumberFormat("en-US").format(value)} credits`
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

