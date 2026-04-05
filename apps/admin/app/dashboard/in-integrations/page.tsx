"use client"

import { useState, useMemo } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { PageHeader } from "@/components/admin/page-header"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface InIntegration {
  id?: string
  provider?: string
  display_name?: string
  unique_key?: string
  meta?: Record<string, unknown>
  nango_config?: Record<string, unknown>
  created_at?: string
  updated_at?: string
}

interface Provider {
  name: string
  display_name: string
  auth_mode: string
  webhook_user_defined_secret?: boolean
}

// ---------------------------------------------------------------------------
// Auth-mode helpers
// ---------------------------------------------------------------------------

const OAUTH_MODES = ["OAUTH2", "OAUTH1", "TBA"]
const APP_MODE = "APP"
const CUSTOM_MODE = "CUSTOM"
const INSTALL_PLUGIN_MODE = "INSTALL_PLUGIN"
const NO_CRED_MODES = ["BASIC", "API_KEY", "NONE", "JWT", "SIGNATURE", "TWO_STEP"]

type AuthMode = string

function needsOAuthFields(mode: AuthMode) {
  return OAUTH_MODES.includes(mode.toUpperCase())
}
function needsAppFields(mode: AuthMode) {
  return mode.toUpperCase() === APP_MODE
}
function needsCustomFields(mode: AuthMode) {
  return mode.toUpperCase() === CUSTOM_MODE
}
function needsInstallPluginFields(mode: AuthMode) {
  return mode.toUpperCase() === INSTALL_PLUGIN_MODE
}
function needsNoCredentials(mode: AuthMode) {
  return NO_CRED_MODES.includes(mode.toUpperCase())
}

function authModeBadgeVariant(mode: string): "default" | "secondary" | "outline" {
  const upper = mode.toUpperCase()
  if (OAUTH_MODES.includes(upper)) return "default"
  if (upper === APP_MODE || upper === CUSTOM_MODE) return "secondary"
  return "outline"
}

// ---------------------------------------------------------------------------
// Credential fields component
// ---------------------------------------------------------------------------

function CredentialFields({
  authMode,
  credentials,
  onChange,
}: {
  authMode: string
  credentials: Record<string, string>
  onChange: (c: Record<string, string>) => void
}) {
  const set = (key: string, value: string) =>
    onChange({ ...credentials, [key]: value })

  if (needsNoCredentials(authMode)) {
    return (
      <p className="text-sm text-muted-foreground">
        No credentials needed for this auth mode.
      </p>
    )
  }

  if (needsOAuthFields(authMode)) {
    return (
      <div className="space-y-3">
        <div className="space-y-2">
          <Label>Client ID</Label>
          <Input
            value={credentials.client_id ?? ""}
            onChange={(e) => set("client_id", e.target.value)}
            placeholder="Client ID"
          />
        </div>
        <div className="space-y-2">
          <Label>Client Secret</Label>
          <Input
            type="password"
            value={credentials.client_secret ?? ""}
            onChange={(e) => set("client_secret", e.target.value)}
            placeholder="Client Secret"
          />
        </div>
      </div>
    )
  }

  if (needsAppFields(authMode)) {
    return (
      <div className="space-y-3">
        <div className="space-y-2">
          <Label>App ID</Label>
          <Input
            value={credentials.app_id ?? ""}
            onChange={(e) => set("app_id", e.target.value)}
            placeholder="App ID"
          />
        </div>
        <div className="space-y-2">
          <Label>App Link</Label>
          <Input
            value={credentials.app_link ?? ""}
            onChange={(e) => set("app_link", e.target.value)}
            placeholder="https://..."
          />
        </div>
        <div className="space-y-2">
          <Label>Private Key</Label>
          <Textarea
            value={credentials.private_key ?? ""}
            onChange={(e) => set("private_key", e.target.value)}
            placeholder="-----BEGIN RSA PRIVATE KEY-----"
            rows={4}
            className="min-w-0 resize-y"
          />
        </div>
      </div>
    )
  }

  if (needsInstallPluginFields(authMode)) {
    return (
      <div className="space-y-2">
        <Label>App Link</Label>
        <Input
          value={credentials.app_link ?? ""}
          onChange={(e) => set("app_link", e.target.value)}
          placeholder="https://..."
        />
      </div>
    )
  }

  if (needsCustomFields(authMode)) {
    return (
      <div className="space-y-3">
        <div className="space-y-2">
          <Label>Client ID</Label>
          <Input
            value={credentials.client_id ?? ""}
            onChange={(e) => set("client_id", e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>Client Secret</Label>
          <Input
            type="password"
            value={credentials.client_secret ?? ""}
            onChange={(e) => set("client_secret", e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>App ID</Label>
          <Input
            value={credentials.app_id ?? ""}
            onChange={(e) => set("app_id", e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>App Link</Label>
          <Input
            value={credentials.app_link ?? ""}
            onChange={(e) => set("app_link", e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>Private Key</Label>
          <Textarea
            value={credentials.private_key ?? ""}
            onChange={(e) => set("private_key", e.target.value)}
            rows={3}
            className="min-w-0 resize-y"
          />
        </div>
      </div>
    )
  }

  // Fallback for unknown modes
  return (
    <p className="text-sm text-muted-foreground">
      No credentials needed for this auth mode.
    </p>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function InIntegrationsPage() {
  const queryClient = useQueryClient()

  // ---- List state ----
  const { data, isLoading } = $api.useQuery("get", "/admin/v1/in-integrations")
  const integrations: InIntegration[] = (data as { data?: InIntegration[] })?.data ?? []

  // ---- Create dialog state ----
  const [createOpen, setCreateOpen] = useState(false)
  const [createStep, setCreateStep] = useState<1 | 2>(1)
  const [providerSearch, setProviderSearch] = useState("")
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null)
  const [createDisplayName, setCreateDisplayName] = useState("")
  const [createCredentials, setCreateCredentials] = useState<Record<string, string>>({})
  const [createMeta, setCreateMeta] = useState("")
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSaving, setCreateSaving] = useState(false)

  // ---- Edit dialog state ----
  const [editingIntegration, setEditingIntegration] = useState<InIntegration | null>(null)
  const [editDisplayName, setEditDisplayName] = useState("")
  const [editMeta, setEditMeta] = useState("")
  const [editCredentials, setEditCredentials] = useState<Record<string, string>>({})
  const [editCredentialsOpen, setEditCredentialsOpen] = useState(false)
  const [editError, setEditError] = useState<string | null>(null)
  const [editSaving, setEditSaving] = useState(false)

  // ---- Detail dialog state ----
  const [detailIntegration, setDetailIntegration] = useState<InIntegration | null>(null)

  // ---- Delete state ----
  const [deletingId, setDeletingId] = useState<string | null>(null)

  // ---- Providers query (only when create dialog is open) ----
  const { data: providersRaw, isLoading: providersLoading } = $api.useQuery(
    "get",
    "/admin/v1/in-integration-providers",
    {},
    { enabled: createOpen }
  )
  const providers: Provider[] = (providersRaw as unknown as Provider[]) ?? []

  const filteredProviders = useMemo(() => {
    if (!providerSearch.trim()) return providers
    const q = providerSearch.toLowerCase()
    return providers.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.display_name.toLowerCase().includes(q)
    )
  }, [providers, providerSearch])

  // ---- Handlers ----

  function openCreateDialog() {
    setCreateStep(1)
    setSelectedProvider(null)
    setProviderSearch("")
    setCreateDisplayName("")
    setCreateCredentials({})
    setCreateMeta("")
    setCreateError(null)
    setCreateOpen(true)
  }

  function selectProvider(provider: Provider) {
    setSelectedProvider(provider)
    setCreateDisplayName(provider.display_name)
    setCreateCredentials({ type: provider.auth_mode })
    setCreateStep(2)
    setCreateError(null)
  }

  async function handleCreate() {
    if (!selectedProvider) return
    setCreateSaving(true)
    setCreateError(null)
    try {
      let metaObj: Record<string, unknown> | undefined
      if (createMeta.trim()) {
        try {
          metaObj = JSON.parse(createMeta)
        } catch {
          setCreateError("Meta must be valid JSON.")
          setCreateSaving(false)
          return
        }
      }

      const creds: Record<string, string> = { ...createCredentials }
      if (!creds.type) creds.type = selectedProvider.auth_mode

      const res = await api.POST("/admin/v1/in-integrations", {
        body: {
          provider: selectedProvider.name,
          display_name: createDisplayName,
          credentials: creds,
          meta: metaObj,
        },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to create integration."
        setCreateError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/in-integrations"] })
      setCreateOpen(false)
    } catch {
      setCreateError("An unexpected error occurred.")
    } finally {
      setCreateSaving(false)
    }
  }

  function openEditDialog(integration: InIntegration) {
    setEditingIntegration(integration)
    setEditDisplayName(integration.display_name ?? "")
    setEditMeta(integration.meta ? JSON.stringify(integration.meta, null, 2) : "")
    setEditCredentials({})
    setEditCredentialsOpen(false)
    setEditError(null)
  }

  async function handleEdit() {
    if (!editingIntegration?.id) return
    setEditSaving(true)
    setEditError(null)
    try {
      let metaObj: Record<string, unknown> | undefined
      if (editMeta.trim()) {
        try {
          metaObj = JSON.parse(editMeta)
        } catch {
          setEditError("Meta must be valid JSON.")
          setEditSaving(false)
          return
        }
      }

      const hasCredentials = Object.values(editCredentials).some((v) => v.trim() !== "")

      const res = await api.PUT("/admin/v1/in-integrations/{id}", {
        params: { path: { id: editingIntegration.id } },
        body: {
          display_name: editDisplayName,
          meta: metaObj,
          ...(hasCredentials ? { credentials: editCredentials } : {}),
        },
      })
      if (res.error) {
        const msg = (res.error as { message?: string }).message || "Failed to update integration."
        setEditError(msg)
        return
      }
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/in-integrations"] })
      setEditingIntegration(null)
    } catch {
      setEditError("An unexpected error occurred.")
    } finally {
      setEditSaving(false)
    }
  }

  async function handleDelete(id: string) {
    setDeletingId(id)
    try {
      await api.DELETE("/admin/v1/in-integrations/{id}", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/in-integrations"] })
    } finally {
      setDeletingId(null)
    }
  }

  // ---- Derive auth mode from nango_config for display ----
  function getAuthMode(integration: InIntegration): string {
    const cfg = integration.nango_config
    if (cfg && typeof cfg === "object" && "auth_mode" in cfg) {
      return String(cfg.auth_mode)
    }
    return "--"
  }

  // ---- Render ----

  return (
    <div className="space-y-6">
      <PageHeader
        title="Platform Integrations"
        description="Manage platform-owned OAuth integrations that end users can connect to."
        actions={
          <Button onClick={openCreateDialog}>Create Integration</Button>
        }
      />

      {/* ---- List ---- */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : integrations.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No platform integrations</EmptyTitle>
            <EmptyDescription>
              Create your first platform integration to let end users connect their accounts.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Provider</TableHead>
                <TableHead>Display Name</TableHead>
                <TableHead>Unique Key</TableHead>
                <TableHead>Auth Mode</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {integrations.map((integration) => {
                const authMode = getAuthMode(integration)
                return (
                  <TableRow
                    key={integration.id}
                    className="cursor-pointer"
                    onClick={() => setDetailIntegration(integration)}
                  >
                    <TableCell className="font-medium">
                      {integration.provider ?? "--"}
                    </TableCell>
                    <TableCell>{integration.display_name ?? "--"}</TableCell>
                    <TableCell className="font-mono text-xs">
                      {integration.unique_key ?? "--"}
                    </TableCell>
                    <TableCell>
                      <Badge variant={authModeBadgeVariant(authMode)} className="text-xs px-1.5 py-0">
                        {authMode}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      <TimeAgo date={integration.created_at} />
                    </TableCell>
                    <TableCell className="text-right">
                      <div
                        className="flex items-center justify-end gap-2"
                        onClick={(e) => e.stopPropagation()}
                      >
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDetailIntegration(integration)}
                        >
                          Config
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => openEditDialog(integration)}
                        >
                          Edit
                        </Button>
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button
                              variant="destructive"
                              size="sm"
                              disabled={deletingId === integration.id}
                            >
                              {deletingId === integration.id ? "Deleting..." : "Delete"}
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Delete integration?</AlertDialogTitle>
                              <AlertDialogDescription>
                                This will permanently delete the &quot;{integration.display_name}&quot; integration
                                and remove it from Nango. This action cannot be undone.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Cancel</AlertDialogCancel>
                              <AlertDialogAction
                                variant="destructive"
                                onClick={() => handleDelete(integration.id!)}
                              >
                                Delete
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {/* ---- Create Dialog ---- */}
      <Dialog open={createOpen} onOpenChange={(open) => !open && setCreateOpen(false)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {createStep === 1 ? "Select a provider" : `Configure ${selectedProvider?.display_name}`}
            </DialogTitle>
            <DialogDescription>
              {createStep === 1
                ? "Choose the OAuth provider for this platform integration."
                : "Enter the credentials and details for this integration."}
            </DialogDescription>
          </DialogHeader>

          {createStep === 1 && (
            <div className="space-y-4">
              <Input
                placeholder="Search providers..."
                value={providerSearch}
                onChange={(e) => setProviderSearch(e.target.value)}
              />
              {providersLoading ? (
                <div className="space-y-2">
                  {Array.from({ length: 4 }).map((_, i) => (
                    <Skeleton key={i} className="h-14 w-full" />
                  ))}
                </div>
              ) : filteredProviders.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  No providers found.
                </p>
              ) : (
                <div className="grid max-h-80 grid-cols-1 gap-2 overflow-y-auto">
                  {filteredProviders.map((provider) => (
                    <button
                      key={provider.name}
                      type="button"
                      className="flex items-center justify-between rounded-lg border border-border p-3 text-left transition-colors hover:bg-muted"
                      onClick={() => selectProvider(provider)}
                    >
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">{provider.display_name}</p>
                        <p className="truncate text-xs text-muted-foreground">{provider.name}</p>
                      </div>
                      <Badge
                        variant={authModeBadgeVariant(provider.auth_mode)}
                        className="ml-2 shrink-0 text-xs px-1.5 py-0"
                      >
                        {provider.auth_mode}
                      </Badge>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          {createStep === 2 && selectedProvider && (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Display Name</Label>
                <Input
                  value={createDisplayName}
                  onChange={(e) => setCreateDisplayName(e.target.value)}
                  placeholder="Display name"
                />
              </div>

              <div className="space-y-2">
                <Label>Credentials</Label>
                <CredentialFields
                  authMode={selectedProvider.auth_mode}
                  credentials={createCredentials}
                  onChange={setCreateCredentials}
                />
              </div>

              <div className="space-y-2">
                <Label>Meta (optional JSON)</Label>
                <Textarea
                  value={createMeta}
                  onChange={(e) => setCreateMeta(e.target.value)}
                  placeholder='{"key": "value"}'
                  rows={3}
                  className="min-w-0 resize-y"
                />
              </div>

              {createError && (
                <p className="text-sm text-destructive">{createError}</p>
              )}
            </div>
          )}

          {createStep === 2 && (
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setCreateStep(1)
                  setSelectedProvider(null)
                  setCreateError(null)
                }}
              >
                Back
              </Button>
              <Button onClick={handleCreate} disabled={createSaving}>
                {createSaving ? "Creating..." : "Create"}
              </Button>
            </DialogFooter>
          )}
        </DialogContent>
      </Dialog>

      {/* ---- Edit Dialog ---- */}
      <Dialog
        open={!!editingIntegration}
        onOpenChange={(open) => !open && setEditingIntegration(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit integration</DialogTitle>
            <DialogDescription>
              Update the display name, metadata, or credentials for this integration.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>Display Name</Label>
              <Input
                value={editDisplayName}
                onChange={(e) => setEditDisplayName(e.target.value)}
                placeholder="Display name"
              />
            </div>
            <div className="space-y-2">
              <Label>Meta (JSON)</Label>
              <Textarea
                value={editMeta}
                onChange={(e) => setEditMeta(e.target.value)}
                placeholder='{"key": "value"}'
                rows={3}
              />
            </div>

            <Collapsible open={editCredentialsOpen} onOpenChange={setEditCredentialsOpen}>
              <CollapsibleTrigger asChild>
                <Button variant="outline" size="sm" className="w-full">
                  {editCredentialsOpen ? "Hide credentials" : "Update credentials"}
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="mt-3 space-y-3">
                <p className="text-xs text-muted-foreground">
                  Leave fields empty to keep existing values. Only filled fields will be updated.
                </p>
                <CredentialFields
                  authMode={getAuthMode(editingIntegration!)}
                  credentials={editCredentials}
                  onChange={setEditCredentials}
                />
              </CollapsibleContent>
            </Collapsible>

            {editError && (
              <p className="text-sm text-destructive">{editError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditingIntegration(null)}>
              Cancel
            </Button>
            <Button onClick={handleEdit} disabled={editSaving}>
              {editSaving ? "Saving..." : "Save changes"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ---- Detail Dialog ---- */}
      <Dialog
        open={!!detailIntegration}
        onOpenChange={(open) => !open && setDetailIntegration(null)}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Integration details</DialogTitle>
            <DialogDescription>
              Full details for {detailIntegration?.display_name ?? detailIntegration?.provider ?? "this integration"}.
            </DialogDescription>
          </DialogHeader>
          {detailIntegration && (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-xs font-medium text-muted-foreground">ID</p>
                  <p className="mt-1 font-mono text-sm">{detailIntegration.id ?? "--"}</p>
                </div>
                <div>
                  <p className="text-xs font-medium text-muted-foreground">Provider</p>
                  <p className="mt-1 text-sm">{detailIntegration.provider ?? "--"}</p>
                </div>
                <div>
                  <p className="text-xs font-medium text-muted-foreground">Display Name</p>
                  <p className="mt-1 text-sm">{detailIntegration.display_name ?? "--"}</p>
                </div>
                <div>
                  <p className="text-xs font-medium text-muted-foreground">Unique Key</p>
                  <p className="mt-1 font-mono text-sm">{detailIntegration.unique_key ?? "--"}</p>
                </div>
                <div>
                  <p className="text-xs font-medium text-muted-foreground">Auth Mode</p>
                  <div className="mt-1">
                    <Badge variant={authModeBadgeVariant(getAuthMode(detailIntegration))} className="text-xs px-1.5 py-0">
                      {getAuthMode(detailIntegration)}
                    </Badge>
                  </div>
                </div>
                <div>
                  <p className="text-xs font-medium text-muted-foreground">Created</p>
                  <p className="mt-1 text-sm">
                    <TimeAgo date={detailIntegration.created_at} />
                  </p>
                </div>
                {detailIntegration.updated_at && (
                  <div>
                    <p className="text-xs font-medium text-muted-foreground">Updated</p>
                    <p className="mt-1 text-sm">
                      <TimeAgo date={detailIntegration.updated_at} />
                    </p>
                  </div>
                )}
              </div>

              {detailIntegration.meta && Object.keys(detailIntegration.meta).length > 0 && (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">Meta</p>
                  <pre className="max-h-48 overflow-auto rounded-lg bg-muted p-3 text-xs">
                    {JSON.stringify(detailIntegration.meta, null, 2)}
                  </pre>
                </div>
              )}

              {detailIntegration.nango_config && (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">Nango Config</p>
                  <pre className="max-h-64 overflow-auto rounded-lg bg-muted p-3 text-xs">
                    {JSON.stringify(detailIntegration.nango_config, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDetailIntegration(null)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
