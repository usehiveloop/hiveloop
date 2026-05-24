"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { useConnectIntegration } from "@/app/w-old/connections/_hooks/use-connect-integration"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import type { components } from "@/lib/api/schema"

export type OnboardingStep = "slack" | "connections" | "business"
export type Integration =
  components["schemas"]["integrationAvailableResponse"]
export type Channel = components["schemas"]["slackChannelResponse"]
export type ConnectionConfigField =
  components["schemas"]["ConnectionConfigField"]
export type OrgUpdateRequest = components["schemas"]["updateOrgRequest"]

const REQUIRED_CONNECTIONS_FOR_BUSINESS_STEP = 3

interface ConnectOptions {
  credentials?: Record<string, string>
  params?: Record<string, string>
  installation?: "outbound"
}

export function needsConnectionForm(integration: Integration): boolean {
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

export function useOnboarding() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const { activeOrg } = useAuth()
  const [step, setStep] = useState<OnboardingStep>("slack")
  const [channelSearch, setChannelSearch] = useState("")
  const [selectedChannelIds, setSelectedChannelIds] = useState<Set<string>>(
    () => new Set()
  )
  const [integrationSearch, setIntegrationSearch] = useState("")
  const [connectDialogOpen, setConnectDialogOpen] = useState(false)
  const [connectDialogSearch, setConnectDialogSearch] = useState("")
  const [preSelectedIntegrationId, setPreSelectedIntegrationId] = useState<
    string | null
  >(null)

  const currentOrgQuery = $api.useQuery("get", "/v1/orgs/current")
  const integrationsQuery = $api.useQuery(
    "get",
    "/v1/integrations/available"
  )
  const connectionsQuery = $api.useQuery("get", "/v1/connections")
  const joinChannelsMutation = $api.useMutation(
    "post",
    "/v1/slack/channels/join"
  )
  const updateOrgMutation = $api.useMutation("patch", "/v1/orgs/current")
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
  )
  const hasRequiredConnections =
    connections.length >= REQUIRED_CONNECTIONS_FOR_BUSINESS_STEP

  const channelsQuery = $api.useQuery(
    "get",
    "/v1/slack/channels",
    {},
    { enabled: slackConnected }
  )
  const channels = channelsQuery.data?.channels ?? []

  const selectableChannels = useMemo(
    () => channels.filter((channel) => !channel.is_private),
    [channels]
  )
  const slackChannelsJoined = useMemo(
    () =>
      channels.some((channel) => !channel.is_private && channel.is_member),
    [channels]
  )

  useEffect(() => {
    if (!slackConnected) return
    setSelectedChannelIds((current) => {
      if (current.size > 0) return current
      return new Set(
        selectableChannels
          .map((channel) => channel.id)
          .filter((id): id is string => Boolean(id))
      )
    })
  }, [selectableChannels, slackConnected])

  useEffect(() => {
    if (activeOrg?.onboarded) {
      router.replace("/w")
      return
    }
    if (connectionsQuery.isLoading) return
    if (!slackConnected) {
      setStep("slack")
      return
    }
    if (channelsQuery.isLoading) return
    if (!slackChannelsJoined) {
      setStep("slack")
      return
    }
    if (hasRequiredConnections) {
      setStep("business")
      return
    }
    setStep("connections")
  }, [
    activeOrg?.onboarded,
    channelsQuery.isLoading,
    connectionsQuery.isLoading,
    hasRequiredConnections,
    router,
    slackChannelsJoined,
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
          .map((connection) => connection.integration_id)
          .filter(Boolean)
      ),
    [connections]
  )

  const filteredIntegrations = useMemo(() => {
    const query = integrationSearch.trim().toLowerCase()
    return integrations
      .filter((integration) => integration.provider !== "slack")
      .filter((integration) => {
        if (!query) return true
        return (
          (integration.display_name ?? "").toLowerCase().includes(query) ||
          (integration.provider ?? "").toLowerCase().includes(query)
        )
      })
  }, [integrationSearch, integrations])

  const refreshOnboardingData = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/orgs/current"] }),
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/connections"],
      }),
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/dashboard"] }),
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/slack/channels"],
      }),
    ])
  }, [queryClient])

  const handleConnect = useCallback(
    (
      integrationId: string,
      options?: ConnectOptions,
      next?: OnboardingStep
    ) => {
      connect(integrationId, {
        ...options,
        onSuccess: async () => {
          await refreshOnboardingData()
          setConnectDialogOpen(false)
          setConnectDialogSearch("")
          setPreSelectedIntegrationId(null)
          if (next) setStep(next)
        },
      })
    },
    [connect, refreshOnboardingData]
  )

  const connectSlack = useCallback(() => {
    if (!slackIntegration?.id) return
    handleConnect(slackIntegration.id)
  }, [handleConnect, slackIntegration?.id])

  const connectIntegration = useCallback(
    (integration: Integration) => {
      if (!integration.id) return
      if (needsConnectionForm(integration)) {
        setPreSelectedIntegrationId(integration.id)
        setConnectDialogOpen(true)
        return
      }
      handleConnect(integration.id)
    },
    [handleConnect]
  )

  const toggleChannel = useCallback((channelID: string | undefined) => {
    if (!channelID) return
    setSelectedChannelIds((current) => {
      const next = new Set(current)
      if (next.has(channelID)) next.delete(channelID)
      else next.add(channelID)
      return next
    })
  }, [])

  const toggleAllFilteredChannels = useCallback(() => {
    const filteredSelectable = filteredChannels
      .filter((channel) => !channel.is_private)
      .map((channel) => channel.id)
      .filter((id): id is string => Boolean(id))

    setSelectedChannelIds((current) => {
      const allSelected = filteredSelectable.every((id) => current.has(id))
      if (allSelected) {
        const next = new Set(current)
        filteredSelectable.forEach((id) => next.delete(id))
        return next
      }
      return new Set([...current, ...filteredSelectable])
    })
  }, [filteredChannels])

  const joinSelectedChannels = useCallback(() => {
    if (selectedChannelIds.size === 0) return

    joinChannelsMutation.mutate(
      { body: { channel_ids: Array.from(selectedChannelIds) } },
      {
        onSuccess: async (response) => {
          const available =
            (response?.joined ?? 0) + (response?.already_member ?? 0)
          const failed = response?.failed ?? 0
          await refreshOnboardingData()
          if (available > 0 && failed === 0) {
            toast.success("Hivy can now work in Slack channels")
            setStep("connections")
            return
          }
          if (failed > 0) {
            toast.error("Some selected channels could not be joined.")
            return
          }
          toast.error("No channels were joined. Try selecting public channels.")
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to join Slack channels")
          )
        },
      }
    )
  }, [joinChannelsMutation, refreshOnboardingData, selectedChannelIds])

  const joinAllPublicChannels = useCallback(() => {
    joinChannelsMutation.mutate(
      { body: { all_public: true } },
      {
        onSuccess: async (response) => {
          const available =
            (response?.joined ?? 0) + (response?.already_member ?? 0)
          const failed = response?.failed ?? 0
          await refreshOnboardingData()
          if (available > 0 && failed === 0) {
            toast.success("Hivy can now work in Slack channels")
            setStep("connections")
            return
          }
          if (failed > 0) {
            toast.error("Some public channels could not be joined.")
            return
          }
          toast.error("No public channels were available to join.")
        },
        onError: (error) => {
          toast.error(
            extractErrorMessage(error, "Failed to join Slack channels")
          )
        },
      }
    )
  }, [joinChannelsMutation, refreshOnboardingData])

  const finishBusinessProfile = useCallback(
    (body: OrgUpdateRequest) => {
      updateOrgMutation.mutate(
        { body },
        {
          onSuccess: async () => {
            await refreshOnboardingData()
            router.replace("/w")
          },
          onError: (error) => {
            toast.error(
              extractErrorMessage(error, "Failed to save workspace profile")
            )
          },
        }
      )
    },
    [refreshOnboardingData, router, updateOrgMutation]
  )

  return {
    step,
    setStep,
    currentOrg: currentOrgQuery.data,
    isCurrentOrgLoading: currentOrgQuery.isLoading,
    integrations: filteredIntegrations,
    isIntegrationsLoading: integrationsQuery.isLoading,
    connections,
    connectedIntegrationIds,
    nonSlackConnections,
    hasRequiredConnections,
    slackIntegration,
    slackConnected,
    connectSlack,
    connectIntegration,
    connectingId,
    integrationSearch,
    setIntegrationSearch,
    channels: filteredChannels,
    isChannelsLoading: channelsQuery.isLoading,
    channelSearch,
    setChannelSearch,
    selectedChannelIds,
    toggleChannel,
    toggleAllFilteredChannels,
    joinSelectedChannels,
    joinAllPublicChannels,
    isJoiningChannels: joinChannelsMutation.isPending,
    finishBusinessProfile,
    isFinishingBusinessProfile: updateOrgMutation.isPending,
    connectDialogOpen,
    setConnectDialogOpen,
    connectDialogSearch,
    setConnectDialogSearch,
    preSelectedIntegrationId,
    setPreSelectedIntegrationId,
    handleConnect,
  }
}
