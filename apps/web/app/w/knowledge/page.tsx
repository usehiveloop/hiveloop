"use client"

import { useState } from "react"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  BookOpen01Icon,
  Books02Icon,
  ConnectIcon,
  File01Icon,
  Globe02Icon,
  Search01Icon,
  TextFontIcon,
} from "@hugeicons/core-free-icons"
import { AddConnectionDialog } from "./_components/add-connection-dialog"

const FILTERS = ["Type", "Creator"] as const

export default function KnowledgePage() {
  const [addConnectionOpen, setAddConnectionOpen] = useState(false)

  const actions = [
    {
      icon: ConnectIcon,
      label: "Add Connection",
      onClick: () => setAddConnectionOpen(true),
    },
    { icon: Globe02Icon, label: "Add URL", onClick: () => {} },
    { icon: File01Icon, label: "Add Files", onClick: () => {} },
    { icon: TextFontIcon, label: "Create Text", onClick: () => {} },
  ] as const

  return (
    <>
      <PageHeader title="Knowledge" />

      <div className="mx-auto w-full max-w-4xl space-y-6 px-6 py-10">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">
              Knowledge Base
            </h1>
            <HugeiconsIcon
              icon={BookOpen01Icon}
              size={20}
              className="text-muted-foreground"
            />
          </div>
          <Badge variant="outline" className="h-7 gap-2 px-3 text-sm font-normal">
            <span className="size-2 rounded-full bg-emerald-500" />
            <span className="text-muted-foreground">RAG Storage:</span>
            <span className="font-semibold text-foreground">0 B</span>
            <span className="text-muted-foreground">/ 1.0 MB</span>
          </Badge>
        </div>

        <div className="grid grid-cols-4 gap-3">
          {actions.map(({ icon, label, onClick }) => (
            <button
              key={label}
              type="button"
              onClick={onClick}
              className="flex flex-col items-start gap-3 rounded-xl border border-border bg-background p-4 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/30"
            >
              <HugeiconsIcon icon={icon} size={22} className="text-foreground" />
              <span className="text-sm font-medium text-foreground">
                {label}
              </span>
            </button>
          ))}
        </div>

        <div className="relative">
          <HugeiconsIcon
            icon={Search01Icon}
            size={16}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input placeholder="Search Knowledge Base..." className="pl-9" />
        </div>

        <div className="flex items-center gap-2">
          {FILTERS.map((label) => (
            <Badge
              key={label}
              variant="outline"
              className="cursor-pointer gap-1 text-muted-foreground hover:bg-muted/50"
            >
              <HugeiconsIcon icon={Add01Icon} size={12} />
              {label}
            </Badge>
          ))}
        </div>

        <div className="flex flex-col items-center justify-center gap-3 rounded-2xl border border-border bg-muted/40 px-6 py-16 text-center">
          <div className="flex size-12 items-center justify-center rounded-2xl border border-border bg-background">
            <HugeiconsIcon
              icon={Books02Icon}
              size={20}
              className="text-foreground"
            />
          </div>
          <div className="text-sm font-semibold text-foreground">
            No documents found
          </div>
          <div className="text-sm text-muted-foreground">
            You don&apos;t have any documents yet.
          </div>
        </div>
      </div>

      <AddConnectionDialog
        open={addConnectionOpen}
        onOpenChange={setAddConnectionOpen}
      />
    </>
  )
}
