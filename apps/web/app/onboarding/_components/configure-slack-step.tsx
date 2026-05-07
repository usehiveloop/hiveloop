"use client"

import { useState } from "react"
import { useWatch } from "react-hook-form"
import { AnimatePresence, motion } from "motion/react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowLeft01Icon,
  ArrowRight01Icon,
  EyeIcon,
  Loading03Icon,
  PlayIcon,
  Tick02Icon,
  ViewOffIcon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { integrationLogoURL } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { InstructionLightbox } from "./instruction-lightbox"
import { StepHeader } from "./step-header"
import { HomeChannelDialog, type SlackChannel } from "./home-channel-dialog"
import { useOnboarding, type OnboardingFormValues } from "./context"

export function ConfigureSlackStep() {
  const { form, goBack, goNext, createEmployee } = useOnboarding()
  const [showBot, setShowBot] = useState(false)
  const [showApp, setShowApp] = useState(false)
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)
  const [videoOpen, setVideoOpen] = useState(false)
  const [channelDialogOpen, setChannelDialogOpen] = useState(false)
  const [channels, setChannels] = useState<SlackChannel[]>([])

  const botToken = useWatch({ control: form.control, name: "slackBotToken" })
  const appToken = useWatch({ control: form.control, name: "slackAppToken" })
  const agentName = useWatch({ control: form.control, name: "agentName" })
  const botValid = botToken?.startsWith("xoxb-") ?? false
  const appValid = appToken?.startsWith("xapp-") ?? false

  const createSlackProfile = $api.useMutation(
    "post",
    "/v1/agents/{agentID}/profiles/slack",
  )
  const updateSlackConfig = $api.useMutation(
    "patch",
    "/v1/agents/{agentID}/profiles/slack/config",
  )
  const submitting = createSlackProfile.isPending
  const errorMessage = createSlackProfile.isError
    ? extractErrorMessage(
        createSlackProfile.error,
        "Could not save Slack credentials. Try again.",
      )
    : null
  const homeChannelError = updateSlackConfig.isError
    ? extractErrorMessage(
        updateSlackConfig.error,
        "Could not set the home channel. Try again.",
      )
    : null

  const canContinue =
    botValid && appValid && !submitting && Boolean(createEmployee.agentId)

  const handleContinue = () => {
    if (!createEmployee.agentId) return
    createSlackProfile.mutate(
      {
        params: { path: { agentID: createEmployee.agentId } },
        body: {
          label: agentName?.trim() || "Slack",
          bot_token: botToken.trim(),
          app_token: appToken.trim(),
        },
      },
      {
        onSuccess: (data) => {
          const identity = (data.profile?.identity ?? {}) as Record<string, unknown>
          const teamName = typeof identity.team_name === "string" ? identity.team_name : ""
          if (teamName && !form.getValues("businessName").trim()) {
            form.setValue("businessName", teamName, { shouldDirty: true })
          }
          setChannels(data.channels ?? [])
          updateSlackConfig.reset()
          setChannelDialogOpen(true)
        },
      },
    )
  }

  const handleHomeChannelConfirm = (channel: SlackChannel) => {
    if (!createEmployee.agentId || !channel.id) return
    updateSlackConfig.mutate(
      {
        params: { path: { agentID: createEmployee.agentId } },
        body: { home_channel_id: channel.id },
      },
      {
        onSuccess: () => {
          setChannelDialogOpen(false)
          goNext()
        },
      },
    )
  }

  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-7">
      <StepHeader
        eyebrow={
          <>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={integrationLogoURL("slack")} alt="" className="h-3.5 w-3.5" />
            Slack credentials
          </>
        }
        title="Paste two tokens"
        description="You copied these after installing the manifest. They're encrypted at rest and never leave your workspace."
      />

      <ol className="flex flex-col gap-3">
        {INSTRUCTION_CARDS.map((card, idx) => (
          <li
            key={idx}
            className="flex items-center gap-4 rounded-xl border border-border bg-muted/30 p-3"
          >
            <button
              type="button"
              onClick={() => setLightboxIndex(idx)}
              aria-label={`Open ${card.title} preview`}
              className="aspect-[5/3] w-24 shrink-0 cursor-pointer overflow-hidden rounded-md border border-border/60 bg-muted transition-opacity outline-none hover:opacity-90 focus-visible:ring-3 focus-visible:ring-ring/30"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={card.image}
                alt=""
                className="h-full w-full object-cover"
              />
            </button>
            <div className="min-w-0 flex-1 space-y-1">
              <h3 className="text-sm font-semibold">{card.title}</h3>
              <p className="text-xs leading-relaxed text-muted-foreground">{card.description}</p>
            </div>
          </li>
        ))}
      </ol>

      <button
        type="button"
        onClick={() => setVideoOpen(true)}
        className="flex cursor-pointer items-center gap-3 rounded-xl border border-border bg-muted/30 p-3 text-left transition-colors outline-none hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/30"
      >
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <HugeiconsIcon icon={PlayIcon} className="size-4" strokeWidth={2} />
        </span>
        <span className="min-w-0 flex-1 text-sm font-medium">
          Need help? Watch a 20-second video
        </span>
        <HugeiconsIcon
          icon={ArrowRight01Icon}
          className="size-4 shrink-0 text-muted-foreground"
        />
      </button>

      <InstructionLightbox
        items={INSTRUCTION_CARDS.map((c) => ({
          title: c.title,
          description: c.description,
          src: c.image,
          kind: "image" as const,
        }))}
        index={lightboxIndex}
        onIndexChange={setLightboxIndex}
        onClose={() => setLightboxIndex(null)}
      />

      <InstructionLightbox
        items={[
          {
            title: "Set up Slack tokens",
            description: "Quick walkthrough of the install + token-grab flow.",
            kind: "youtube",
            videoId: "vcOG0fRxdoc",
          },
        ]}
        index={videoOpen ? 0 : null}
        onIndexChange={() => {
          /* single-item lightbox; no nav */
        }}
        onClose={() => setVideoOpen(false)}
      />

      <div className="divide-y divide-border overflow-hidden rounded-2xl border border-border bg-background">
        <CredentialField
          id="slack-bot-token"
          label="Bot User OAuth Token"
          prefix="xoxb-"
          fieldName="slackBotToken"
          reveal={showBot}
          onToggleReveal={() => setShowBot((prev) => !prev)}
          valid={botValid}
        />
        <CredentialField
          id="slack-app-token"
          label="App-Level Token"
          prefix="xapp-"
          fieldName="slackAppToken"
          reveal={showApp}
          onToggleReveal={() => setShowApp((prev) => !prev)}
          valid={appValid}
        />
      </div>

      {errorMessage ? (
        <div className="flex items-start gap-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-[13px] text-destructive">
          <HugeiconsIcon
            icon={Alert02Icon}
            className="mt-0.5 size-4 shrink-0"
            strokeWidth={2}
          />
          <span className="leading-relaxed">{errorMessage}</span>
        </div>
      ) : null}

      <div className="flex items-center justify-between">
        <Button
          variant="ghost"
          onClick={goBack}
          disabled={submitting}
          className="gap-2"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} className="size-4" />
          Back
        </Button>
        <Button
          onClick={handleContinue}
          disabled={!canContinue}
          className="gap-2"
        >
          {submitting ? (
            <>
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-4 animate-spin"
                strokeWidth={2}
              />
              Verifying…
            </>
          ) : (
            <>
              Continue
              <HugeiconsIcon icon={ArrowRight01Icon} className="size-4" />
            </>
          )}
        </Button>
      </div>

      <HomeChannelDialog
        open={channelDialogOpen}
        onOpenChange={setChannelDialogOpen}
        channels={channels}
        submitting={updateSlackConfig.isPending}
        errorMessage={homeChannelError}
        onConfirm={handleHomeChannelConfirm}
      />
    </div>
  )
}

// Drop real screenshots at these paths to populate the cards. Aspect ratio
// 5:3 — anything roughly that shape will look right; cards render with
// object-cover so portraits crop cleanly.
const INSTRUCTION_CARDS: Array<{
  title: string
  description: string
  image: string
}> = [
  {
    title: "Install app and copy bot token",
    description: "Click Install to Workspace. Copy the bot token (xoxb-).",
    image: "/images/onboarding/slack/bot-token.png",
  },
  {
    title: "Generate an App-Level Token",
    description: "Add all the scopes shown, then generate. Copy the xapp- token.",
    image: "/images/onboarding/slack/generate-app-token.png",
  },
  {
    title: "Update your employee's profile",
    description: "Set the app name, icon, and background color.",
    image: "/images/onboarding/slack/profile.png",
  },
]

function CredentialField({
  id,
  label,
  prefix,
  fieldName,
  reveal,
  onToggleReveal,
  valid,
}: {
  id: string
  label: string
  prefix: string
  fieldName: keyof Pick<OnboardingFormValues, "slackBotToken" | "slackAppToken">
  reveal: boolean
  onToggleReveal: () => void
  valid: boolean
}) {
  const { form } = useOnboarding()

  return (
    <div className="flex flex-col gap-2.5 p-4 transition-colors focus-within:bg-muted/30">
      <div className="flex items-center justify-between">
        <Label htmlFor={id} className="text-[13px] font-medium">
          {label}
        </Label>
        <code className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium tracking-wider text-muted-foreground">
          {prefix}
        </code>
      </div>

      <div className="relative">
        <Input
          id={id}
          type={reveal ? "text" : "password"}
          placeholder={`${prefix}•••••••••••••`}
          spellCheck={false}
          autoComplete="off"
          autoCorrect="off"
          autoCapitalize="off"
          className="h-10 pr-20 font-mono text-[13px] tracking-tight"
          {...form.register(fieldName)}
        />
        <div className="absolute inset-y-0 right-1.5 flex items-center gap-0.5">
          <AnimatePresence>
            {valid ? (
              <motion.span
                key="ok"
                initial={{ opacity: 0, scale: 0.6 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={{ opacity: 0, scale: 0.6 }}
                transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
                className="flex size-6 items-center justify-center rounded-full bg-primary/15 text-primary"
                aria-label="Valid token prefix"
              >
                <HugeiconsIcon icon={Tick02Icon} className="size-3" strokeWidth={2.75} />
              </motion.span>
            ) : null}
          </AnimatePresence>
          <button
            type="button"
            onClick={onToggleReveal}
            aria-label={reveal ? "Hide token" : "Show token"}
            className="flex size-7 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <HugeiconsIcon
              icon={reveal ? ViewOffIcon : EyeIcon}
              className="size-3.5"
              strokeWidth={1.75}
            />
          </button>
        </div>
      </div>
    </div>
  )
}
