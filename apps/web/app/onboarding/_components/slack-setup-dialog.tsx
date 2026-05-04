"use client"

import { useWatch } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import { LinkSquare02Icon, Tick02Icon } from "@hugeicons/core-free-icons"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { integrationLogoURL } from "@/components/integration-logo"
import { buildSlackAppCreateUrl } from "../slack-manifest"
import { useOnboarding } from "./context"

export function SlackSetupDialog({
  open,
  onOpenChange,
  onContinue,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onContinue: () => void
}) {
  const { form } = useOnboarding()
  const agentName = useWatch({ control: form.control, name: "agentName" }) || "Hermes"
  const agentDescription = useWatch({ control: form.control, name: "agentDescription" })

  const slackAppUrl = buildSlackAppCreateUrl({ name: agentName, description: agentDescription })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
        <ManifestPreview agentName={agentName} />

        <div className="flex flex-col gap-6 p-6">
          <div className="flex flex-col gap-1.5">
            <DialogTitle className="font-display text-xl tracking-tight">
              Your Slack manifest is ready
            </DialogTitle>
            <DialogDescription className="text-[13.5px] leading-relaxed">
              We configured the bot, scopes, and Socket Mode for you. Create the app in Slack and
              install it to your workspace.
            </DialogDescription>
          </div>

          <ol className="flex flex-col gap-3">
            <Step done>Manifest configured</Step>
            <Step n={1}>Review and create the app in Slack</Step>
            <Step n={2}>Install it to your workspace</Step>
          </ol>

          <Button
            onClick={() => {
              window.open(slackAppUrl, "_blank", "noreferrer,noopener")
              onContinue()
            }}
            className="flex w-full items-center justify-center gap-2"
          >
            <HugeiconsIcon icon={LinkSquare02Icon} className="size-4" />
            Create Slack app for {agentName}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function Step({
  done,
  n,
  children,
}: {
  done?: boolean
  n?: number
  children: React.ReactNode
}) {
  return (
    <li className="flex items-center gap-3">
      {done ? (
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
          <HugeiconsIcon icon={Tick02Icon} className="size-3" strokeWidth={2.75} />
        </span>
      ) : (
        <span className="flex size-5 shrink-0 items-center justify-center rounded-full border-[1.5px] border-muted-foreground/25 text-[10px] font-semibold text-muted-foreground/80">
          {n}
        </span>
      )}
      <span
        className={`text-sm ${done ? "text-muted-foreground line-through decoration-muted-foreground/40" : "font-medium text-foreground"}`}
      >
        {children}
      </span>
    </li>
  )
}

function ManifestPreview({ agentName }: { agentName: string }) {
  return (
    <div className="bg-[var(--surface-code)] px-6 py-5 text-foreground/90">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2 font-mono text-[10.5px] uppercase tracking-[0.14em] text-foreground/40">
          <span className="size-1.5 rounded-full bg-[var(--slack-mark)]" />
          manifest.yaml
        </div>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={integrationLogoURL("slack")}
          alt=""
          className="h-3.5 w-3.5 opacity-60"
        />
      </div>
      <pre className="font-mono text-[12.5px] leading-[1.65] text-foreground/85">
        <code>
          <K>display_information</K>:
          {"\n  "}name: <S>{`"${agentName}"`}</S>
          {"\n"}<K>features</K>:
          {"\n  "}bot_user:
          {"\n    "}display_name: <S>{`"${agentName}"`}</S>
          {"\n"}<K>oauth_config</K>:
          {"\n  "}scopes.bot: [<S>chat:write</S>, <S>im:history</S>, <S>users:read</S>]
          {"\n"}<K>settings</K>:
          {"\n  "}socket_mode: <B>true</B>
        </code>
      </pre>
    </div>
  )
}

function K({ children }: { children: React.ReactNode }) {
  return <span className="text-[var(--syntax-key)]">{children}</span>
}
function S({ children }: { children: React.ReactNode }) {
  return <span className="text-[var(--syntax-string)]">{children}</span>
}
function B({ children }: { children: React.ReactNode }) {
  return <span className="text-[var(--syntax-bool)]">{children}</span>
}
