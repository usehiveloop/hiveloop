"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { PageHeader } from "@/components/admin/page-header"
import { TimeAgo } from "@/components/admin/time-ago"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

type AuditEntry = {
  id: number
  admin_id: string
  admin_email: string
  method: string
  path: string
  resource: string
  resource_id: string
  action: string
  status_code: number
  payload: Record<string, unknown>
  ip_address: string
  latency_ms: number
  created_at: string
}

const resourceOptions = [
  "users",
  "orgs",
  "credentials",
  "api-keys",
  "tokens",
  "identities",
  "agents",
  "sandboxes",
  "sandbox-templates",
  "conversations",
  "custom-domains",
  "workspace-storage",
]

function methodVariant(method: string): "default" | "secondary" | "destructive" | "outline" {
  switch (method) {
    case "POST":
      return "default"
    case "PUT":
      return "secondary"
    case "DELETE":
      return "destructive"
    default:
      return "outline"
  }
}

function statusVariant(code: number): "default" | "secondary" | "destructive" | "outline" {
  if (code >= 200 && code < 300) return "default"
  if (code >= 400 && code < 500) return "secondary"
  if (code >= 500) return "destructive"
  return "outline"
}

export default function AdminAuditPage() {
  const [resource, setResource] = useState("")
  const [action, setAction] = useState("")
  const [selectedEntry, setSelectedEntry] = useState<AuditEntry | null>(null)

  const { data, isLoading, error } = $api.useQuery("get", "/admin/v1/admin-audit", {
    params: {
      query: {
        ...(resource && resource !== "all" ? { resource } : {}),
        ...(action ? { action } : {}),
        limit: 100,
      },
    },
  })

  const entries = ((data as Record<string, unknown>)?.data ?? []) as AuditEntry[]

  return (
    <div className="space-y-6">
      <PageHeader
        title="Admin Audit Log"
        description="History of all mutating operations performed via the admin panel."
      />

      <div className="flex items-center gap-3">
        <Select value={resource || "all"} onValueChange={(v) => setResource(v === "all" ? "" : v)}>
          <SelectTrigger className="w-48">
            <SelectValue placeholder="All resources" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All resources</SelectItem>
            {resourceOptions.map((r) => (
              <SelectItem key={r} value={r}>
                {r}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Input
          placeholder="Filter by action..."
          value={action}
          onChange={(e) => setAction(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : error ? (
        <Card>
          <CardContent className="py-8 text-center text-sm text-destructive">
            Failed to load admin audit log.
          </CardContent>
        </Card>
      ) : entries.length === 0 ? (
        <Card>
          <CardContent className="py-12 text-center text-sm text-muted-foreground">
            No admin operations recorded yet.
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Method</TableHead>
                <TableHead>Resource</TableHead>
                <TableHead>Resource ID</TableHead>
                <TableHead>Admin</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Latency</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {entries.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell className="text-muted-foreground">
                    <TimeAgo date={entry.created_at} />
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {entry.action}
                  </TableCell>
                  <TableCell>
                    <Badge variant={methodVariant(entry.method)} className="font-mono text-xs">
                      {entry.method}
                    </Badge>
                  </TableCell>
                  <TableCell>{entry.resource}</TableCell>
                  <TableCell className="max-w-[120px] truncate font-mono text-xs text-muted-foreground">
                    {entry.resource_id || "—"}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {entry.admin_email}
                  </TableCell>
                  <TableCell>
                    <Badge variant={statusVariant(entry.status_code)} className="font-mono text-xs">
                      {entry.status_code}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right text-muted-foreground">
                    {entry.latency_ms}ms
                  </TableCell>
                  <TableCell>
                    {entry.payload && Object.keys(entry.payload).length > 0 && (
                      <Button
                        variant="ghost"
                        size="xs"
                        onClick={() => setSelectedEntry(entry)}
                      >
                        Payload
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={!!selectedEntry} onOpenChange={() => setSelectedEntry(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              Audit Payload — {selectedEntry?.action}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-2 text-sm">
              <div className="text-muted-foreground">Path</div>
              <div className="font-mono text-xs">{selectedEntry?.path}</div>
              <div className="text-muted-foreground">Resource ID</div>
              <div className="font-mono text-xs">{selectedEntry?.resource_id || "—"}</div>
              <div className="text-muted-foreground">Admin</div>
              <div>{selectedEntry?.admin_email}</div>
              <div className="text-muted-foreground">IP Address</div>
              <div className="font-mono text-xs">{selectedEntry?.ip_address || "—"}</div>
              <div className="text-muted-foreground">Status</div>
              <div>{selectedEntry?.status_code}</div>
              <div className="text-muted-foreground">Latency</div>
              <div>{selectedEntry?.latency_ms}ms</div>
            </div>
            <div>
              <div className="mb-1 text-sm text-muted-foreground">Sanitized Payload</div>
              <pre className="max-h-64 overflow-auto rounded-md bg-muted p-3 font-mono text-xs">
                {JSON.stringify(selectedEntry?.payload, null, 2)}
              </pre>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
