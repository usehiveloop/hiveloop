"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowLeft01Icon,
  CheckmarkCircle02Icon,
  LockIcon,
  Loading03Icon,
  RepositoryIcon,
} from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { integrationLogoURL } from "@/components/integration-logo"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

const GITHUB_PROVIDER = "github"

interface GitHubRepository {
  id: string
  node_id?: string
  name: string
  full_name: string
  private: boolean
  html_url?: string
  description?: string
  owner?: string
}

interface GitHubRepositoriesResponse {
  repositories: GitHubRepository[]
  selected_repositories: GitHubRepository[]
}

async function backendJSON<T>(
  path: string,
  init?: RequestInit & { body?: BodyInit | null }
): Promise<T> {
  const response = await fetch(`/api/proxy${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  })
  const data = await response.json().catch(() => null)
  if (!response.ok) {
    throw new Error(
      data && typeof data === "object" && "error" in data
        ? String(data.error)
        : `Request failed with status ${response.status}`
    )
  }
  return data as T
}

export function GithubStep() {
  const { createEmployee, goBack, goNext } = useOnboarding()
  const queryClient = useQueryClient()
  const [connecting, setConnecting] = useState(false)
  const [loadingRepositories, setLoadingRepositories] = useState(false)
  const [savingRepositories, setSavingRepositories] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [repositoryDialogOpen, setRepositoryDialogOpen] = useState(false)
  const [repositories, setRepositories] = useState<GitHubRepository[]>([])
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [hasSavedRepositories, setHasSavedRepositories] = useState(false)

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
  const agentId = createEmployee.agentId

  async function attachProfileAndLoadRepositories(connectionId: string) {
    if (!agentId) {
      throw new Error("Create the employee profile before connecting GitHub.")
    }
    await backendJSON(`/v1/agents/${agentId}/profiles/github`, {
      method: "POST",
      body: JSON.stringify({ connection_id: connectionId }),
    })
    const data = await backendJSON<GitHubRepositoriesResponse>(
      `/v1/agents/${agentId}/profiles/github/repositories`
    )
    setRepositories(data.repositories ?? [])
    const selected = data.selected_repositories ?? []
    setSelectedIDs(new Set(selected.map((repo) => repo.id)))
    setHasSavedRepositories(selected.length > 0)
    setRepositoryDialogOpen(true)
  }

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

      const connectionID = connection.data?.id
      if (!connectionID) throw new Error("GitHub connection was saved without an id")
      await attachProfileAndLoadRepositories(connectionID)
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

  async function handleSelectRepositories() {
    if (!githubConnection?.id || loadingRepositories) return
    setLoadingRepositories(true)
    setErrorMessage(null)
    try {
      await attachProfileAndLoadRepositories(githubConnection.id)
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Could not load GitHub repositories. Try again.")
      )
    } finally {
      setLoadingRepositories(false)
    }
  }

  function toggleRepository(repoID: string, checked: boolean) {
    setSelectedIDs((current) => {
      const next = new Set(current)
      if (checked) next.add(repoID)
      else next.delete(repoID)
      return next
    })
  }

  async function handleSaveRepositories() {
    if (!agentId) return
    const selected = repositories.filter((repo) => selectedIDs.has(repo.id))
    if (selected.length === 0) {
      setErrorMessage("Select at least one repository for this employee.")
      return
    }
    setSavingRepositories(true)
    setErrorMessage(null)
    try {
      await backendJSON(`/v1/agents/${agentId}/profiles/github/repositories`, {
        method: "PATCH",
        body: JSON.stringify({ repositories: selected }),
      })
      setHasSavedRepositories(true)
      setRepositoryDialogOpen(false)
      goNext()
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Could not save repository access. Try again.")
      )
    } finally {
      setSavingRepositories(false)
    }
  }

  const loading = integrationsLoading || connectionsLoading
  const busy = connecting || loadingRepositories || savingRepositories
  const choiceDisabled = busy || loading || !githubIntegration

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
          title={
            hasSavedRepositories
              ? "GitHub repositories selected"
              : githubConnection
                ? "Select GitHub repositories"
                : "Connect GitHub Profile"
          }
          description={
            hasSavedRepositories
              ? "Repository access is configured for this employee."
              : githubConnection
                ? "Choose the repositories this employee can inspect and work in."
              : "Sign in with GitHub so Hiveloop can request repository access for this employee."
          }
          onClick={
            githubConnection
              ? choiceDisabled
                ? () => {}
                : handleSelectRepositories
              : choiceDisabled
                ? () => {}
                : handleConnect
          }
          trailing={
            connecting || loadingRepositories ? (
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
        {hasSavedRepositories ? (
          <Button onClick={goNext} disabled={busy}>
            Continue
          </Button>
        ) : null}
      </div>

      <RepositoryPickerDialog
        open={repositoryDialogOpen}
        onOpenChange={setRepositoryDialogOpen}
        repositories={repositories}
        selectedIDs={selectedIDs}
        onToggle={toggleRepository}
        onSave={handleSaveRepositories}
        saving={savingRepositories}
      />
    </div>
  )
}

function RepositoryPickerDialog({
  open,
  onOpenChange,
  repositories,
  selectedIDs,
  onToggle,
  onSave,
  saving,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  repositories: GitHubRepository[]
  selectedIDs: Set<string>
  onToggle: (repoID: string, checked: boolean) => void
  onSave: () => void
  saving: boolean
}) {
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!saving) onOpenChange(nextOpen)
      }}
    >
      <DialogContent
        className="flex h-[min(680px,85vh)] flex-col p-6 sm:max-w-lg"
        showCloseButton={!saving}
      >
        <DialogHeader>
          <DialogTitle>Choose repositories</DialogTitle>
          <DialogDescription className="mt-2">
            Pick the repositories this employee can use. You can adjust this later.
          </DialogDescription>
        </DialogHeader>

        <div className="mt-4 min-h-0 flex-1 overflow-y-auto">
          {repositories.length === 0 ? (
            <div className="flex h-full min-h-48 flex-col items-center justify-center gap-2 text-center">
              <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                <HugeiconsIcon
                  icon={RepositoryIcon}
                  size={20}
                  className="text-muted-foreground"
                />
              </div>
              <p className="text-sm font-medium text-foreground">
                No repositories found
              </p>
              <p className="max-w-sm text-[13px] leading-relaxed text-muted-foreground">
                This GitHub profile did not return any repositories. Confirm the
                connected account has repository access and try again.
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              {repositories.map((repo) => {
                const checked = selectedIDs.has(repo.id)
                return (
                  <ChoiceCard
                    key={repo.id}
                    icon={RepositoryIcon}
                    title={repo.full_name}
                    description={
                      repo.description || repo.owner || "GitHub repository"
                    }
                    selected={checked}
                    onClick={() => onToggle(repo.id, !checked)}
                    trailing={
                      <RepositoryChoiceTrailing
                        isPrivate={repo.private}
                        selected={checked}
                      />
                    }
                  />
                )
              })}
            </div>
          )}
        </div>

        <DialogFooter className="mt-4 sm:items-center sm:justify-between">
          <p className="text-[13px] text-muted-foreground">
            {selectedIDs.size} selected
          </p>
          <Button
            onClick={onSave}
            disabled={saving || selectedIDs.size === 0}
            className="min-w-32"
          >
            {saving ? "Saving..." : "Save and continue"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RepositoryChoiceTrailing({
  isPrivate,
  selected,
}: {
  isPrivate: boolean
  selected: boolean
}) {
  return (
    <span className="mt-0.5 flex shrink-0 items-center gap-2">
      {isPrivate ? (
        <HugeiconsIcon
          icon={LockIcon}
          className="size-4 text-muted-foreground"
          strokeWidth={2}
          aria-label="Private repository"
        />
      ) : null}
      {selected ? (
        <HugeiconsIcon
          icon={CheckmarkCircle02Icon}
          className="size-5 text-emerald-600"
          strokeWidth={2}
        />
      ) : null}
    </span>
  )
}
