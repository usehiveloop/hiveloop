"use client"

import { useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import Nango, { AuthError } from "@nangohq/frontend"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Alert02Icon,
  Copy01Icon,
  Loading03Icon,
  Plug01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { ChoiceCard } from "@/app/w/agents/_components/create-agent/choice-card"
import { FormEmptyWell, FormSection } from "@/app/w/_components/form-section"
import {
  IntegrationLogo,
  integrationLogoURL,
} from "@/components/integration-logo"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

type AvailableProfile =
  components["schemas"]["profileProviderAvailableResponse"]

const OAUTH_MODES = ["OAUTH2", "OAUTH1", "TBA"]
const NO_CRED_MODES = [
  "BASIC",
  "API_KEY",
  "NONE",
  "JWT",
  "SIGNATURE",
  "TWO_STEP",
]

export function EmployeeProfilesSection({
  employeeID,
}: {
  employeeID: string
}) {
  const [dialogOpen, setDialogOpen] = useState(false)
  const profilesQuery = $api.useQuery(
    "get",
    "/v1/agents/{agentID}/profiles/available",
    {
      params: { path: { agentID: employeeID } },
    },
    {
      enabled: Boolean(employeeID),
    }
  )

  const profiles = useMemo(() => profilesQuery.data ?? [], [profilesQuery.data])
  const connectedProfiles = useMemo(
    () => profiles.filter((profile) => profile.profile),
    [profiles]
  )

  return (
    <>
      <FormSection
        title="Profiles"
        description="Connect employee-specific accounts that should not be reused across the workspace."
      >
        <div className="flex flex-col gap-2">
          {profilesQuery.isLoading ? (
            <ProfilesSkeleton />
          ) : connectedProfiles.length === 0 ? (
            <FormEmptyWell
              icon={Plug01Icon}
              message="No profiles connected."
              action={
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setDialogOpen(true)}
                >
                  Manage profiles
                </Button>
              }
            />
          ) : (
            <>
              {connectedProfiles.map((profile) => (
                <ConnectedProfileRow key={profile.provider} profile={profile} />
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-1 w-fit"
                onClick={() => setDialogOpen(true)}
              >
                Manage profiles
              </Button>
            </>
          )}
        </div>
      </FormSection>

      <EmployeeProfilesDialog
        employeeID={employeeID}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        profiles={profiles}
        loading={profilesQuery.isLoading}
        error={
          profilesQuery.isError
            ? extractErrorMessage(
                profilesQuery.error,
                "Failed to load profiles"
              )
            : null
        }
      />
    </>
  )
}

function EmployeeProfilesDialog({
  employeeID,
  open,
  onOpenChange,
  profiles,
  loading,
  error,
}: {
  employeeID: string
  open: boolean
  onOpenChange: (open: boolean) => void
  profiles: AvailableProfile[]
  loading: boolean
  error: string | null
}) {
  const queryClient = useQueryClient()
  const [customAppProfile, setCustomAppProfile] =
    useState<AvailableProfile | null>(null)
  const [connectingProvider, setConnectingProvider] = useState<string | null>(
    null
  )

  async function connectProfile(profile: AvailableProfile) {
    if (!profile.provider || profile.profile || connectingProvider) return
    if (profile.employee_profile?.custom_app) {
      setConnectingProvider(profile.provider)
      try {
        await openCustomAppSetup(profile)
      } catch (err) {
        toast.error(extractErrorMessage(err, "Failed to load custom app setup"))
      } finally {
        setConnectingProvider(null)
      }
      return
    }

    await startOAuthProfileConnect(profile)
  }

  async function openCustomAppSetup(profile: AvailableProfile) {
    if (!profile.provider) return
    if (profile.custom_app_integration_id) {
      setCustomAppProfile(profile)
      return
    }
    const placeholder = await api.POST(
      "/v1/agents/{agentID}/profiles/{provider}/custom-app",
      {
        params: {
          path: { agentID: employeeID, provider: profile.provider },
        },
        body: { display_name: profile.display_name } as never,
      }
    )
    if (placeholder.error) throw placeholder.error
    const integration = placeholder.data?.integration
    setCustomAppProfile({
      ...profile,
      custom_app_integration_id: integration?.id,
      provider_config_key: placeholder.data?.provider_config_key,
      nango_config: integration?.nango_config ?? profile.nango_config,
    })
    invalidateProfileQueries(queryClient, employeeID)
  }

  async function startOAuthProfileConnect(profile: AvailableProfile) {
    if (!profile.provider || profile.profile || connectingProvider) return
    setConnectingProvider(profile.provider)
    try {
      const session = await api.POST(
        "/v1/agents/{agentID}/profiles/{provider}/connect-session",
        {
          params: {
            path: { agentID: employeeID, provider: profile.provider },
          },
        }
      )
      if (session.error) throw session.error

      const { token, provider_config_key: providerConfigKey } =
        session.data as { token: string; provider_config_key: string }
      const nango = new Nango({
        connectSessionToken: token,
        host: process.env.NEXT_PUBLIC_CONNECTIONS_HOST,
      })
      const authResult = await nango.auth(providerConfigKey)
      const complete = await api.POST(
        "/v1/agents/{agentID}/profiles/{provider}/complete",
        {
          params: {
            path: { agentID: employeeID, provider: profile.provider },
          },
          body: { nango_connection_id: authResult.connectionId } as never,
        }
      )
      if (complete.error) throw complete.error

      toast.success(`${profile.display_name ?? "Profile"} connected`)
      invalidateProfileQueries(queryClient, employeeID)
    } catch (err) {
      if (err instanceof AuthError && err.type === "window_closed") return
      toast.error(extractErrorMessage(err, "Failed to connect profile"))
    } finally {
      setConnectingProvider(null)
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="flex h-[min(680px,85vh)] flex-col overflow-hidden p-6 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Manage profiles</DialogTitle>
            <DialogDescription>
              Choose the employee-specific accounts this employee can use.
            </DialogDescription>
          </DialogHeader>

          <div className="mt-4 flex flex-1 flex-col gap-2 overflow-y-auto">
            {loading ? (
              <ProfilesSkeleton />
            ) : error ? (
              <div className="flex gap-3 rounded-xl bg-destructive/10 p-4 text-sm text-destructive">
                <HugeiconsIcon
                  icon={Alert02Icon}
                  className="mt-0.5 size-4 shrink-0"
                  strokeWidth={2}
                />
                <span>{error}</span>
              </div>
            ) : profiles.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-3 py-12">
                <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                  <HugeiconsIcon
                    icon={Plug01Icon}
                    size={20}
                    className="text-muted-foreground"
                  />
                </div>
                <div className="text-center">
                  <p className="text-sm font-medium text-foreground">
                    No profiles available
                  </p>
                  <p className="mt-1 max-w-[260px] text-xs text-muted-foreground">
                    Profile-capable integrations will appear here.
                  </p>
                </div>
              </div>
            ) : (
              profiles.map((profile) => (
                <ProfileChoiceCard
                  key={profile.provider}
                  profile={profile}
                  busy={connectingProvider === profile.provider}
                  disabled={Boolean(connectingProvider)}
                  onConnect={() => connectProfile(profile)}
                />
              ))
            )}
          </div>

          <div className="shrink-0 pt-4">
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => onOpenChange(false)}
            >
              Done
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <CustomAppDialog
        employeeID={employeeID}
        profile={customAppProfile}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setCustomAppProfile(null)
        }}
        onUpdated={(profile) => {
          setCustomAppProfile(null)
          void (async () => {
            await refreshProfileQueries(queryClient, employeeID)
            await startOAuthProfileConnect(profile)
          })()
        }}
      />
    </>
  )
}

function ProfileChoiceCard({
  profile,
  busy,
  disabled,
  onConnect,
}: {
  profile: AvailableProfile
  busy: boolean
  disabled: boolean
  onConnect: () => void
}) {
  const connected = Boolean(profile.profile)
  const customAppRequired = profileNeedsCustomAppSetup(profile)

  return (
    <ChoiceCard
      logoUrl={integrationLogoURL(profile.provider ?? "")}
      logoSize={32}
      title={profile.display_name ?? profile.provider ?? "Profile"}
      description={
        connected
          ? "Connected"
          : customAppRequired
            ? "Set up this employee's custom app before connecting."
            : "Connect this profile to the employee."
      }
      selected={connected}
      onClick={connected || disabled ? () => {} : onConnect}
      trailing={
        connected ? (
          <HugeiconsIcon
            icon={Tick02Icon}
            size={16}
            className="mt-0.5 shrink-0 text-primary"
          />
        ) : busy ? (
          <HugeiconsIcon
            icon={Loading03Icon}
            size={16}
            className="mt-0.5 shrink-0 animate-spin text-primary"
          />
        ) : (
          <span className="inline-flex h-8 shrink-0 items-center justify-center rounded-full bg-primary px-3 text-xs font-medium text-primary-foreground">
            {customAppRequired ? "Set up" : "Connect"}
          </span>
        )
      }
    />
  )
}

function profileNeedsCustomAppSetup(profile: AvailableProfile) {
  return Boolean(
    profile.employee_profile?.custom_app && !profile.custom_app_configured
  )
}

async function refreshProfileQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  employeeID: string
) {
  invalidateProfileQueries(queryClient, employeeID)
  await queryClient.refetchQueries({
    queryKey: ["get", "/v1/agents/{agentID}/profiles/available"],
  })
}

function CustomAppDialog({
  employeeID,
  profile,
  onOpenChange,
  onUpdated,
}: {
  employeeID: string
  profile: AvailableProfile | null
  onOpenChange: (open: boolean) => void
  onUpdated: (profile: AvailableProfile) => void
}) {
  const [credentials, setCredentials] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  const nangoConfig = profile?.nango_config
  const profileName = (profile?.display_name ?? "profile").toLowerCase()
  const authMode = nangoConfig?.auth_mode ?? "OAUTH2"
  const scopes = profile?.employee_profile?.scopes ?? []
  const generatedWebhookSecret = stringFromConfig(nangoConfig?.webhook_secret)
  const hasUserDefinedWebhookSecret = Boolean(
    nangoConfig?.webhook_user_defined_secret
  )
  const canSubmit =
    Boolean(profile?.provider) &&
    customAppCredentialsValid(
      authMode,
      credentials,
      hasUserDefinedWebhookSecret
    ) &&
    !submitting

  useEffect(() => {
    if (!profile) {
      setCredentials({})
      return
    }
    setCredentials(
      generatedWebhookSecret
        ? { webhook_secret: generatedWebhookSecret }
        : {}
    )
  }, [generatedWebhookSecret, profile])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!profile?.provider || !canSubmit) return
    setSubmitting(true)
    try {
      const resp = await api.PUT(
        "/v1/agents/{agentID}/profiles/{provider}/custom-app",
        {
          params: {
            path: { agentID: employeeID, provider: profile.provider },
          },
          body: {
            display_name: profile.display_name,
            credentials: customAppCredentials(authMode, credentials, scopes),
          } as never,
        }
      )
      if (resp.error) throw resp.error
      toast.success(`${profile.display_name ?? "Profile"} custom app saved`)
      setCredentials({})
      const integration = resp.data?.integration
      onUpdated({
        ...profile,
        custom_app_configured: true,
        custom_app_integration_id:
          integration?.id ?? profile.custom_app_integration_id,
        provider_config_key:
          resp.data?.provider_config_key ?? profile.provider_config_key,
        nango_config: integration?.nango_config ?? profile.nango_config,
      })
    } catch (err) {
      toast.error(extractErrorMessage(err, "Failed to save custom app"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={Boolean(profile)} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100vh-2rem)] w-full max-w-[calc(100vw-2rem)] flex-col overflow-hidden p-0 sm:max-w-lg">
        <DialogHeader className="px-6 pt-6 pb-0">
          <DialogTitle>Set up {profileName}</DialogTitle>
          <DialogDescription>
            Add the OAuth app credentials for {profileName}.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col">
          <div className="min-w-0 flex-1 overflow-y-auto px-6 py-5">
            <div className="flex min-w-0 flex-col gap-4">
              <div className="flex min-w-0 items-center gap-3 rounded-xl bg-muted/50 p-3">
                {profile?.provider ? (
                  <IntegrationLogo provider={profile.provider} size={32} />
                ) : null}
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {profile?.display_name ?? "Profile"}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {authMode}
                  </p>
                </div>
              </div>

              <CustomAppWebhookDetails nangoConfig={nangoConfig} />

              <CustomAppScopes scopes={scopes} />

              <CustomAppCredentialsFields
                authMode={authMode}
                hasUserDefinedWebhookSecret={hasUserDefinedWebhookSecret}
                generatedWebhookSecret={generatedWebhookSecret}
                credentials={credentials}
                onChange={setCredentials}
              />
            </div>
          </div>

          <div className="border-t border-border/60 px-6 pt-4 pb-6">
            <Button
              type="submit"
              disabled={!canSubmit}
              loading={submitting}
              className="w-full"
            >
              Next
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CustomAppScopes({ scopes }: { scopes: string[] }) {
  const [copied, setCopied] = useState(false)
  if (scopes.length === 0) return null
  const value = scopes.join(",")

  async function copyValue() {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className="min-w-0 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <Label>OAuth scopes</Label>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={copyValue}
          className="h-7 shrink-0 px-2 text-xs"
        >
          <HugeiconsIcon
            icon={copied ? Tick02Icon : Copy01Icon}
            className="size-3.5"
            strokeWidth={2.5}
          />
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {scopes.map((scope) => (
          <Badge
            key={scope}
            variant="ghost"
            className="rounded-md bg-muted px-2 py-1 font-mono text-[11px] font-normal text-muted-foreground"
          >
            {scope}
          </Badge>
        ))}
      </div>
    </div>
  )
}

function CustomAppCredentialsFields({
  authMode,
  hasUserDefinedWebhookSecret,
  generatedWebhookSecret,
  credentials,
  onChange,
}: {
  authMode: string
  hasUserDefinedWebhookSecret: boolean
  generatedWebhookSecret: string
  credentials: Record<string, string>
  onChange: (value: Record<string, string>) => void
}) {
  const fields = customAppCredentialFields(authMode)
  const set = (key: string, value: string) =>
    onChange({ ...credentials, [key]: value })

  if (fields.length === 0 && !hasUserDefinedWebhookSecret) {
    return (
      <p className="text-sm text-muted-foreground">
        No credentials are needed for this auth mode.
      </p>
    )
  }

  return (
    <>
      {fields.map((field, index) => (
        <div key={field.key} className="flex flex-col gap-2">
          <Label htmlFor={`custom-app-${field.key}`}>{field.label}</Label>
          {field.multiline ? (
            <Textarea
              id={`custom-app-${field.key}`}
              value={credentials[field.key] ?? ""}
              onChange={(event) => set(field.key, event.target.value)}
              className="min-h-24"
              placeholder={field.placeholder}
              autoFocus={index === 0}
              required
            />
          ) : (
            <Input
              id={`custom-app-${field.key}`}
              type={field.secret ? "password" : "text"}
              value={credentials[field.key] ?? ""}
              onChange={(event) => set(field.key, event.target.value)}
              placeholder={field.placeholder}
              autoFocus={index === 0}
              required
            />
          )}
        </div>
      ))}
      {hasUserDefinedWebhookSecret ? (
        <div className="flex flex-col gap-2">
          <Label htmlFor="custom-app-webhook-secret">Webhook secret</Label>
          <Input
            id="custom-app-webhook-secret"
            type="text"
            value={credentials.webhook_secret ?? ""}
            onChange={(event) => set("webhook_secret", event.target.value)}
            placeholder={
              generatedWebhookSecret ||
              "Paste the webhook signing secret from your app settings"
            }
            required
          />
        </div>
      ) : null}
    </>
  )
}

function CustomAppWebhookDetails({
  nangoConfig,
}: {
  nangoConfig?: AvailableProfile["nango_config"]
}) {
  const callbackURL = stringFromConfig(nangoConfig?.callback_url)
  const webhookURL = stringFromConfig(nangoConfig?.webhook_url)
  const webhookSecret = stringFromConfig(nangoConfig?.webhook_secret)
  const hasUserDefinedWebhookSecret = Boolean(
    nangoConfig?.webhook_user_defined_secret
  )
  if (
    !callbackURL &&
    !webhookURL &&
    (!webhookSecret || hasUserDefinedWebhookSecret)
  ) {
    return null
  }

  return (
    <div className="flex min-w-0 flex-col gap-3 rounded-xl border border-border bg-muted/30 p-3">
      {callbackURL ? (
        <ReadonlyValue label="OAuth callback URL" value={callbackURL} />
      ) : null}
      {webhookURL ? (
        <ReadonlyValue label="Webhook URL" value={webhookURL} />
      ) : null}
      {webhookSecret && !hasUserDefinedWebhookSecret ? (
        <ReadonlyValue label="Webhook secret" value={webhookSecret} secret />
      ) : null}
    </div>
  )
}

function ReadonlyValue({
  label,
  value,
  secret,
}: {
  label: string
  value: string
  secret?: boolean
}) {
  const [copied, setCopied] = useState(false)

  async function copyValue() {
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1600)
  }

  return (
    <div className="min-w-0 space-y-1.5">
      <Label>{label}</Label>
      <InputGroup>
        <InputGroupInput
          value={value}
          readOnly
          type={secret ? "password" : "text"}
          className="font-mono text-xs"
          onFocus={(event) => event.currentTarget.select()}
        />
        <InputGroupAddon align="inline-end">
          <InputGroupButton
          type="button"
          size="icon-sm"
          onClick={copyValue}
          aria-label={`Copy ${label}`}
        >
          <HugeiconsIcon
            icon={copied ? Tick02Icon : Copy01Icon}
            className="size-3.5"
            strokeWidth={2.5}
          />
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </div>
  )
}

function ConnectedProfileRow({ profile }: { profile: AvailableProfile }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/50 p-3">
      <div className="flex min-w-0 items-center gap-3">
        {profile.provider ? (
          <IntegrationLogo provider={profile.provider} size={32} />
        ) : (
          <div className="flex size-8 items-center justify-center rounded-md bg-muted text-muted-foreground">
            <HugeiconsIcon
              icon={Plug01Icon}
              className="size-4"
              strokeWidth={2}
            />
          </div>
        )}
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {profile.display_name ?? profile.provider ?? "Profile"}
          </p>
          <p className="truncate text-xs text-muted-foreground">
            {profile.provider}
          </p>
        </div>
      </div>
      <Badge
        variant="ghost"
        className="shrink-0 gap-1.5 bg-success/15 text-success"
      >
        <HugeiconsIcon
          icon={Tick02Icon}
          className="size-3"
          strokeWidth={2.75}
        />
        Connected
      </Badge>
    </div>
  )
}

function ProfilesSkeleton() {
  return (
    <div className="flex flex-col gap-2" aria-busy="true">
      {Array.from({ length: 2 }).map((_, index) => (
        <Skeleton key={index} className="h-[66px] rounded-xl" />
      ))}
    </div>
  )
}

function invalidateProfileQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  _employeeID: string
) {
  queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
  queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees/{id}"] })
  queryClient.invalidateQueries({
    queryKey: ["get", "/v1/agents/{agentID}/profiles/available"],
  })
  queryClient.invalidateQueries({
    queryKey: ["get", "/v1/employees/{id}/connections/available"],
  })
  queryClient.invalidateQueries({
    queryKey: ["get", "/v1/employees/{id}/agent-templates"],
  })
}

function needsOAuthFields(authMode: string) {
  return OAUTH_MODES.includes(authMode.toUpperCase())
}

function needsAppFields(authMode: string) {
  return authMode.toUpperCase() === "APP"
}

function needsCustomFields(authMode: string) {
  return authMode.toUpperCase() === "CUSTOM"
}

function needsInstallPluginFields(authMode: string) {
  return authMode.toUpperCase() === "INSTALL_PLUGIN"
}

function needsNoCredentials(authMode: string) {
  return NO_CRED_MODES.includes(authMode.toUpperCase())
}

function customAppCredentialFields(authMode: string) {
  if (needsNoCredentials(authMode)) return []
  if (needsOAuthFields(authMode)) {
    return [
      {
        key: "client_id",
        label: "Client ID",
        placeholder: "Paste the OAuth client ID from your app",
      },
      {
        key: "client_secret",
        label: "Client secret",
        secret: true,
        placeholder: "Paste the OAuth client secret from your app",
      },
    ]
  }
  if (needsAppFields(authMode)) {
    return [
      {
        key: "app_id",
        label: "App ID",
        placeholder: "Paste the app ID from your app settings",
      },
      {
        key: "app_link",
        label: "App link",
        placeholder: "Paste the app installation or app settings URL",
      },
      {
        key: "private_key",
        label: "Private key",
        secret: true,
        multiline: true,
        placeholder: "Paste the private key generated for this app",
      },
    ]
  }
  if (needsInstallPluginFields(authMode)) {
    return [
      {
        key: "app_link",
        label: "App link",
        placeholder: "Paste the plugin installation or app settings URL",
      },
    ]
  }
  if (needsCustomFields(authMode)) {
    return [
      {
        key: "client_id",
        label: "Client ID",
        placeholder: "Paste the OAuth client ID from your app",
      },
      {
        key: "client_secret",
        label: "Client secret",
        secret: true,
        placeholder: "Paste the OAuth client secret from your app",
      },
      {
        key: "app_id",
        label: "App ID",
        placeholder: "Paste the app ID from your app settings",
      },
      {
        key: "app_link",
        label: "App link",
        placeholder: "Paste the app installation or app settings URL",
      },
      {
        key: "private_key",
        label: "Private key",
        secret: true,
        multiline: true,
        placeholder: "Paste the private key generated for this app",
      },
    ]
  }
  return []
}

function customAppCredentialsValid(
  authMode: string,
  credentials: Record<string, string>,
  requiresWebhookSecret: boolean
) {
  for (const field of customAppCredentialFields(authMode)) {
    if (!credentials[field.key]?.trim()) return false
  }
  if (requiresWebhookSecret && !credentials.webhook_secret?.trim()) return false
  return true
}

function customAppCredentials(
  authMode: string,
  credentials: Record<string, string>,
  scopes: string[]
) {
  const out: Record<string, string> = { type: authMode }
  if (scopes.length > 0) out.scopes = scopes.join(",")
  for (const [key, value] of Object.entries(credentials)) {
    const trimmed = value.trim()
    if (trimmed) out[key] = trimmed
  }
  return out
}

function stringFromConfig(value: unknown) {
  return typeof value === "string" ? value : ""
}
