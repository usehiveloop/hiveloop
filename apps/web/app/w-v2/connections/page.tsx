"use client"

import { useMemo, useState, type ReactNode } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Search01Icon } from "@hugeicons/core-free-icons"
import {
  AwsIcon,
  FigmaIcon,
  GithubIcon,
  GoogleCloudIcon,
  GoogleDriveIcon,
  GoogleExcelIcon,
  PostgresIcon,
  SentryIcon,
  SlackIcon,
  StripeIcon,
  TrelloIcon,
} from "@/components/icons"
import { Input } from "@/components/ui/input"

interface Integration {
  id: string
  name: string
  icon: ReactNode
}

const integrations: Integration[] = [
  {
    id: "slack",
    name: "Slack",
    icon: <SlackIcon size={24} />,
  },
  {
    id: "github",
    name: "GitHub",
    icon: <GithubIcon size={24} />,
  },
  {
    id: "sheets",
    name: "Google Sheets",
    icon: <GoogleExcelIcon size={24} />,
  },
  {
    id: "drive",
    name: "Google Drive",
    icon: <GoogleDriveIcon size={24} />,
  },
  {
    id: "figma",
    name: "Figma",
    icon: <FigmaIcon size={24} />,
  },
  {
    id: "stripe",
    name: "Stripe",
    icon: <StripeIcon size={24} />,
  },
  {
    id: "trello",
    name: "Trello",
    icon: <TrelloIcon size={24} />,
  },
  {
    id: "sentry",
    name: "Sentry",
    icon: <SentryIcon size={24} />,
  },
  {
    id: "postgres",
    name: "Postgres",
    icon: <PostgresIcon size={24} />,
  },
  {
    id: "gcp",
    name: "Google Cloud",
    icon: <GoogleCloudIcon size={24} />,
  },
  {
    id: "aws",
    name: "AWS",
    icon: <AwsIcon size={24} />,
  },
]

export default function ConnectionsPage() {
  const [search, setSearch] = useState("")

  const filteredIntegrations = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return integrations

    return integrations.filter((integration) =>
      integration.name.toLowerCase().includes(query)
    )
  }, [search])

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-7">
      <div className="flex flex-col gap-5">
        <div className="max-w-2xl">
          <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground md:text-4xl">
            Connections
          </h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">
            Connect the tools Hivy can work with across your workspace.
          </p>
        </div>

        <div className="relative">
          <HugeiconsIcon
            icon={Search01Icon}
            className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search integrations"
            className="h-11 rounded-md bg-card pl-9"
          />
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {filteredIntegrations.map((integration) => {
          return (
            <button
              key={integration.id}
              type="button"
              className="group relative flex cursor-pointer items-center gap-3 rounded-md border border-border bg-card p-4 text-left transition-colors hover:border-muted-foreground/25 hover:bg-muted/20 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/30 focus-visible:outline-none"
            >
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-background">
                {integration.icon}
              </div>
              <div className="min-w-0 flex-1">
                <h2 className="truncate text-sm font-semibold text-foreground">
                  {integration.name}
                </h2>
              </div>
            </button>
          )
        })}
      </div>

      {filteredIntegrations.length === 0 ? (
        <div className="flex h-40 items-center justify-center rounded-md border border-dashed border-border text-sm text-muted-foreground">
          No integrations found
        </div>
      ) : null}
    </div>
  )
}
