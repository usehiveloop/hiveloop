"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
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
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty"

const AUTH_SCHEMES = ["bearer", "x-api-key", "api-key", "query_param"] as const

const EMPTY_FORM = {
  label: "",
  provider_id: "",
  api_key: "",
  base_url: "",
  auth_scheme: "",
}

export default function SystemCredentialsPage() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useState("all")
  const [revokingId, setRevokingId] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState(EMPTY_FORM)
  const [createError, setCreateError] = useState<string | null>(null)
  const [createSaving, setCreateSaving] = useState(false)

  const { data, isLoading, error } = $api.useQuery(
    "get",
    "/admin/v1/system-credentials",
  )

  const credentials = (data ?? []).filter((c) => {
    if (tab === "active") return !c.revoked_at
    if (tab === "revoked") return !!c.revoked_at
    return true
  })

  async function handleRevoke(id: string) {
    setRevokingId(id)
    try {
      await api.POST("/admin/v1/system-credentials/{id}/revoke", {
        params: { path: { id } },
      })
      queryClient.invalidateQueries({
        queryKey: ["get", "/admin/v1/system-credentials"],
      })
    } finally {
      setRevokingId(null)
    }
  }

  function openCreateDialog() {
    setCreateForm(EMPTY_FORM)
    setCreateError(null)
    setCreateOpen(true)
  }

  async function handleCreate() {
    if (!createForm.provider_id.trim() || !createForm.api_key.trim()) {
      setCreateError("Provider ID and API key are required.")
      return
    }
    setCreateSaving(true)
    setCreateError(null)
    try {
      const res = await api.POST("/admin/v1/system-credentials", {
        body: {
          label: createForm.label || undefined,
          provider_id: createForm.provider_id.trim(),
          api_key: createForm.api_key,
          base_url: createForm.base_url.trim() || undefined,
          auth_scheme: createForm.auth_scheme || undefined,
        },
      })
      if (res.error) {
        const msg =
          (res.error as { error?: string; message?: string }).error ||
          (res.error as { message?: string }).message ||
          "Failed to create system credential."
        setCreateError(msg)
        return
      }
      queryClient.invalidateQueries({
        queryKey: ["get", "/admin/v1/system-credentials"],
      })
      setCreateOpen(false)
    } catch {
      setCreateError("An unexpected error occurred.")
    } finally {
      setCreateSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="System Credentials"
        description="Platform-owned credentials used by agents that opted out of BYOK."
      />

      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="active">Active</TabsTrigger>
            <TabsTrigger value="revoked">Revoked</TabsTrigger>
          </TabsList>
        </Tabs>

        <Button onClick={openCreateDialog}>Add credential</Button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          Failed to load system credentials. Please try again.
        </div>
      ) : credentials.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No system credentials</EmptyTitle>
            <EmptyDescription>
              {tab !== "all"
                ? "No credentials match the current filter."
                : "Create one to allow agents that opted out of BYOK to use platform-owned keys."}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Label</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>Auth scheme</TableHead>
                <TableHead>Base URL</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {credentials.map((cred) => (
                <TableRow key={cred.id}>
                  <TableCell className="font-medium">
                    {cred.label || (
                      <span className="text-muted-foreground">--</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {cred.provider_id || "--"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {cred.auth_scheme || "--"}
                  </TableCell>
                  <TableCell className="max-w-[260px] truncate font-mono text-xs text-muted-foreground">
                    {cred.base_url || "--"}
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      status={cred.revoked_at ? "revoked" : "active"}
                    />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={cred.created_at} />
                  </TableCell>
                  <TableCell className="text-right">
                    {!cred.revoked_at && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={revokingId === cred.id}
                          >
                            {revokingId === cred.id ? "Revoking..." : "Revoke"}
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>
                              Revoke system credential
                            </AlertDialogTitle>
                            <AlertDialogDescription>
                              Agents that picked this credential will fail their
                              next resolution and fall back to another system
                              credential. If none remain they will return 503.
                              This cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() => handleRevoke(cred.id!)}
                            >
                              Revoke
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          if (!open) setCreateError(null)
          setCreateOpen(open)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add system credential</DialogTitle>
            <DialogDescription>
              The API key is encrypted at rest and cannot be retrieved after
              creation.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="create-label">Label</Label>
              <Input
                id="create-label"
                value={createForm.label}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, label: e.target.value }))
                }
                placeholder="e.g. Anthropic primary"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-provider">
                Provider ID <span className="text-destructive">*</span>
              </Label>
              <Input
                id="create-provider"
                value={createForm.provider_id}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, provider_id: e.target.value }))
                }
                placeholder="e.g. anthropic, openai"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-api-key">
                API key <span className="text-destructive">*</span>
              </Label>
              <Input
                id="create-api-key"
                type="password"
                autoComplete="off"
                value={createForm.api_key}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, api_key: e.target.value }))
                }
                placeholder="sk-..."
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-base-url">Base URL</Label>
              <Input
                id="create-base-url"
                value={createForm.base_url}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, base_url: e.target.value }))
                }
                placeholder="Defaults to provider's registered URL"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-auth-scheme">Auth scheme</Label>
              <Select
                value={createForm.auth_scheme}
                onValueChange={(v) =>
                  setCreateForm((f) => ({ ...f, auth_scheme: v }))
                }
              >
                <SelectTrigger id="create-auth-scheme" className="w-full">
                  <SelectValue placeholder="Default for provider" />
                </SelectTrigger>
                <SelectContent>
                  {AUTH_SCHEMES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {s}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {createError && (
              <p className="text-sm text-destructive">{createError}</p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleCreate} disabled={createSaving}>
              {createSaving ? "Creating..." : "Create credential"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
