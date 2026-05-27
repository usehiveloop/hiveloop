"use client"

import { useEffect } from "react"
import { motion } from "motion/react"
import { useForm } from "react-hook-form"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowRight01Icon,
  CheckmarkCircle01Icon,
  Loading03Icon,
  Plug01Icon,
  Search01Icon,
  SlackIcon,
} from "@hugeicons/core-free-icons"
import { AddConnectionDialog } from "@/app/w/connections/_components/add-connection-dialog"
import { IntegrationLogo } from "@/components/integration-logo"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import {
  type Channel,
  type Integration,
  type OnboardingStep,
  type OrgUpdateRequest,
  useOnboarding,
} from "./use-onboarding"

type StepItem = {
  id: OnboardingStep
  label: string
}

const steps: StepItem[] = [
  { id: "slack", label: "Invite hivy to slack" },
  { id: "connections", label: "Add connections" },
  { id: "business", label: "Your business" },
]

type BusinessFormValues = Required<
  Pick<OrgUpdateRequest, "name" | "website" | "prompt_company">
>

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg
      width="16"
      height="16"
      viewBox="0 0 16 16"
      fill="none"
      className={className}
    >
      <path
        d="M3 8.5L6.5 12L13 5"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

function StepIndicator({ current }: { current: OnboardingStep }) {
  const currentIndex = steps.findIndex((step) => step.id === current)

  return (
    <div className="flex w-full max-w-lg items-start justify-between">
      {steps.map((step, index) => {
        const isActive = index === currentIndex
        const isDone = index < currentIndex
        const isPending = index > currentIndex

        return (
          <div
            key={step.id}
            className="flex flex-1 items-center last:flex-initial"
          >
            <div className="flex flex-col items-center gap-2.5">
              <div className="relative">
                {isActive ? (
                  <motion.div
                    layoutId="active-step-ring"
                    className="absolute -inset-1.5 rounded-full border-2 border-primary/30"
                    transition={{ type: "spring", stiffness: 300, damping: 30 }}
                  />
                ) : null}
                <motion.div
                  initial={false}
                  animate={{
                    backgroundColor:
                      isDone || isActive ? "var(--primary)" : "transparent",
                    borderColor:
                      isDone || isActive ? "var(--primary)" : "var(--border)",
                    color:
                      isDone || isActive
                        ? "var(--primary-foreground)"
                        : "var(--muted-foreground)",
                  }}
                  transition={{ duration: 0.25, ease: "easeOut" }}
                  className={cn(
                    "flex size-10 items-center justify-center rounded-full border-2 text-sm font-semibold",
                    isPending && "opacity-50"
                  )}
                >
                  {isDone ? <CheckIcon className="size-4" /> : index + 1}
                </motion.div>
              </div>

              <span
                className={cn(
                  "text-xs font-medium whitespace-nowrap transition-colors duration-300",
                  isActive || isDone
                    ? "text-foreground"
                    : "text-muted-foreground/60"
                )}
              >
                {step.label}
              </span>
            </div>

            {index < steps.length - 1 ? (
              <div className="relative mx-2 -mt-6 h-[2px] flex-1 overflow-hidden rounded-full bg-border sm:mx-4">
                <motion.div
                  className="absolute inset-y-0 left-0 rounded-full bg-primary"
                  initial={false}
                  animate={{ width: isDone ? "100%" : "0%" }}
                  transition={{ duration: 0.35, ease: "easeOut" }}
                />
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

function SlackLogo({
  className,
  size = 48,
}: {
  className?: string
  size?: number
}) {
  return (
    <svg viewBox="0 0 127 127" width={size} height={size} className={className}>
      <path
        d="M27.2 80.0c0 7.3-5.9 13.2-13.2 13.2S.8 87.3.8 80c0-7.3 5.9-13.2 13.2-13.2h13.2v13.2zm6.6 0c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2v33c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V80z"
        fill="#E01E5A"
      />
      <path
        d="M47.0 27.0c-7.3 0-13.2-5.9-13.2-13.2S39.7.6 47.0.6s13.2 5.9 13.2 13.2v13.2H47.0zm0 6.7c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H13.5C6.2 60.1.3 54.2.3 46.9s5.9-13.2 13.2-13.2h33.5z"
        fill="#36C5F0"
      />
      <path
        d="M99.9 46.9c0-7.3 5.9-13.2 13.2-13.2s13.2 5.9 13.2 13.2-5.9 13.2-13.2 13.2H99.9V46.9zm-6.6 0c0 7.3-5.9 13.2-13.2 13.2s-13.2-5.9-13.2-13.2V13.5C66.9 6.2 72.8.3 80.1.3s13.2 5.9 13.2 13.2v33.4z"
        fill="#2EB67D"
      />
      <path
        d="M80.1 99.8c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2-13.2-5.9-13.2-13.2V99.8h13.2zm0-6.6c-7.3 0-13.2-5.9-13.2-13.2s5.9-13.2 13.2-13.2h33.5c7.3 0 13.2 5.9 13.2 13.2s-5.9 13.2-13.2 13.2H80.1z"
        fill="#ECB22E"
      />
    </svg>
  )
}

function GhostLogo({ size = 40 }: { size?: number }) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      fill="currentColor"
      className="text-muted-foreground"
    >
      <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
      <ellipse
        cx="318.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
      <ellipse
        cx="457.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
    </svg>
  )
}

function SlackStep({
  slackConnected,
  slackIntegration,
  connectingId,
  channels,
  isLoadingChannels,
  channelSearch,
  selectedChannelIds,
  isJoiningChannels,
  onConnectSlack,
  onSearchChannels,
  onToggleChannel,
  onToggleAllChannels,
  onJoinSelectedChannels,
  onJoinAllPublicChannels,
}: {
  slackConnected: boolean
  slackIntegration?: Integration
  connectingId: string | null
  channels: Channel[]
  isLoadingChannels: boolean
  channelSearch: string
  selectedChannelIds: Set<string>
  isJoiningChannels: boolean
  onConnectSlack: () => void
  onSearchChannels: (value: string) => void
  onToggleChannel: (id: string | undefined) => void
  onToggleAllChannels: () => void
  onJoinSelectedChannels: () => void
  onJoinAllPublicChannels: () => void
}) {
  if (!slackConnected) {
    return (
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.3, ease: "easeOut" }}
        className="flex flex-col items-center text-center"
      >
        <div className="mb-8 flex items-center gap-4">
          <GhostLogo size={48} />
          <div className="flex items-center gap-1.5 text-muted-foreground">
            <div className="h-px w-8 bg-border" />
            <HugeiconsIcon icon={ArrowRight01Icon} size={14} />
            <div className="h-px w-8 bg-border" />
          </div>
          <SlackLogo size={48} />
        </div>

        <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Connect your Slack workspace
        </h1>
        <p className="mt-4 max-w-md text-base leading-relaxed text-muted-foreground">
          Hivy lives in Slack. Connecting your workspace lets Hivy join
          channels, answer questions, and complete tasks alongside your team.
        </p>

        <div className="mt-8">
          <Button
            size="lg"
            onClick={onConnectSlack}
            disabled={!slackIntegration?.id}
            loading={connectingId === slackIntegration?.id}
            className="min-w-64 gap-2"
          >
            <HugeiconsIcon icon={SlackIcon} size={18} />
            Connect Slack workspace
          </Button>
        </div>
      </motion.div>
    )
  }

  const selectedInView = channels.filter(
    (channel) => channel.id && selectedChannelIds.has(channel.id)
  ).length
  const selectableInView = channels.filter(
    (channel) => !channel.is_private
  ).length

  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: "easeOut" }}
      className="flex w-full max-w-md flex-col"
    >
      <div className="mb-8 flex flex-col items-center text-center">
        <div className="mb-4 flex size-12 items-center justify-center rounded-xl border border-border bg-card">
          <HugeiconsIcon icon={SlackIcon} size={22} />
        </div>
        <h2 className="font-heading text-2xl font-normal tracking-[-0.02em] text-foreground">
          Choose channels
        </h2>
        <p className="mt-1.5 text-sm text-muted-foreground">
          Select the public Slack channels Hivy should join.
        </p>
      </div>

      <div className="relative mb-4">
        <HugeiconsIcon
          icon={Search01Icon}
          className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={channelSearch}
          onChange={(event) => onSearchChannels(event.target.value)}
          placeholder="Search channels"
          className="bg-white pl-9 dark:bg-input"
        />
      </div>

      <div className="mb-3 flex items-center justify-between px-1">
        <span className="text-xs text-muted-foreground">
          {selectedInView} of {selectableInView} visible public channels
          selected
        </span>
        <Button
          variant="ghost"
          size="sm"
          onClick={onToggleAllChannels}
          className="h-auto px-0 py-0 text-xs font-medium text-muted-foreground hover:text-foreground"
        >
          Toggle visible
        </Button>
      </div>

      <ScrollArea className="mb-6 h-80 md:h-105">
        {isLoadingChannels ? (
          <div className="flex flex-col gap-2 pr-2">
            {Array.from({ length: 8 }).map((_, index) => (
              <Skeleton key={index} className="h-14 w-full rounded-xl" />
            ))}
          </div>
        ) : channels.length === 0 ? (
          <div className="flex h-40 items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
            No channels found
          </div>
        ) : (
          <div className="flex flex-col gap-2 pr-2">
            {channels.map((channel) => {
              const isSelected = Boolean(
                channel.id && selectedChannelIds.has(channel.id)
              )
              const selectable = !channel.is_private
              return (
                <button
                  key={channel.id}
                  type="button"
                  disabled={!selectable}
                  onClick={() => onToggleChannel(channel.id)}
                  className={cn(
                    "group flex items-center gap-4 rounded-xl border border-border bg-card px-5 py-2 text-left transition-all hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60",
                    selectable && "cursor-pointer"
                  )}
                >
                  <div className="flex min-w-0 flex-1 flex-col">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold text-foreground">
                        #{channel.name}
                      </span>
                      {channel.is_private ? (
                        <Badge variant="secondary">Private</Badge>
                      ) : null}
                      {channel.is_member ? (
                        <Badge variant="outline">Joined</Badge>
                      ) : null}
                    </div>
                    <span className="text-xs text-muted-foreground">
                      {channel.num_members ?? 0} members
                    </span>
                  </div>

                  <div
                    className={cn(
                      "shrink-0 text-primary transition-opacity",
                      isSelected ? "opacity-100" : "opacity-0"
                    )}
                  >
                    <CheckIcon className="size-5" />
                  </div>
                </button>
              )
            })}
          </div>
        )}
      </ScrollArea>

      <div className="grid gap-3 sm:grid-cols-2">
        <Button
          variant="outline"
          size="lg"
          onClick={onJoinAllPublicChannels}
          loading={isJoiningChannels}
        >
          Invite all public
        </Button>
        <Button
          size="lg"
          onClick={onJoinSelectedChannels}
          disabled={selectedChannelIds.size === 0}
          loading={isJoiningChannels}
          className="gap-2"
        >
          Invite selected
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            size={16}
            data-icon="inline-end"
          />
        </Button>
      </div>
    </motion.div>
  )
}

function ConnectionsStep({
  integrations,
  connectedIntegrationIds,
  nonSlackConnections,
  isLoading,
  connectingId,
  search,
  canContinue,
  onSearch,
  onConnect,
  onContinue,
}: {
  integrations: Integration[]
  connectedIntegrationIds: Set<string | undefined>
  nonSlackConnections: unknown[]
  isLoading: boolean
  connectingId: string | null
  search: string
  canContinue: boolean
  onSearch: (value: string) => void
  onConnect: (integration: Integration) => void
  onContinue: () => void
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: "easeOut" }}
      className="flex w-full max-w-2xl flex-col"
    >
      <div className="mb-8 text-left">
        <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
          Add connections
        </h1>
        <p className="mt-4 max-w-lg text-base leading-relaxed text-muted-foreground">
          Connect the tools Hivy will work with: code, docs, tickets, billing,
          and customer systems.
        </p>
      </div>

      <div className="relative mb-6">
        <HugeiconsIcon
          icon={Search01Icon}
          className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          placeholder="Search integrations"
          className="bg-white pl-9 dark:bg-input"
        />
      </div>

      <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {isLoading
          ? Array.from({ length: 9 }).map((_, index) => (
              <Skeleton key={index} className="h-14 rounded-xl" />
            ))
          : integrations.map((integration) => {
              const isConnected = connectedIntegrationIds.has(integration.id)
              const isConnecting = connectingId === integration.id
              return (
                <button
                  key={integration.id}
                  type="button"
                  disabled={isConnected || connectingId !== null}
                  onClick={() => onConnect(integration)}
                  className={cn(
                    "group relative flex cursor-pointer items-center gap-3 rounded-xl border border-border bg-card px-5 py-2 text-left transition-all hover:bg-muted disabled:cursor-not-allowed disabled:opacity-70",
                    isConnected && "border-primary/30 bg-primary/4"
                  )}
                >
	                  <IntegrationLogo
	                    provider={integration.provider ?? ""}
	                    size={40}
	                  />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                    {integration.display_name}
                  </span>
                  {isConnected ? (
                    <HugeiconsIcon
                      icon={CheckmarkCircle01Icon}
                      className="size-4 text-primary"
                    />
                  ) : isConnecting ? (
                    <HugeiconsIcon
                      icon={Loading03Icon}
                      className="size-4 animate-spin text-muted-foreground"
                    />
                  ) : null}
                </button>
              )
            })}
      </div>

      {!isLoading && integrations.length === 0 ? (
        <div className="mb-6 flex h-32 items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
          No integrations found
        </div>
      ) : null}

      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">
          {nonSlackConnections.length} of 2 connected
        </span>
        <Button onClick={onContinue} disabled={!canContinue} className="gap-2">
          Continue
          <HugeiconsIcon
            icon={ArrowRight01Icon}
            size={16}
            data-icon="inline-end"
          />
        </Button>
      </div>
    </motion.div>
  )
}

function BusinessStep({
  currentOrg,
  isLoading,
  isSaving,
  onFinish,
}: {
  currentOrg?: {
    name?: string
    website?: string
    prompt_company?: string
  }
  isLoading: boolean
  isSaving: boolean
  onFinish: (body: OrgUpdateRequest) => void
}) {
  const { register, handleSubmit, reset, watch } = useForm<BusinessFormValues>({
    defaultValues: {
      name: "",
      website: "",
      prompt_company: "",
    },
  })

  useEffect(() => {
    if (!currentOrg) return
    reset({
      name: currentOrg.name ?? "",
      website: currentOrg.website ?? "",
      prompt_company: currentOrg.prompt_company ?? "",
    })
  }, [currentOrg, reset])

  const businessName = watch("name")
  const onSubmit = handleSubmit((values) =>
    onFinish({
      name: values.name,
      website: values.website,
      prompt_company: values.prompt_company,
    })
  )

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: "easeOut" }}
      className="grid w-full max-w-4xl gap-10 lg:grid-cols-[minmax(0,420px)_1fr] lg:items-center"
    >
      <div className="flex w-full flex-col">
        <div className="mb-8 text-center lg:text-left">
          <h1 className="font-heading text-3xl leading-[1.1] font-normal tracking-[-0.02em] text-foreground md:text-4xl">
            Tell hivy more about your business
          </h1>
          <p className="mx-auto mt-4 max-w-md text-base leading-relaxed text-muted-foreground lg:mx-0">
            This becomes workspace context for Hivy when it plans work and
            answers teammates.
          </p>
        </div>

        {isLoading ? (
          <div className="space-y-5">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-32 w-full" />
          </div>
        ) : (
          <form onSubmit={onSubmit} className="flex flex-col gap-5">
            <div className="space-y-2">
              <Label htmlFor="business-name">Business name</Label>
              <Input
                id="business-name"
                placeholder="Acme Inc"
                required
                {...register("name", { required: true })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="business-website">Website</Label>
              <Input
                id="business-website"
                type="url"
                placeholder="https://acme.com"
                {...register("website")}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="business-description">
                What does your business do?
              </Label>
              <Textarea
                id="business-description"
                className="min-h-28"
                placeholder="We build software for remote teams..."
                rows={8}
                {...register("prompt_company")}
              />
            </div>

            <Button
              type="submit"
              size="lg"
              loading={isSaving}
              disabled={!businessName?.trim()}
              className="mt-2 w-full gap-2"
            >
              Finish
              <HugeiconsIcon
                icon={ArrowRight01Icon}
                size={16}
                data-icon="inline-end"
              />
            </Button>
          </form>
        )}
      </div>

      <div className="hidden overflow-hidden rounded-2xl border border-border bg-card shadow-xl lg:block">
        <div className="flex h-[380px]">
          <div className="w-44 shrink-0 bg-[#3F0E40] px-3 py-3">
            <div className="mb-4 flex items-center gap-2">
              <SlackLogo size={20} />
              <div className="truncate text-sm font-bold text-white">
                {businessName?.trim() || currentOrg?.name || "Workspace"}
              </div>
            </div>
            {["general", "engineering", "design", "announcements"].map(
              (channel) => (
                <div
                  key={channel}
                  className={cn(
                    "flex items-center gap-2 rounded px-2 py-1 text-sm",
                    channel === "general"
                      ? "bg-[#1164A3] text-white"
                      : "text-white/60"
                  )}
                >
                  <span className="text-white/40">#</span>
                  <span className="truncate">{channel}</span>
                </div>
              )
            )}
          </div>
          <div className="flex flex-1 flex-col bg-white">
            <div className="border-b border-gray-200 px-5 py-3">
              <span className="text-base font-bold text-gray-900">
                #general
              </span>
            </div>
            <div className="flex-1 px-5 py-5">
              <div className="flex items-start gap-3 py-2">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary text-white">
                  <GhostLogo size={22} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-baseline gap-2">
                    <span className="text-sm font-bold text-gray-900">
                      hivy
                    </span>
                    <span className="rounded bg-gray-100 px-1 py-0 text-[10px] font-semibold text-gray-500">
                      APP
                    </span>
                    <span className="text-xs text-gray-400">9:41 AM</span>
                  </div>
                  <p className="mt-1 text-sm leading-relaxed text-gray-700">
                    I have your workspace context now. Mention me when you want
                    help with a task.
                  </p>
                </div>
              </div>
            </div>
            <div className="border-t border-gray-200 px-5 py-3">
              <div className="rounded-lg border border-gray-300 px-4 py-2.5 text-sm text-gray-400">
                Message #general
              </div>
            </div>
          </div>
        </div>
      </div>
    </motion.div>
  )
}

export default function OnboardingPage() {
  const onboarding = useOnboarding()

  return (
    <main className="relative flex min-h-screen flex-col items-center font-display text-foreground">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute top-0 left-1/2 h-160 w-160 -translate-x-1/2 rounded-full bg-(--glow-center) opacity-20 blur-[160px]" />
      </div>

      <div className="relative z-10 flex w-full flex-col items-center gap-6 px-4 pt-8 pb-6 sm:pt-12">
        <div className="flex items-center gap-2">
          <GhostLogo size={28} />
          <span className="font-heading text-lg font-medium tracking-tight text-foreground">
            hivy
          </span>
        </div>
        <StepIndicator current={onboarding.step} />
      </div>

      <div className="relative z-10 flex w-full flex-1 flex-col items-center justify-center px-4 py-8 sm:py-12">
        {onboarding.step === "slack" ? (
          <SlackStep
            slackConnected={onboarding.slackConnected}
            slackIntegration={onboarding.slackIntegration}
            connectingId={onboarding.connectingId}
            channels={onboarding.channels}
            isLoadingChannels={onboarding.isChannelsLoading}
            channelSearch={onboarding.channelSearch}
            selectedChannelIds={onboarding.selectedChannelIds}
            isJoiningChannels={onboarding.isJoiningChannels}
            onConnectSlack={onboarding.connectSlack}
            onSearchChannels={onboarding.setChannelSearch}
            onToggleChannel={onboarding.toggleChannel}
            onToggleAllChannels={onboarding.toggleAllFilteredChannels}
            onJoinSelectedChannels={onboarding.joinSelectedChannels}
            onJoinAllPublicChannels={onboarding.joinAllPublicChannels}
          />
        ) : null}

        {onboarding.step === "connections" ? (
          <ConnectionsStep
            integrations={onboarding.integrations}
            connectedIntegrationIds={onboarding.connectedIntegrationIds}
            nonSlackConnections={onboarding.nonSlackConnections}
            isLoading={onboarding.isIntegrationsLoading}
            connectingId={onboarding.connectingId}
            search={onboarding.integrationSearch}
            canContinue={onboarding.hasRequiredConnections}
            onSearch={onboarding.setIntegrationSearch}
            onConnect={onboarding.connectIntegration}
            onContinue={() => onboarding.setStep("business")}
          />
        ) : null}

        {onboarding.step === "business" ? (
          <BusinessStep
            currentOrg={onboarding.currentOrg}
            isLoading={onboarding.isCurrentOrgLoading}
            isSaving={onboarding.isFinishingBusinessProfile}
            onFinish={onboarding.finishBusinessProfile}
          />
        ) : null}
      </div>

      <div className="relative z-10 px-4 py-6 text-center text-xs text-muted-foreground/60">
        <p>
          Need help?{" "}
          <a
            href="mailto:hello@usehivy.com"
            className="text-muted-foreground underline underline-offset-2 transition-colors hover:text-foreground"
          >
            Contact support
          </a>
        </p>
      </div>

      <AddConnectionDialog
        open={onboarding.connectDialogOpen}
        onOpenChange={onboarding.setConnectDialogOpen}
        search={onboarding.connectDialogSearch}
        onSearchChange={onboarding.setConnectDialogSearch}
        connectingId={onboarding.connectingId}
        onConnect={(integrationId, options) =>
          onboarding.handleConnect(integrationId, options)
        }
        preSelectedIntegrationId={onboarding.preSelectedIntegrationId}
        onPreSelectedClear={() => onboarding.setPreSelectedIntegrationId(null)}
      />
    </main>
  )
}
