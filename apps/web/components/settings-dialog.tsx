"use client"

import * as React from "react"
import { useState } from "react"
import { motion } from "motion/react"
import { cn } from "@/lib/utils"
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  DropdownMenu as ActionsMenu,
  DropdownMenuContent as ActionsMenuContent,
  DropdownMenuGroup as ActionsMenuGroup,
  DropdownMenuItem as ActionsMenuItem,
  DropdownMenuSeparator as ActionsMenuSeparator,
  DropdownMenuTrigger as ActionsMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Skeleton } from "@/components/ui/skeleton"
import { ProviderLogo } from "@/components/provider-logo"
import { AddLlmKeyDialog } from "@/app/w/agents/_components/create-agent/add-llm-key-dialog"
import { SandboxTemplatesList } from "@/app/w/settings/_components/sandbox-templates"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { ConfirmDialog } from "@/components/confirm-dialog"
import type { components } from "@/lib/api/schema"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  UserCircleIcon,
  UserGroupIcon,
  CreditCardIcon,
  Notification01Icon,
  Key01Icon,
  ShieldKeyIcon,
  Settings01Icon,
  ArrowDown01Icon,
  ArtificialIntelligence01Icon,
  Add01Icon,
  MoreHorizontalIcon,
  Delete02Icon,
  PauseIcon,
  Copy01Icon,
  ArrowRight01Icon,
  Tick02Icon,
  ContainerIcon,
  BookOpen01Icon,
  GitBranchIcon,
  File01Icon,
  Globe02Icon,
} from "@hugeicons/core-free-icons"

interface SettingsItem {
  id: string
  label: string
  icon: React.ComponentProps<typeof HugeiconsIcon>["icon"]
}

interface SettingsGroup {
  label: string
  items: SettingsItem[]
}

const settingsGroups: SettingsGroup[] = [
  {
    label: "Workspace",
    items: [
      { id: "general", label: "General", icon: Settings01Icon },
      { id: "members", label: "Members", icon: UserGroupIcon },
      { id: "billing", label: "Billing", icon: CreditCardIcon },
    ],
  },
  {
    label: "Account",
    items: [
      { id: "profile", label: "Profile", icon: UserCircleIcon },
      { id: "notifications", label: "Notifications", icon: Notification01Icon },
      { id: "security", label: "Security", icon: ShieldKeyIcon },
    ],
  },
  {
    label: "Developer",
    items: [
      { id: "llm-keys", label: "LLM Keys", icon: ArtificialIntelligence01Icon },
      { id: "api-keys", label: "API Keys", icon: Key01Icon },
      { id: "skills", label: "Skills", icon: BookOpen01Icon },
      { id: "sandboxes", label: "Sandboxes", icon: ContainerIcon },
    ],
  },
]

const allItems = settingsGroups.flatMap((group) => group.items)

function GeneralSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Workspace name</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          The name of your workspace visible to all members.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Workspace URL</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Your workspace&apos;s unique URL identifier.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Timezone</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Set the default timezone for your workspace.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground text-destructive">Delete workspace</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Permanently delete this workspace and all its data. This action cannot be undone.
        </p>
      </div>
    </div>
  )
}

function ProfileSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Display name</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Your name as displayed to other members.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Email address</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          The email address associated with your account.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Avatar</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Upload a profile picture to personalize your account.
        </p>
      </div>
    </div>
  )
}

function MembersSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Team members</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage who has access to this workspace.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Pending invitations</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          View and manage pending workspace invitations.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Roles</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure roles and permissions for workspace members.
        </p>
      </div>
    </div>
  )
}

function BillingSettings() {
  const queryClient = useQueryClient()
  const { data: subscriptionData, isLoading } = $api.useQuery("get", "/v1/billing/subscription", {}, {
    refetchOnWindowFocus: true,
  })
  const checkoutMutation = $api.useMutation("post", "/v1/billing/checkout")
  const portalMutation = $api.useMutation("post", "/v1/billing/portal")

  const subscription = subscriptionData as { plan?: string; status?: string; product_type?: string } | undefined

  function handleUpgrade(productType: string) {
    checkoutMutation.mutate(
      {
        body: {
          product_type: productType,
          success_url: `${window.location.origin}/w?checkout=success`,
        },
      },
      {
        onSuccess: (response) => {
          const checkoutUrl = (response as { checkout_url?: string })?.checkout_url
          if (checkoutUrl) {
            window.location.href = checkoutUrl
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to start checkout"))
        },
      },
    )
  }

  function handleManageBilling() {
    portalMutation.mutate(
      {},
      {
        onSuccess: (response) => {
          const portalUrl = (response as { portal_url?: string })?.portal_url
          if (portalUrl) {
            window.open(portalUrl, "_blank")
            // Invalidate subscription query so it refetches when user returns from portal
            queryClient.invalidateQueries({ queryKey: ["get", "/v1/billing/subscription"] })
          }
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to open billing portal"))
        },
      },
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }

  const isPro = subscription?.plan === "pro"

  if (isPro) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-foreground">Current plan</h3>
          <Badge variant="default" className="bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20">Pro</Badge>
        </div>
        <div className="rounded-xl border border-border p-5 space-y-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-foreground">
                {subscription?.product_type === "pro_dedicated" ? "Pro Dedicated" : "Pro Shared"}
              </p>
              <p className="text-xs text-muted-foreground mt-1">
                {subscription?.product_type === "pro_dedicated"
                  ? "$6.99/agent/month — 300 runs included, dedicated sandbox"
                  : "$4.99/agent/month — 300 runs included, shared sandbox"}
              </p>
            </div>
            <Badge variant="outline" className="text-green-600 dark:text-green-400 border-green-500/30">{subscription?.status}</Badge>
          </div>
        </div>
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={handleManageBilling}
            loading={portalMutation.isPending}
          >
            Manage billing
          </Button>
          <p className="mt-2 text-xs text-muted-foreground">
            View invoices, update payment methods, or cancel your subscription.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-foreground">Current plan</h3>
          <Badge variant="secondary">Free</Badge>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          1 agent, 100 runs/month, shared sandbox only.
        </p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="rounded-xl border border-border p-5 space-y-3">
          <div>
            <p className="text-sm font-medium text-foreground">Pro Shared</p>
            <p className="text-xs text-muted-foreground mt-1">$4.99/agent/month</p>
          </div>
          <ul className="space-y-1.5 text-xs text-muted-foreground">
            <li>Unlimited agents</li>
            <li>300 runs/agent/month</li>
            <li>$0.01/run overage</li>
          </ul>
          <Button
            size="sm"
            className="w-full"
            onClick={() => handleUpgrade("pro_shared")}
            loading={checkoutMutation.isPending}
          >
            Upgrade
          </Button>
        </div>

        <div className="rounded-xl border-2 border-primary/30 p-5 space-y-3 relative overflow-hidden">
          <div
            className="absolute inset-0 pointer-events-none"
            style={{
              background: "radial-gradient(circle at 50% 0%, color-mix(in oklch, var(--primary) 6%, transparent) 0%, transparent 60%)",
            }}
          />
          <div className="relative">
            <p className="text-sm font-medium text-foreground">Pro Dedicated</p>
            <p className="text-xs text-muted-foreground mt-1">$6.99/agent/month</p>
          </div>
          <ul className="space-y-1.5 text-xs text-muted-foreground relative">
            <li>Everything in Pro Shared</li>
            <li>Isolated sandbox per run</li>
            <li>Shell, filesystem, git access</li>
          </ul>
          <Button
            size="sm"
            className="w-full relative"
            onClick={() => handleUpgrade("pro_dedicated")}
            loading={checkoutMutation.isPending}
          >
            Upgrade
          </Button>
        </div>
      </div>
    </div>
  )
}

function NotificationsSettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Email notifications</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Configure which events trigger email notifications.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">In-app notifications</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Manage your in-app notification preferences.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Agent alerts</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Get notified when agents complete runs, encounter errors, or need attention.
        </p>
      </div>
    </div>
  )
}

function formatDate(dateStr: string) {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

type Credential = components["schemas"]["credentialResponse"]

function LlmKeysSettings() {
  const queryClient = useQueryClient()
  const [addKeyOpen, setAddKeyOpen] = useState(false)
  const [deleting, setDeleting] = useState<Credential | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/credentials")
  const credentials = data?.data ?? []
  const deleteCredential = $api.useMutation("delete", "/v1/credentials/{id}")

  function handleDelete() {
    if (!deleting?.id) return

    deleteCredential.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.label}" deleted`)
          queryClient.setQueryData(
            ["get", "/v1/credentials"],
            (old: typeof data) => old ? { ...old, data: old.data?.filter((credential) => credential.id !== deleting.id) } : old,
          )
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to delete credential"))
          setDeleting(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Model provider keys</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            API keys for LLM providers that power your agents.
          </p>
        </div>
        <Button size="sm" onClick={() => setAddKeyOpen(true)} variant='secondary'>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add key
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : credentials.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No LLM keys yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Add a provider key to start running agents.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setAddKeyOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={ArtificialIntelligence01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Add LLM key</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Connect a provider like OpenAI or Anthropic to power your agents.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Label</span>
              <span className="w-24 shrink-0 text-right">Requests</span>
              <span className="w-28 shrink-0 text-right">Last used</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {credentials.map((credential) => (
              <div key={credential.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex items-center gap-3 flex-1 min-w-0">
                    <ProviderLogo provider={credential.provider_id ?? ""} size={24} />
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-foreground truncate">{credential.label}</p>
                      <p className="text-xs text-muted-foreground">{credential.provider_id}</p>
                    </div>
                  </div>
                  <span className="w-24 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.request_count ?? 0}
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.last_used_at ? formatDate(credential.last_used_at) : "\u2014"}
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {credential.created_at ? formatDate(credential.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <CredentialActions onDelete={() => setDeleting(credential)} />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <ProviderLogo provider={credential.provider_id ?? ""} size={24} />
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground truncate">{credential.label}</p>
                        <p className="text-xs text-muted-foreground">{credential.provider_id}</p>
                      </div>
                    </div>
                    <CredentialActions onDelete={() => setDeleting(credential)} />
                  </div>
                  <div className="flex items-center gap-4 text-xs text-muted-foreground font-mono tabular-nums">
                    <span>{credential.request_count ?? 0} requests</span>
                    <span>{credential.created_at ? formatDate(credential.created_at) : "\u2014"}</span>
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <AddLlmKeyDialog open={addKeyOpen} onOpenChange={setAddKeyOpen} />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        title="Delete LLM key"
        description={`This will permanently delete "${deleting?.label ?? ""}" and any agents using it will no longer be able to make LLM calls.`}
        confirmText="delete"
        confirmLabel="Delete key"
        destructive
        loading={deleteCredential.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}

interface CredentialActionsProps {
  onDelete: () => void
}

function CredentialActions({ onDelete }: CredentialActionsProps) {
  return (
    <ActionsMenu>
      <ActionsMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </ActionsMenuTrigger>
      <ActionsMenuContent align="end" sideOffset={4}>
        <ActionsMenuGroup>
          <ActionsMenuItem>
            <HugeiconsIcon icon={PauseIcon} size={16} className="text-muted-foreground" />
            Deactivate
          </ActionsMenuItem>
        </ActionsMenuGroup>
        <ActionsMenuSeparator />
        <ActionsMenuGroup>
          <ActionsMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Delete
          </ActionsMenuItem>
        </ActionsMenuGroup>
      </ActionsMenuContent>
    </ActionsMenu>
  )
}

type ApiKey = components["schemas"]["apiKeyResponse"]

function ApiKeysSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [revoking, setRevoking] = useState<ApiKey | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/api-keys")
  const apiKeys = data?.data ?? []
  const revokeKey = $api.useMutation("delete", "/v1/api-keys/{id}")

  function handleRevoke() {
    if (!revoking?.id) return

    revokeKey.mutate(
      { params: { path: { id: revoking.id } } },
      {
        onSuccess: () => {
          toast.success(`"${revoking.name}" revoked`)
          queryClient.setQueryData(
            ["get", "/v1/api-keys"],
            (old: typeof data) => old ? { ...old, data: old.data?.filter((key) => key.id !== revoking.id) } : old,
          )
          setRevoking(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to revoke API key"))
          setRevoking(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">API keys</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Keys for programmatic access to your workspace.
          </p>
        </div>
        <Button size="sm" variant="secondary" onClick={() => setCreateOpen(true)}>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Create key
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : apiKeys.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No API keys yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Create a key to access the API programmatically.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setCreateOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={Key01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Create API key</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Generate a key to authenticate requests to the Ziraloop API.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-32 shrink-0">Key</span>
              <span className="w-20 shrink-0">Scopes</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {apiKeys.map((apiKey) => (
              <div key={apiKey.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-foreground truncate">{apiKey.name}</p>
                  </div>
                  <span className="w-32 shrink-0 text-[11px] text-muted-foreground font-mono tabular-nums truncate">
                    {apiKey.key_prefix ? `${apiKey.key_prefix}...` : "\u2014"}
                  </span>
                  <div className="w-20 shrink-0">
                    <ScopeBadge scopes={apiKey.scopes} />
                  </div>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {apiKey.created_at ? formatDate(apiKey.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <ApiKeyActions onRevoke={() => setRevoking(apiKey)} />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
                  <div className="flex items-center justify-between">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate">{apiKey.name}</p>
                      <p className="text-xs text-muted-foreground font-mono">{apiKey.key_prefix ? `${apiKey.key_prefix}...` : "\u2014"}</p>
                    </div>
                    <ApiKeyActions onRevoke={() => setRevoking(apiKey)} />
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono tabular-nums">
                    <span>{apiKey.created_at ? formatDate(apiKey.created_at) : "\u2014"}</span>
                    <ScopeBadge scopes={apiKey.scopes} />
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <CreateApiKeyDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={revoking !== null}
        onOpenChange={(open) => { if (!open) setRevoking(null) }}
        title="Revoke API key"
        description={`This will permanently revoke "${revoking?.name ?? ""}". Any requests using this key will be rejected immediately.`}
        confirmText="delete"
        confirmLabel="Revoke key"
        destructive
        loading={revokeKey.isPending}
        onConfirm={handleRevoke}
      />
    </div>
  )
}

interface ApiKeyActionsProps {
  onRevoke: () => void
}

function ApiKeyActions({ onRevoke }: ApiKeyActionsProps) {
  return (
    <ActionsMenu>
      <ActionsMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </ActionsMenuTrigger>
      <ActionsMenuContent align="end" sideOffset={4}>
        <ActionsMenuGroup>
          <ActionsMenuItem>
            <HugeiconsIcon icon={PauseIcon} size={16} className="text-muted-foreground" />
            Deactivate
          </ActionsMenuItem>
        </ActionsMenuGroup>
        <ActionsMenuSeparator />
        <ActionsMenuGroup>
          <ActionsMenuItem variant="destructive" onClick={onRevoke}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Revoke
          </ActionsMenuItem>
        </ActionsMenuGroup>
      </ActionsMenuContent>
    </ActionsMenu>
  )
}

const API_KEY_SCOPES = [
  { id: "all", label: "All", description: "Full access to every resource" },
  { id: "agents", label: "Agents", description: "Create, update, and run agents" },
  { id: "credentials", label: "Credentials", description: "Manage LLM provider keys" },
  { id: "connect", label: "Connect", description: "Manage OAuth connections" },
  { id: "integrations", label: "Integrations", description: "Configure third-party integrations" },
  { id: "tokens", label: "Tokens", description: "Issue and revoke tokens" },
] as const

function ScopeBadge({ scopes }: { scopes?: string[] }) {
  if (!scopes || scopes.length === 0) return <span className="text-[11px] text-muted-foreground">{"\u2014"}</span>

  if (scopes.length === 1 && scopes[0] === "all") {
    return <Badge variant="secondary" className="text-[10px]">all</Badge>
  }

  return (
    <Tooltip>
      <TooltipTrigger className="cursor-default">
        <Badge variant="secondary" className="text-[10px]">{scopes.length} scopes</Badge>
      </TooltipTrigger>
      <TooltipContent>
        {scopes.join(", ")}
      </TooltipContent>
    </Tooltip>
  )
}

function CreateApiKeyDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState("")
  const [selectedScopes, setSelectedScopes] = useState<string[]>(["all"])
  const [createdKey, setCreatedKey] = useState<string | null>(null)
  const createKey = $api.useMutation("post", "/v1/api-keys")

  function reset() {
    setName("")
    setSelectedScopes(["all"])
    setCreatedKey(null)
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function toggleScope(scope: string) {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((item) => item !== scope) : [...prev, scope],
    )
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!name.trim() || selectedScopes.length === 0) return

    createKey.mutate(
      { body: { name: name.trim(), scopes: selectedScopes } },
      {
        onSuccess: (response) => {
          const key = (response as { key?: string })?.key
          if (key) {
            setCreatedKey(key)
          } else {
            toast.success("API key created")
            handleOpenChange(false)
          }
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/api-keys"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create API key"))
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-md max-h-[90dvh] overflow-y-auto">
        <DialogTitle>{createdKey ? "API key created" : "Create API key"}</DialogTitle>

        {createdKey ? (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              Copy your API key now. You won&apos;t be able to see it again.
            </p>
            <div className="flex items-center gap-2">
              <Input value={createdKey} readOnly className="font-mono text-xs" />
              <Button
                variant="outline"
                size="icon-sm"
                onClick={() => {
                  navigator.clipboard.writeText(createdKey)
                  toast.success("Copied to clipboard")
                }}
              >
                <HugeiconsIcon icon={Copy01Icon} size={14} />
              </Button>
            </div>
            <Button onClick={() => handleOpenChange(false)} className="w-full">Done</Button>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <Label htmlFor="api-key-name">Name</Label>
              <Input
                id="api-key-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="e.g. CI/CD pipeline"
                required
                autoFocus
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label>Scopes</Label>
              <div className="flex flex-col gap-1.5">
                {API_KEY_SCOPES.map((scope) => {
                  const isSelected = selectedScopes.includes(scope.id)
                  return (
                    <button
                      key={scope.id}
                      type="button"
                      onClick={() => toggleScope(scope.id)}
                      className={cn(
                        "flex items-center gap-3 w-full rounded-xl p-3 text-left transition-colors cursor-pointer",
                        isSelected
                          ? "bg-primary/5 border border-primary/20"
                          : "bg-muted/50 hover:bg-muted border border-transparent"
                      )}
                    >
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium text-foreground">{scope.label}</p>
                        <p className="text-xs text-muted-foreground mt-0.5">{scope.description}</p>
                      </div>
                      {isSelected && (
                        <HugeiconsIcon icon={Tick02Icon} size={16} className="text-primary shrink-0" />
                      )}
                    </button>
                  )
                })}
              </div>
            </div>

            <Button type="submit" className="w-full" loading={createKey.isPending} disabled={!name.trim() || selectedScopes.length === 0}>
              Create key
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

type SkillRow = components["schemas"]["skillResponse"]

function SkillHydrationBadge({ skill }: { skill: SkillRow }) {
  const status = skill.hydration_status ?? "pending"

  if (status === "error") {
    return (
      <Tooltip>
        <TooltipTrigger className="cursor-default">
          <Badge variant="secondary" className="text-[10px] bg-red-500/10 text-red-600 dark:text-red-400 border-red-500/20">
            error
          </Badge>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs">
          <p className="text-xs font-mono whitespace-pre-wrap">{skill.hydration_error ?? "Unknown error"}</p>
        </TooltipContent>
      </Tooltip>
    )
  }

  if (status === "pending") {
    return (
      <Badge variant="secondary" className="text-[10px] bg-yellow-500/10 text-yellow-600 dark:text-yellow-400 border-yellow-500/20">
        pending
      </Badge>
    )
  }

  return (
    <Badge variant="default" className="text-[10px] bg-green-500/10 text-green-600 dark:text-green-400 border-green-500/20">
      ready
    </Badge>
  )
}

function SkillsSettings() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<SkillRow | null>(null)
  const { data, isLoading } = $api.useQuery("get", "/v1/skills", {
    params: { query: { scope: "own" } },
  })
  const skills = data?.data ?? []
  const deleteSkill = $api.useMutation("delete", "/v1/skills/{id}")

  function handleDelete() {
    if (!deleting?.id) return

    deleteSkill.mutate(
      { params: { path: { id: deleting.id } } },
      {
        onSuccess: () => {
          toast.success(`"${deleting.name}" archived`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          setDeleting(null)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to archive skill"))
          setDeleting(null)
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-foreground">Skills</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Reusable instruction bundles your agents can invoke on demand.
          </p>
        </div>
        <Button size="sm" variant="secondary" onClick={() => setCreateOpen(true)}>
          <HugeiconsIcon icon={Add01Icon} size={14} data-icon="inline-start" />
          Add skill
        </Button>
      </div>

      <div className="flex flex-col gap-2">
        {isLoading ? (
          Array.from({ length: 3 }).map((_, index) => (
            <Skeleton key={index} className="h-[52px] w-full rounded-xl" />
          ))
        ) : skills.length === 0 ? (
          <div className="flex flex-col items-center py-14">
            <div className="text-center mb-6">
              <h2 className="font-heading text-lg font-semibold text-foreground">No skills yet</h2>
              <p className="text-sm text-muted-foreground mt-1.5 max-w-xs">
                Create a skill to give your agents reusable capabilities.
              </p>
            </div>
            <div className="w-full max-w-sm">
              <button
                type="button"
                onClick={() => setCreateOpen(true)}
                className="group flex items-start gap-4 w-full rounded-xl bg-muted/50 p-4 text-left transition-colors hover:bg-muted cursor-pointer"
              >
                <HugeiconsIcon icon={BookOpen01Icon} size={20} className="shrink-0 mt-0.5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-semibold text-foreground">Create a skill</p>
                  <p className="text-[13px] text-muted-foreground mt-0.5 leading-relaxed">
                    Write inline instructions or sync from a Git repository.
                  </p>
                </div>
                <HugeiconsIcon icon={ArrowRight01Icon} size={16} className="text-muted-foreground/30 shrink-0 mt-0.5" />
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="hidden md:flex items-center gap-3 px-4 py-1 text-[10px] font-mono uppercase tracking-[1px] text-muted-foreground/50">
              <span className="flex-1 min-w-0">Name</span>
              <span className="w-20 shrink-0 text-right">Source</span>
              <span className="w-20 shrink-0 text-right">Status</span>
              <span className="w-28 shrink-0 text-right">Created</span>
              <span className="w-8 shrink-0" />
            </div>

            {skills.map((skill) => (
              <div key={skill.id}>
                {/* Desktop row */}
                <div className="hidden md:flex items-center gap-3 rounded-xl border border-border px-4 py-2.5 transition-colors hover:border-primary">
                  <div className="flex items-center gap-3 flex-1 min-w-0">
                    <HugeiconsIcon
                      icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                      size={16}
                      className="shrink-0 text-muted-foreground"
                    />
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                      {skill.description && (
                        <p className="text-xs text-muted-foreground truncate">{skill.description}</p>
                      )}
                    </div>
                  </div>
                  <span className="w-20 shrink-0 text-right">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                  </span>
                  <span className="w-20 shrink-0 text-right">
                    <SkillHydrationBadge skill={skill} />
                  </span>
                  <span className="w-28 shrink-0 text-right text-[11px] text-muted-foreground font-mono tabular-nums">
                    {skill.created_at ? formatDate(skill.created_at) : "\u2014"}
                  </span>
                  <div className="w-8 shrink-0 flex justify-center">
                    <SkillActions
                      skill={skill}
                      onDelete={() => setDeleting(skill)}
                    />
                  </div>
                </div>

                {/* Mobile row */}
                <div className="flex md:hidden flex-col gap-3 rounded-xl border border-border p-4 transition-colors hover:border-primary">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0 flex-1">
                      <HugeiconsIcon
                        icon={skill.source_type === "git" ? GitBranchIcon : File01Icon}
                        size={16}
                        className="shrink-0 text-muted-foreground"
                      />
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-foreground truncate">{skill.name}</p>
                        {skill.description && (
                          <p className="text-xs text-muted-foreground truncate">{skill.description}</p>
                        )}
                      </div>
                    </div>
                    <SkillActions
                      skill={skill}
                      onDelete={() => setDeleting(skill)}
                    />
                  </div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono tabular-nums">
                    <Badge variant="secondary" className="text-[10px]">
                      {skill.source_type === "git" ? "git" : "inline"}
                    </Badge>
                    <SkillHydrationBadge skill={skill} />
                    <span>{skill.created_at ? formatDate(skill.created_at) : "\u2014"}</span>
                  </div>
                </div>
              </div>
            ))}
          </>
        )}
      </div>

      <CreateSkillDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        title="Archive skill"
        description={`This will archive "${deleting?.name ?? ""}". Agents using this skill will no longer be able to invoke it.`}
        confirmText="archive"
        confirmLabel="Archive skill"
        destructive
        loading={deleteSkill.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}

interface SkillActionsProps {
  skill: SkillRow
  onDelete: () => void
}

function SkillActions({ skill, onDelete }: SkillActionsProps) {
  const queryClient = useQueryClient()
  const publishSkill = $api.useMutation("post", "/v1/skills/{id}/publish")
  const unpublishSkill = $api.useMutation("delete", "/v1/skills/{id}/publish")

  function handlePublish() {
    if (!skill.id) return
    publishSkill.mutate(
      { params: { path: { id: skill.id } } },
      {
        onSuccess: () => {
          toast.success(`"${skill.name}" published to marketplace`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to publish skill"))
        },
      },
    )
  }

  function handleUnpublish() {
    if (!skill.id) return
    unpublishSkill.mutate(
      { params: { path: { id: skill.id } } },
      {
        onSuccess: () => {
          toast.success(`"${skill.name}" removed from marketplace`)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to unpublish skill"))
        },
      },
    )
  }

  const isPublished = !!skill.public_skill_id

  return (
    <ActionsMenu>
      <ActionsMenuTrigger className="flex items-center justify-center h-8 w-8 rounded-lg transition-colors hover:bg-muted outline-none">
        <HugeiconsIcon icon={MoreHorizontalIcon} size={16} className="text-muted-foreground" />
      </ActionsMenuTrigger>
      <ActionsMenuContent align="end" sideOffset={4}>
        <ActionsMenuGroup>
          {isPublished ? (
            <ActionsMenuItem onClick={handleUnpublish} disabled={unpublishSkill.isPending}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Unpublish from marketplace
            </ActionsMenuItem>
          ) : (
            <ActionsMenuItem onClick={handlePublish} disabled={publishSkill.isPending}>
              <HugeiconsIcon icon={Globe02Icon} size={16} className="text-muted-foreground" />
              Publish to marketplace
            </ActionsMenuItem>
          )}
        </ActionsMenuGroup>
        <ActionsMenuSeparator />
        <ActionsMenuGroup>
          <ActionsMenuItem variant="destructive" onClick={onDelete}>
            <HugeiconsIcon icon={Delete02Icon} size={16} />
            Archive
          </ActionsMenuItem>
        </ActionsMenuGroup>
      </ActionsMenuContent>
    </ActionsMenu>
  )
}

function CreateSkillDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queryClient = useQueryClient()
  const [sourceType, setSourceType] = useState<"inline" | "git">("inline")
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [repoUrl, setRepoUrl] = useState("")
  const [repoSubpath, setRepoSubpath] = useState("")
  const [repoRef, setRepoRef] = useState("main")
  const createSkill = $api.useMutation("post", "/v1/skills")

  function reset() {
    setSourceType("inline")
    setName("")
    setDescription("")
    setRepoUrl("")
    setRepoSubpath("")
    setRepoRef("main")
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) reset()
    onOpenChange(nextOpen)
  }

  function handleSubmit(event: React.FormEvent) {
    event.preventDefault()
    if (!name.trim()) return

    const body: Record<string, unknown> = {
      name: name.trim(),
      description: description.trim() || undefined,
      source_type: sourceType,
    }

    if (sourceType === "git") {
      if (!repoUrl.trim()) {
        toast.error("Repository URL is required for git skills")
        return
      }
      body.repo_url = repoUrl.trim()
      body.repo_subpath = repoSubpath.trim() || undefined
      body.repo_ref = repoRef.trim() || "main"
    } else {
      body.bundle = {
        id: name.trim().toLowerCase().replace(/\s+/g, "-"),
        title: name.trim(),
        description: description.trim(),
        content: "",
        references: [],
      }
    }

    createSkill.mutate(
      { body: body as never },
      {
        onSuccess: () => {
          toast.success("Skill created")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/skills"] })
          handleOpenChange(false)
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to create skill"))
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton className="sm:max-w-md max-h-[90dvh] overflow-y-auto">
        <DialogTitle>Create skill</DialogTitle>

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <Tabs
            value={sourceType}
            onValueChange={(value) => setSourceType(value as "inline" | "git")}
          >
            <TabsList className="w-full">
              <TabsTrigger value="inline">
                <HugeiconsIcon icon={File01Icon} size={14} />
                Inline
              </TabsTrigger>
              <TabsTrigger value="git">
                <HugeiconsIcon icon={GitBranchIcon} size={14} />
                Git
              </TabsTrigger>
            </TabsList>

            <TabsContent value="inline">
              <div className="flex flex-col gap-4 pt-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-name-inline">Name</Label>
                  <Input
                    id="skill-name-inline"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="e.g. Browser automation"
                    required
                    autoFocus
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-description-inline">Description</Label>
                  <Input
                    id="skill-description-inline"
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="What this skill does..."
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent value="git">
              <div className="flex flex-col gap-4 pt-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-name-git">Name</Label>
                  <Input
                    id="skill-name-git"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="e.g. Browser automation"
                    required
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-description-git">Description</Label>
                  <Input
                    id="skill-description-git"
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="What this skill does..."
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="skill-repo-url">Repository URL</Label>
                  <Input
                    id="skill-repo-url"
                    value={repoUrl}
                    onChange={(event) => setRepoUrl(event.target.value)}
                    placeholder="https://github.com/org/repo"
                    required
                  />
                </div>
                <div className="flex gap-3">
                  <div className="flex flex-col gap-2 flex-1">
                    <Label htmlFor="skill-repo-subpath">Subpath</Label>
                    <Input
                      id="skill-repo-subpath"
                      value={repoSubpath}
                      onChange={(event) => setRepoSubpath(event.target.value)}
                      placeholder="skills/my-skill"
                    />
                  </div>
                  <div className="flex flex-col gap-2 w-24">
                    <Label htmlFor="skill-repo-ref">Ref</Label>
                    <Input
                      id="skill-repo-ref"
                      value={repoRef}
                      onChange={(event) => setRepoRef(event.target.value)}
                      placeholder="main"
                    />
                  </div>
                </div>
              </div>
            </TabsContent>
          </Tabs>

          <Button type="submit" className="w-full" loading={createSkill.isPending} disabled={!name.trim()}>
            Create skill
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SecuritySettings() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-foreground">Two-factor authentication</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Add an extra layer of security to your account.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Active sessions</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          View and manage your active sessions across devices.
        </p>
      </div>
      <div>
        <h3 className="text-sm font-medium text-foreground">Audit log</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Review a log of actions taken in your workspace.
        </p>
      </div>
    </div>
  )
}

const sectionComponents: Record<string, React.ComponentType> = {
  general: GeneralSettings,
  profile: ProfileSettings,
  members: MembersSettings,
  billing: BillingSettings,
  notifications: NotificationsSettings,
  "llm-keys": LlmKeysSettings,
  "api-keys": ApiKeysSettings,
  skills: SkillsSettings,
  security: SecuritySettings,
  sandboxes: SandboxTemplatesList,
}

interface SettingsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialSection?: string
}

export function SettingsDialog({ open, onOpenChange, initialSection }: SettingsDialogProps) {
  const [activeSection, setActiveSection] = useState(initialSection ?? "general")

  React.useEffect(() => {
    if (open && initialSection) {
      setActiveSection(initialSection)
    }
  }, [open, initialSection])

  const ActiveComponent = sectionComponents[activeSection]
  const activeLabel = allItems.find((item) => item.id === activeSection)?.label ?? "Settings"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton
        className="max-w-[100vw] h-dvh rounded-none p-0 gap-0 overflow-hidden md:max-w-5xl md:h-160 md:rounded-4xl"
      >
        <DialogTitle className="sr-only md:hidden">Settings</DialogTitle>
        <div className="flex flex-col md:flex-row h-full">
          <div className="flex md:hidden shrink-0 flex-col border-b border-border">
            <div className="flex items-center px-4 pt-4 pb-2">
              <h2 className="font-mono uppercase text-muted-foreground text-sm font-medium">Settings</h2>
            </div>
            <div className="px-4 pb-3 my-4">
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={<Button variant="outline" className="w-full justify-between" />}
                >
                  <span className="flex items-center gap-2">
                    <HugeiconsIcon icon={allItems.find((item) => item.id === activeSection)?.icon ?? Settings01Icon} size={14} />
                    {activeLabel}
                  </span>
                  <HugeiconsIcon icon={ArrowDown01Icon} size={14} className="text-muted-foreground" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="min-w-[calc(100vw-2rem)]">
                  <DropdownMenuRadioGroup
                    value={activeSection}
                    onValueChange={(value) => setActiveSection(value)}
                  >
                    {settingsGroups.map((group, index) => (
                      <DropdownMenuGroup key={group.label}>
                        <DropdownMenuLabel>{group.label}</DropdownMenuLabel>
                        {group.items.map((item) => (
                          <DropdownMenuRadioItem key={item.id} value={item.id}>
                            <HugeiconsIcon icon={item.icon} size={14} />
                            {item.label}
                          </DropdownMenuRadioItem>
                        ))}
                        {index < settingsGroups.length - 1 && <DropdownMenuSeparator />}
                      </DropdownMenuGroup>
                    ))}
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>

          <nav className="hidden md:flex w-52 shrink-0 flex-col gap-4 border-r border-border bg-muted/30 p-3">
            {settingsGroups.map((group) => (
              <div key={group.label} className="flex flex-col gap-1">
                <p className="px-2.5 pb-1 font-mono text-[10px] uppercase tracking-[1.5px] text-muted-foreground">
                  {group.label}
                </p>
                {group.items.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => setActiveSection(item.id)}
                    className={cn(
                      "relative flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm transition-colors",
                      activeSection === item.id
                        ? "text-foreground"
                        : "text-muted-foreground hover:bg-background/50 hover:text-foreground"
                    )}
                  >
                    {activeSection === item.id && (
                      <motion.div
                        layoutId="settings-nav-active"
                        className="absolute inset-0 rounded-xl bg-background shadow-sm ring-1 ring-border"
                        transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
                      />
                    )}
                    <span className="relative flex items-center gap-2.5">
                      <HugeiconsIcon icon={item.icon} size={16} />
                      {item.label}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </nav>

          <div className="flex flex-1 flex-col overflow-hidden">
            <div className="hidden md:block shrink-0 border-b border-border px-6 py-4">
              <h2 className="font-heading text-base font-medium">{activeLabel}</h2>
            </div>
            <div className="flex-1 overflow-y-auto px-4 py-4 md:px-6 md:py-5">
              <ActiveComponent />
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
