"use client"

import { useMemo } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import { Plug01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { OnboardingShell } from "./_components/onboarding-shell"
import { useConnectIntegration } from "@/app/w/connections/_hooks/use-connect-integration"

export default function OnboardingPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const integrationsQuery = $api.useQuery("get", "/v1/in/integrations/available")
  const { connect, connectingId } = useConnectIntegration()

  const slackIntegration = useMemo(
    () =>
      (integrationsQuery.data ?? []).find(
        (integration) => integration.provider === "slack"
      ),
    [integrationsQuery.data]
  )

  function handleInstallSlack() {
    if (!slackIntegration?.id) return
    connect(slackIntegration.id, {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
        router.replace("/w")
      },
    })
  }

  return (
    <OnboardingShell>
      <main className="mx-auto flex min-h-[calc(100vh-4rem)] w-full max-w-md flex-col justify-center px-6 py-16">
        <div className="flex flex-col items-center gap-7 text-center">
          <div className="flex size-14 items-center justify-center rounded-lg border border-border bg-muted/40">
            <HugeiconsIcon icon={Plug01Icon} className="size-6 text-foreground" aria-hidden />
          </div>
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-normal">
              Connect Slack
            </h1>
            <p className="text-sm leading-6 text-muted-foreground">
              Hivy is already set up for your workspace. Install the Slack app
              to finish onboarding.
            </p>
          </div>
          {integrationsQuery.isLoading ? (
            <Skeleton className="h-10 w-44 rounded-md" />
          ) : (
            <Button
              className="w-full"
              onClick={handleInstallSlack}
              disabled={!slackIntegration?.id}
              loading={connectingId === slackIntegration?.id}
            >
              Install Slack app
            </Button>
          )}
        </div>
      </main>
    </OnboardingShell>
  )
}
