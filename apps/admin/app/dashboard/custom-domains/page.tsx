"use client"

import { useState } from "react"
import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/admin/page-header"
import { StatusBadge } from "@/components/admin/status-badge"
import { TimeAgo } from "@/components/admin/time-ago"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
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

export default function CustomDomainsPage() {
  const [orgId, setOrgId] = useState("")
  const [tab, setTab] = useState("all")
  const queryClient = useQueryClient()

  const verifiedFilter = tab === "all" ? undefined : tab === "verified" ? "true" : "false"

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/custom-domains", {
    params: {
      query: {
        org_id: orgId || undefined,
        verified: verifiedFilter,
      },
    },
  })

  const domains = data?.data ?? []

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/custom-domains/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/custom-domains"] })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Custom Domains"
        description="Custom domains across all organizations."
      />

      <div className="flex items-center gap-3">
        <Input
          placeholder="Filter by Org ID..."
          value={orgId}
          onChange={(e) => setOrgId(e.target.value)}
          className="max-w-xs"
        />
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="verified">Verified</TabsTrigger>
          <TabsTrigger value="unverified">Unverified</TabsTrigger>
        </TabsList>

        <TabsContent value={tab}>
          {isLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : domains.length === 0 ? (
            <div className="flex h-48 items-center justify-center rounded-lg border border-border">
              <p className="text-sm text-muted-foreground">No custom domains found.</p>
            </div>
          ) : (
            <div className="rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Domain</TableHead>
                    <TableHead>Org ID</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Verified At</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {domains.map((domain) => (
                    <TableRow key={domain.id}>
                      <TableCell className="font-medium">
                        {domain.domain ?? "--"}
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {domain.org_id ?? "--"}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={domain.verified ? "verified" : "pending"} />
                      </TableCell>
                      <TableCell>
                        <TimeAgo date={domain.verified_at} />
                      </TableCell>
                      <TableCell>
                        <TimeAgo date={domain.created_at} />
                      </TableCell>
                      <TableCell className="text-right">
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button variant="destructive" size="xs">
                              Delete
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>Delete domain?</AlertDialogTitle>
                              <AlertDialogDescription>
                                This will permanently delete the custom domain &quot;{domain.domain}&quot;. This action cannot be undone.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Cancel</AlertDialogCancel>
                              <AlertDialogAction
                                variant="destructive"
                                onClick={() => handleDelete(domain.id!)}
                              >
                                Delete
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}
