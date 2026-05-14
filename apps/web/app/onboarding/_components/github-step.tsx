"use client"

import { useState, type MouseEvent } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowLeft01Icon,
  CheckmarkCircle02Icon,
  LockIcon,
  Loading03Icon,
  RefreshIcon,
  RepositoryIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { toast } from "sonner"
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
import type { components } from "@/lib/api/schema"
import { StepHeader } from "./step-header"
import { useOnboarding } from "./context"

const GITHUB_PROVIDER = "github"

type GitHubRepository = components["schemas"]["gitHubRepository"]
type Employee = components["schemas"]["employeeListItem"]
type AgentProfile = components["schemas"]["agentProfileResponse"]

export function GithubStep() {
  const { createEmployee, goBack, goNext } = useOnboarding()
  const queryClient = useQueryClient()
  const [connecting, setConnecting] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [repositoryErrorMessage, setRepositoryErrorMessage] = useState<string | null>(null)
  const [repositoryDialogOpen, setRepositoryDialogOpen] = useState(false)
  const [repositories, setRepositories] = useState<GitHubRepository[]>([])
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())

  const { data: integrations, isLoading: integrationsLoading } = $api.useQuery(
    "get",
    "/v1/in/integrations/available"
  )

  const githubIntegration = integrations?.find(
    (integration) => integration.provider === GITHUB_PROVIDER
  )
  const agentId = createEmployee.agentId
  const employeeQuery = $api.useQuery(
    "get",
    "/v1/employees/{id}",
    { params: { path: { id: agentId ?? "" } } },
    { enabled: Boolean(agentId), retry: false }
  )
  const employee = employeeQuery.data as Employee | undefined
  const githubProfile = employee?.profiles?.find(
    (profile) => profile.provider === GITHUB_PROVIDER && profile.status === "active"
  )
  const githubConnectionID = stringFromProfileConfig(githubProfile, "in_connection_id")
  const savedRepositories = selectedRepositoriesFromProfile(githubProfile)
  const hasSavedRepositories = savedRepositories.length > 0

  const createGitHubProfile = $api.useMutation(
    "post",
    "/v1/agents/{agentID}/profiles/github"
  )
  const updateGitHubRepositories = $api.useMutation(
    "patch",
    "/v1/agents/{agentID}/profiles/github/repositories"
  )
  const repositoriesQuery = $api.useQuery(
    "get",
    "/v1/agents/{agentID}/profiles/github/repositories",
    { params: { path: { agentID: agentId ?? "" } } },
    { enabled: false, retry: false }
  )
  const reconnectGitHubProfile = useMutation({
    mutationFn: async ({ connectionId }: { connectionId: string }) => {
      const session = await api.POST("/v1/in/connections/{id}/reconnect-session", {
        params: { path: { id: connectionId } },
      })
      if (session.error) throw session.error

      const { token, provider_config_key: providerConfigKey } =
        session.data as { token: string; provider_config_key: string }

      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })

      await nango.reconnect(providerConfigKey)
      await refreshProfile(connectionId)
      await queryClient.invalidateQueries({ queryKey: ["get", "/v1/in/connections"] })
    },
    onMutate: () => {
      setErrorMessage(null)
      setRepositoryErrorMessage(null)
    },
    onSuccess: () => {
      toast.success("GitHub profile reconnected")
    },
    onError: (error) => {
      if (error instanceof AuthError && error.type === "window_closed") return
      setErrorMessage(extractErrorMessage(error, "Could not reconnect GitHub. Try again."))
    },
  })

  async function loadRepositories() {
    if (!agentId) {
      throw new Error("Create the employee profile before connecting GitHub.")
    }
    const { data, error } = await repositoriesQuery.refetch()
    if (error) throw error
    if (!data) throw new Error("GitHub repositories response was empty")

    setRepositories(data.repositories ?? [])
    setRepositoryErrorMessage(null)
    const selected = data.selected_repositories ?? []
    setSelectedIDs(
      new Set(selected.map((repo) => repo.id).filter((id): id is string => Boolean(id)))
    )
    setRepositoryDialogOpen(true)
  }

  async function attachProfileAndLoadRepositories(connectionId: string) {
    if (!agentId) {
      throw new Error("Create the employee profile before connecting GitHub.")
    }
    await createGitHubProfile.mutateAsync({
      params: { path: { agentID: agentId } },
      body: { connection_id: connectionId },
    })
    await queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
    await employeeQuery.refetch()
    await loadRepositories()
  }

  async function refreshProfile(connectionId: string) {
    if (!agentId) {
      throw new Error("Create the employee profile before reconnecting GitHub.")
    }
    await createGitHubProfile.mutateAsync({
      params: { path: { agentID: agentId } },
      body: { connection_id: connectionId },
    })
    await queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
    await employeeQuery.refetch()
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

  function handleReconnect(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    if (!githubProfile || !githubConnectionID || reconnectGitHubProfile.isPending) return
    reconnectGitHubProfile.mutate({ connectionId: githubConnectionID })
  }

  async function handleSelectRepositories() {
    if (!githubProfile || loadingRepositories) return
    setErrorMessage(null)
    try {
      await loadRepositories()
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Could not load GitHub repositories. Try again.")
      )
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
    const selected = repositories.filter((repo) => repo.id && selectedIDs.has(repo.id))
    if (selected.length === 0) {
      setRepositoryErrorMessage("Select at least one repository for this employee.")
      return
    }
    setRepositoryErrorMessage(null)
    setErrorMessage(null)
    try {
      await updateGitHubRepositories.mutateAsync({
        params: { path: { agentID: agentId } },
        body: { repositories: selected },
      })
      await queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
      await employeeQuery.refetch()
      setRepositoryDialogOpen(false)
      goNext()
    } catch (error) {
      setRepositoryErrorMessage(formatGitHubRepositorySaveError(error))
    }
  }

  const loading = integrationsLoading || employeeQuery.isLoading
  const loadingRepositories =
    createGitHubProfile.isPending || repositoriesQuery.isFetching
  const savingRepositories = updateGitHubRepositories.isPending
  const reconnecting = reconnectGitHubProfile.isPending
  const busy = connecting || reconnecting || loadingRepositories || savingRepositories
  const choiceDisabled = busy || loading || !githubIntegration || !agentId

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
              : githubProfile
                ? "Select GitHub repositories"
                : "Connect GitHub Profile"
          }
          description={
            hasSavedRepositories
              ? "Repository access is configured for this employee."
              : githubProfile
                ? "Choose the repositories this employee can inspect and work in."
              : "Sign in with GitHub so Hiveloop can request repository access for this employee."
          }
          onClick={
            githubProfile
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
            ) : githubProfile ? (
              <span className="flex shrink-0 items-center gap-1.5">
                <HugeiconsIcon
                  icon={CheckmarkCircle02Icon}
                  className="size-5 text-emerald-600"
                  strokeWidth={2}
                />
                <button
                  type="button"
                  onClick={handleReconnect}
                  disabled={reconnecting || !githubConnectionID}
                  aria-label="Reconnect GitHub profile"
                  className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                >
                  <HugeiconsIcon
                    icon={reconnecting ? Loading03Icon : RefreshIcon}
                    className={"size-4" + (reconnecting ? " animate-spin" : "")}
                    strokeWidth={2}
                  />
                </button>
              </span>
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
        errorMessage={repositoryErrorMessage}
      />
    </div>
  )
}

function selectedRepositoriesFromProfile(
  profile: AgentProfile | undefined
): GitHubRepository[] {
  const raw = profile?.config?.selected_repositories
  if (!Array.isArray(raw)) return []
  return raw.filter(isGitHubRepository)
}

function stringFromProfileConfig(profile: AgentProfile | undefined, key: string): string | null {
  const value = profile?.config?.[key]
  return typeof value === "string" && value.length > 0 ? value : null
}

function isGitHubRepository(value: unknown): value is GitHubRepository {
  return Boolean(value && typeof value === "object" && "id" in value)
}

function RepositoryPickerDialog({
  open,
  onOpenChange,
  repositories,
  selectedIDs,
  onToggle,
  onSave,
  saving,
  errorMessage,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  repositories: GitHubRepository[]
  selectedIDs: Set<string>
  onToggle: (repoID: string, checked: boolean) => void
  onSave: () => void
  saving: boolean
  errorMessage: string | null
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
                const repoID = repo.id ?? repo.full_name ?? repo.name ?? ""
                const checked = selectedIDs.has(repoID)
                return (
                  <RepositoryChoiceCard
                    key={repoID}
                    repo={repo}
                    selected={checked}
                    onToggle={() => repoID && onToggle(repoID, !checked)}
                  />
                )
              })}
            </div>
          )}
        </div>

        {errorMessage ? (
          <div className="mt-4 flex items-start gap-2.5 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-left text-[13px] text-destructive">
            <HugeiconsIcon
              icon={Alert02Icon}
              className="mt-0.5 size-4 shrink-0"
              strokeWidth={2}
            />
            <span className="leading-relaxed">{errorMessage}</span>
          </div>
        ) : null}

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

function formatGitHubRepositorySaveError(error: unknown): string {
  const permissionError = parseGitHubRepositoryPermissionError(error)
  if (permissionError) {
    const repositories = permissionError.checks
      .map((check) => {
        const missing = [
          !check.can_write ? "write access" : null,
          !check.can_manage_webhooks ? "webhook/admin access" : null,
          !check.can_read ? "read access" : null,
        ].filter(Boolean)
        const missingText = missing.length > 0 ? ` Missing: ${missing.join(", ")}.` : ""
        return `${check.repository}.${missingText} ${check.message ?? ""}`.trim()
      })
      .join(" ")
    return `The connected GitHub account does not have enough access for the selected repositories. Hiveloop needs read access, write access, and repository webhook/admin access. ${repositories} Reconnect GitHub with a repository admin account, or ask a repository admin to grant the missing access, then try again.`
  }

  const message = extractErrorMessage(error, "Could not save repository access. Try again.")
  if (message.includes("failed to create GitHub webhook")) {
    return `${message}. The connected GitHub profile can see this repository, but GitHub did not allow Hiveloop to create a repository webhook. Reconnect GitHub with an account that has admin access to the repository, or ask a repository admin to grant webhook management access, then try again.`
  }
  if (message.includes("github webhook setup is not configured")) {
    return "GitHub webhook setup is not configured on Hiveloop. This is a platform configuration issue; please contact support."
  }
  if (message.includes("failed to prepare GitHub webhook secret")) {
    return "Hiveloop could not prepare the GitHub webhook secret. This is a platform issue; please try again or contact support."
  }
  return message
}

function parseGitHubRepositoryPermissionError(error: unknown):
  | {
      checks: Array<{
        repository: string
        can_read: boolean
        can_write: boolean
        can_manage_webhooks: boolean
        message?: string
      }>
    }
  | null {
  if (!error || typeof error !== "object") return null
  const candidate = error as {
    code?: unknown
    checks?: unknown
    error?: {
      code?: unknown
      checks?: unknown
    }
  }
  const code = candidate.code ?? candidate.error?.code
  const checks = candidate.checks ?? candidate.error?.checks
  if (code !== "github_repository_permissions_missing" || !Array.isArray(checks)) {
    return null
  }
  return {
    checks: checks
      .filter((check): check is Record<string, unknown> => Boolean(check && typeof check === "object"))
      .map((check) => ({
        repository: typeof check.repository === "string" ? check.repository : "Selected repository",
        can_read: check.can_read === true,
        can_write: check.can_write === true,
        can_manage_webhooks: check.can_manage_webhooks === true,
        message: typeof check.message === "string" ? check.message : undefined,
      })),
  }
}

function RepositoryChoiceCard({
  repo,
  selected,
  onToggle,
}: {
  repo: GitHubRepository
  selected: boolean
  onToggle: () => void
}) {
  const description = repo.description || repo.owner || "GitHub repository"

  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={selected}
      className={
        "group flex w-full items-start gap-4 rounded-xl p-4 text-left transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/30 " +
        (selected
          ? "border border-primary/20 bg-primary/5"
          : "border border-transparent bg-muted/50 hover:bg-muted")
      }
    >
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-background text-muted-foreground ring-1 ring-border">
        <HugeiconsIcon icon={RepositoryIcon} className="size-4" strokeWidth={2} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold text-foreground">
            {repo.full_name}
          </span>
          {repo.private ? (
            <HugeiconsIcon
              icon={LockIcon}
              className="size-3.5 shrink-0 text-muted-foreground"
              strokeWidth={2}
              aria-label="Private repository"
            />
          ) : null}
        </span>
        <span className="mt-0.5 line-clamp-2 text-[13px] leading-relaxed text-muted-foreground">
          {description}
        </span>
      </span>
      {selected ? (
        <HugeiconsIcon
          icon={Tick02Icon}
          className="mt-0.5 size-4 shrink-0 text-primary"
          strokeWidth={2.5}
        />
      ) : null}
    </button>
  )
}
