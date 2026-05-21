"use client"

import { useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
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
import { useConnectIntegration } from "@/app/w/connections/_hooks/use-connect-integration"
import { IntegrationLogo } from "@/components/integration-logo"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import { OnboardingShell } from "./_components/onboarding-shell"
import type { components } from "@/lib/api/schema"

type Step = "slack" | "channels" | "tools"
type Integration = components["schemas"]["inIntegrationAvailableResponse"]
type Channel = components["schemas"]["slackChannelResponse"]
type ConnectionConfigField = components["schemas"]["ConnectionConfigField"]

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

function needsForm(integration: Integration): boolean {
  const authMode = integration.nango_config?.auth_mode
  const installation = integration.nango_config?.installation
  if (authMode === "API_KEY" || authMode === "BASIC") return true
  if (installation === "outbound") return true

  const connectionConfig = integration.nango_config?.connection_config as
    | Record<string, ConnectionConfigField>
    | undefined
  return Boolean(
    connectionConfig &&
      Object.values(connectionConfig).some((field) => !field.automated)
  )
}

export default function OnboardingPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const [step, setStep] = useState<Step>("slack")
  const [channelSearch, setChannelSearch] = useState("")
  const [selectedChannelIds, setSelectedChannelIds] = useState<Set<string>>(
    () => new Set()
  )
  const [connectOpen, setConnectOpen] = useState(false)
  const [connectSearch, setConnectSearch] = useState("")
  const [preSelectedIntegrationId, setPreSelectedIntegrationId] =
    useState<string | null>(null)

  const integrationsQuery = $api.useQuery("get", "/v1/in/integrations/available")
  const connectionsQuery = $api.useQuery("get", "/v1/in/connections")
  const joinChannels = $api.useMutation("post", "/v1/slack/channels/join")
  const { connect, connectingId } = useConnectIntegration()

  const connections = connectionsQuery.data?.data ?? []
  const integrations = integrationsQuery.data ?? []
  const slackIntegration = integrations.find(
    (integration) => integration.provider === "slack"
  )
  const slackConnected = connections.some(
    (connection) => connection.provider === "slack"
  )
  const nonSlackConnections = connections.filter(
    (connection) => connection.provider !== "slack"
  ).length

  const channelsQuery = $api.useQuery(
    "get",
    "/v1/slack/channels",
    {},
    { enabled: slackConnected }
  )
  const channels = channelsQuery.data?.channels ?? []
  const hasAvailableChannel = channels.some(
    (channel) => !channel.is_private && channel.is_member
  )

  useEffect(() => {
    if (connectionsQuery.isLoading) return
    if (!slackConnected) {
      setStep("slack")
      return
    }
    if (channelsQuery.isLoading) return
    setStep((current) =>
      current === "tools" && hasAvailableChannel ? "tools" : "channels"
    )
  }, [
    channelsQuery.isLoading,
    connectionsQuery.isLoading,
    hasAvailableChannel,
    slackConnected,
  ])

  const filteredChannels = useMemo(() => {
    const query = channelSearch.trim().toLowerCase()
    if (!query) return channels
    return channels.filter((channel) =>
      (channel.name ?? "").toLowerCase().includes(query)
    )
  }, [channelSearch, channels])

  const connectedIntegrationIds = useMemo(
    () =>
      new Set(
        connections
          .map((connection) => connection.in_integration_id)
          .filter(Boolean)
      ),
    [connections]
  )

  async function refreshOnboardingData() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/dashboard"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/slack/channels"] }),
    ])
  }

  function handleConnect(
    integrationId: string,
    options?: ConnectOptions,
    next?: Step
  ) {
    connect(integrationId, {
      ...options,
      onSuccess: async () => {
        await refreshOnboardingData()
        setConnectOpen(false)
        setConnectSearch("")
        setPreSelectedIntegrationId(null)
        if (next) setStep(next)
      },
    })
  }

  function handleIntegrationClick(integration: Integration) {
    if (!integration.id) return
    if (needsForm(integration)) {
      setPreSelectedIntegrationId(integration.id)
      setConnectOpen(true)
      return
    }
    handleConnect(integration.id)
  }

  function handleJoinChannels(body: { all_public?: boolean; channel_ids?: string[] }) {
    joinChannels.mutate(
      { body },
      {
        onSuccess: async (response) => {
          const available =
            (response?.joined ?? 0) + (response?.already_member ?? 0)
          const selectedPublicMember = channels.some(
            (channel) =>
              !channel.is_private &&
              channel.is_member &&
              channel.id !== undefined &&
              selectedChannelIds.has(channel.id)
          )
          const publicAvailable =
            (body.all_public === true && hasAvailableChannel) ||
            (response?.joined ?? 0) > 0 ||
            selectedPublicMember
          await refreshOnboardingData()
          if (available > 0 && publicAvailable) {
            toast.success("Hivy can now work in Slack channels")
            setSelectedChannelIds(new Set())
            setStep("tools")
            return
          }
          toast.error("No channels were joined. Try selecting channels manually.")
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to join Slack channels"))
        },
      }
    )
  }

  function toggleChannel(channelID: string | undefined) {
    if (!channelID) return
    setSelectedChannelIds((current) => {
      const next = new Set(current)
      if (next.has(channelID)) next.delete(channelID)
      else next.add(channelID)
      return next
    })
  }

  return (
    <OnboardingShell>
      <div className="w-full max-w-5xl py-8">
        <div className="mb-8 flex flex-col gap-6 md:flex-row md:items-end md:justify-between">
          <div className="max-w-2xl">
            <p className="text-sm font-medium text-primary">Set up Hivy</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-normal text-foreground">
              Connect Slack, invite Hivy, then add your tools.
            </h1>
            <p className="mt-3 text-sm leading-6 text-muted-foreground">
              Slack is required. Extra tools are optional now and make Hivy more
              useful once your workspace opens.
            </p>
          </div>
          <StepRail current={step} slackConnected={slackConnected} />
        </div>

        <div className="border border-border bg-card">
          {step === "slack" ? (
            <SlackStep
              isLoading={integrationsQuery.isLoading}
              slackIntegration={slackIntegration}
              connectingId={connectingId}
              onInstall={() => {
                if (!slackIntegration?.id) return
                handleConnect(slackIntegration.id, undefined, "channels")
              }}
            />
          ) : null}
          {step === "channels" ? (
            <ChannelsStep
              channels={filteredChannels}
              isLoading={channelsQuery.isLoading}
              search={channelSearch}
              selectedIds={selectedChannelIds}
              joining={joinChannels.isPending}
              onSearch={setChannelSearch}
              onToggle={toggleChannel}
              onJoinAll={() => handleJoinChannels({ all_public: true })}
              onJoinSelected={() =>
                handleJoinChannels({
                  channel_ids: Array.from(selectedChannelIds),
                })
              }
            />
          ) : null}
          {step === "tools" ? (
            <ToolsStep
              integrations={integrations}
              connectedIntegrationIds={connectedIntegrationIds}
              nonSlackConnections={nonSlackConnections}
              isLoading={integrationsQuery.isLoading || connectionsQuery.isLoading}
              connectingId={connectingId}
              onConnect={handleIntegrationClick}
              onSkip={() => router.replace("/w")}
            />
          ) : null}
        </div>
      </div>

      <AddConnectionDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        search={connectSearch}
        onSearchChange={setConnectSearch}
        connectingId={connectingId}
        onConnect={(integrationId, options) => handleConnect(integrationId, options)}
        preSelectedIntegrationId={preSelectedIntegrationId}
        onPreSelectedClear={() => setPreSelectedIntegrationId(null)}
      />
    </OnboardingShell>
  )
}

function StepRail({
  current,
  slackConnected,
}: {
  current: Step
  slackConnected: boolean
}) {
  const items: Array<{ id: Step; label: string }> = [
    { id: "slack", label: "Slack" },
    { id: "channels", label: "Channels" },
    { id: "tools", label: "Tools" },
  ]

  return (
    <div className="flex min-w-64 items-center gap-2">
      {items.map((item, index) => {
        const active = item.id === current
        const done =
          (item.id === "slack" && slackConnected) ||
          items.findIndex((candidate) => candidate.id === current) > index
        return (
          <div key={item.id} className="flex flex-1 items-center gap-2">
            <div
              className={cn(
                "flex size-8 items-center justify-center rounded-full border text-xs font-medium",
                active
                  ? "border-primary bg-primary text-primary-foreground"
                  : done
                    ? "border-primary/30 bg-primary/10 text-primary"
                    : "border-border text-muted-foreground"
              )}
            >
              {done ? (
                <HugeiconsIcon icon={CheckmarkCircle01Icon} size={16} />
              ) : (
                index + 1
              )}
            </div>
            <span className="hidden text-sm text-muted-foreground sm:inline">
              {item.label}
            </span>
          </div>
        )
      })}
    </div>
  )
}

function SlackStep({
  isLoading,
  slackIntegration,
  connectingId,
  onInstall,
}: {
  isLoading: boolean
  slackIntegration?: Integration
  connectingId: string | null
  onInstall: () => void
}) {
  return (
    <div className="grid gap-8 p-6 md:grid-cols-[1fr_320px] md:p-8">
      <div className="flex flex-col justify-center">
        <div className="flex size-12 items-center justify-center rounded-lg border border-border bg-muted/40">
          <HugeiconsIcon icon={SlackIcon} className="size-6 text-foreground" />
        </div>
        <h2 className="mt-6 text-2xl font-semibold tracking-normal">
          Install the Slack app
        </h2>
        <p className="mt-3 max-w-xl text-sm leading-6 text-muted-foreground">
          Hivy works from Slack first. Installing the app creates the workspace
          connection and unlocks the rest of onboarding.
        </p>
        <div className="mt-6">
          {isLoading ? (
            <Skeleton className="h-10 w-44" />
          ) : (
            <Button
              onClick={onInstall}
              disabled={!slackIntegration?.id}
              loading={connectingId === slackIntegration?.id}
            >
              Install Slack app
              <HugeiconsIcon icon={ArrowRight01Icon} size={16} data-icon="inline-end" />
            </Button>
          )}
        </div>
      </div>
      <div className="border border-border bg-muted/25 p-5">
        <p className="text-sm font-medium text-foreground">What happens next</p>
        <div className="mt-4 space-y-4 text-sm text-muted-foreground">
          <p>Hivy is already created for this workspace.</p>
          <p>You choose which Slack channels Hivy can join.</p>
          <p>Extra connections can be added before you enter the dashboard.</p>
        </div>
      </div>
    </div>
  )
}

function ChannelsStep({
  channels,
  isLoading,
  search,
  selectedIds,
  joining,
  onSearch,
  onToggle,
  onJoinAll,
  onJoinSelected,
}: {
  channels: Channel[]
  isLoading: boolean
  search: string
  selectedIds: Set<string>
  joining: boolean
  onSearch: (value: string) => void
  onToggle: (id: string | undefined) => void
  onJoinAll: () => void
  onJoinSelected: () => void
}) {
  return (
    <div className="grid gap-8 p-6 md:grid-cols-[340px_1fr] md:p-8">
      <div>
        <div className="flex size-12 items-center justify-center rounded-lg border border-border bg-muted/40">
          <HugeiconsIcon icon={Plug01Icon} className="size-6 text-foreground" />
        </div>
        <h2 className="mt-6 text-2xl font-semibold tracking-normal">
          Invite Hivy into Slack channels
        </h2>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          The fastest path is inviting Hivy into every public channel. Private
          channels appear only after Hivy is already a member.
        </p>
        <div className="mt-6 flex flex-col gap-3">
          <Button onClick={onJoinAll} loading={joining}>
            Invite all public channels
          </Button>
          <Button
            variant="outline"
            onClick={onJoinSelected}
            loading={joining}
            disabled={selectedIds.size === 0}
          >
            Invite selected channels
          </Button>
        </div>
      </div>
      <div className="min-w-0 border border-border bg-background">
        <div className="border-b border-border p-4">
          <div className="relative">
            <HugeiconsIcon
              icon={Search01Icon}
              className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={search}
              onChange={(event) => onSearch(event.target.value)}
              placeholder="Search channels"
              className="pl-9"
            />
          </div>
        </div>
        <ScrollArea className="h-[420px]">
          {isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className="h-14 w-full" />
              ))}
            </div>
          ) : channels.length === 0 ? (
            <div className="flex h-48 items-center justify-center p-6 text-center text-sm text-muted-foreground">
              No available channels found.
            </div>
          ) : (
            <div className="divide-y divide-border">
              {channels.map((channel) => {
                const selectable = !channel.is_private
                return (
                  <label
                    key={channel.id}
                    className={cn(
                      "flex items-center gap-3 px-4 py-3",
                      selectable
                        ? "cursor-pointer"
                        : "cursor-not-allowed opacity-70"
                    )}
                  >
                    <Checkbox
                      checked={Boolean(channel.id && selectedIds.has(channel.id))}
                      disabled={!selectable}
                      onCheckedChange={() => {
                        if (selectable) onToggle(channel.id)
                      }}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        #{channel.name}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {channel.is_private ? "Private" : "Public"}
                        {channel.is_member ? " · Hivy is already in this channel" : ""}
                      </span>
                    </span>
                    {channel.is_member ? (
                      <Badge variant="outline">Available</Badge>
                    ) : null}
                  </label>
                )
              })}
            </div>
          )}
        </ScrollArea>
      </div>
    </div>
  )
}

function ToolsStep({
  integrations,
  connectedIntegrationIds,
  nonSlackConnections,
  isLoading,
  connectingId,
  onConnect,
  onSkip,
}: {
  integrations: Integration[]
  connectedIntegrationIds: Set<string | undefined>
  nonSlackConnections: number
  isLoading: boolean
  connectingId: string | null
  onConnect: (integration: Integration) => void
  onSkip: () => void
}) {
  const progress = Math.min(nonSlackConnections, 3)

  return (
    <div className="p-6 md:p-8">
      <div className="flex flex-col gap-6 md:flex-row md:items-end md:justify-between">
        <div>
          <h2 className="text-2xl font-semibold tracking-normal">
            Add more tools
          </h2>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
            Hivy already has Slack. Three more active connections gives Hivy
            enough context for the dashboard to feel complete.
          </p>
        </div>
        <div className="w-full max-w-xs">
          <div className="mb-2 flex items-center justify-between text-xs text-muted-foreground">
            <span>{progress} of 3 tools connected</span>
            <span>{Math.round((progress / 3) * 100)}%</span>
          </div>
          <Progress value={progress} max={3} />
        </div>
      </div>

      <div className="mt-8">
        {isLoading ? (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-20 w-full" />
            ))}
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {integrations.map((integration) => {
              const connected = connectedIntegrationIds.has(integration.id)
              const connecting = connectingId === integration.id
              return (
                <button
                  key={integration.id}
                  type="button"
                  disabled={connected || connectingId !== null}
                  onClick={() => onConnect(integration)}
                  className="flex min-h-20 items-center gap-3 border border-border bg-background p-4 text-left transition-colors hover:border-primary disabled:cursor-not-allowed disabled:opacity-70"
                >
                  <IntegrationLogo provider={integration.provider ?? ""} size={28} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {integration.display_name}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {connected ? "Connected" : "Connect to workspace"}
                    </span>
                  </span>
                  {connected ? (
                    <Badge variant="outline">Connected</Badge>
                  ) : connecting ? (
                    <HugeiconsIcon
                      icon={Loading03Icon}
                      className="size-4 animate-spin text-muted-foreground"
                    />
                  ) : null}
                </button>
              )
            })}
          </div>
        )}
      </div>

      <div className="mt-8 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
        <Button variant="outline" onClick={onSkip}>
          Skip for now
        </Button>
        <Button onClick={onSkip}>
          Continue to dashboard
          <HugeiconsIcon icon={ArrowRight01Icon} size={16} data-icon="inline-end" />
        </Button>
      </div>
    </div>
  )
}
