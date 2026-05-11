"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowLeft01Icon,
  CheckmarkCircle02Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { Button } from "@/components/ui/button"
import { integrationLogoURL } from "@/components/integration-logo"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

const GITHUB_PROVIDER = "github"

export function GithubStep() {
  const { goBack, goNext } = useOnboarding()
  const queryClient = useQueryClient()
  const [connecting, setConnecting] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const { data: integrations, isLoading: integrationsLoading } = $api.useQuery(
    "get",
    "/v1/in/integrations/available"
  )

  const githubIntegration = integrations?.find(
    (integration) => integration.provider === GITHUB_PROVIDER
  )
  const { data: connectionsData, isLoading: connectionsLoading } =
    $api.useQuery("get", "/v1/in/connections")
  const githubConnection = connectionsData?.data?.find(
    (connection) => connection.provider === GITHUB_PROVIDER
  )

  async function handleConnect() {
    if (!githubIntegration?.id || connecting) return

    setConnecting(true)
    setErrorMessage(null)

    try {
      const session = await api.POST(
        "/v1/in/integrations/{id}/connect-session",
        {
          params: { path: { id: githubIntegration.id } },
        }
      )
      if (session.error) throw new Error("Failed to create session")

      const { token, provider_config_key: providerConfigKey } =
        session.data as {
          token: string
          provider_config_key: string
        }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      const authResult = await nango.auth(providerConfigKey)
      const connection = await api.POST(
        "/v1/in/integrations/{id}/connections",
        {
          params: { path: { id: githubIntegration.id } },
          body: { nango_connection_id: authResult.connectionId } as never,
        }
      )
      if (connection.error) throw new Error("Failed to save GitHub connection")

      await queryClient.invalidateQueries({
        queryKey: ["get", "/v1/in/connections"],
      })
      goNext()
    } catch (error) {
      if (error instanceof AuthError && error.type === "window_closed") {
        return
      }
      setErrorMessage(
        extractErrorMessage(error, "Could not connect GitHub. Try again.")
      )
    } finally {
      setConnecting(false)
    }
  }

  const loading = integrationsLoading || connectionsLoading
  const choiceDisabled = connecting || loading || !githubIntegration

  return (
    <div className="mx-auto flex w-full max-w-md flex-col items-center gap-8 text-center">
      <StepHeader
        title="Connect GitHub Profile"
        description="Authorize the GitHub identity your first employee will use. Repository selection comes next."
      />

      <div className="flex w-full flex-col gap-3">
        <ChoiceCard
          logoUrl={integrationLogoURL(GITHUB_PROVIDER)}
          logoSize={32}
          title={githubConnection ? "GitHub Profile connected" : "Connect GitHub Profile"}
          description={
            githubConnection
              ? "Your employee can use this GitHub identity once repository access is configured."
              : "Sign in with GitHub so Hiveloop can request repository access for this employee."
          }
          onClick={
            githubConnection ? goNext : choiceDisabled ? () => {} : handleConnect
          }
          trailing={
            connecting ? (
              <HugeiconsIcon
                icon={Loading03Icon}
                className="size-4 shrink-0 animate-spin text-muted-foreground"
                strokeWidth={2}
              />
            ) : githubConnection ? (
              <HugeiconsIcon
                icon={CheckmarkCircle02Icon}
                className="size-5 shrink-0 text-emerald-600"
                strokeWidth={2}
              />
            ) : undefined
          }
        />
        {!githubIntegration && !loading ? (
          <p className="text-left text-[13px] leading-relaxed text-muted-foreground">
            GitHub Profile is not available in this workspace yet. The backend
            integration must be enabled before this step can complete.
          </p>
        ) : null}
      </div>

      {errorMessage ? (
        <div className="flex w-full items-start gap-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-left text-[13px] text-destructive">
          <HugeiconsIcon
            icon={Alert02Icon}
            className="mt-0.5 size-4 shrink-0"
            strokeWidth={2}
          />
          <span className="leading-relaxed">{errorMessage}</span>
        </div>
      ) : null}

      <div className="flex w-full items-center justify-between">
        <Button
          variant="ghost"
          onClick={goBack}
          disabled={connecting}
          className="gap-2"
        >
          <HugeiconsIcon icon={ArrowLeft01Icon} className="size-4" />
          Back
        </Button>
        {githubConnection ? (
          <Button onClick={goNext} disabled={connecting}>
            Continue
          </Button>
        ) : null}
      </div>
    </div>
  )
}
