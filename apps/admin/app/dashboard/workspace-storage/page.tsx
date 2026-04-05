"use client"

import { $api } from "@/lib/api/hooks"
import { api } from "@/lib/api/client"
import { useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/admin/page-header"
import { TimeAgo } from "@/components/admin/time-ago"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
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

export default function WorkspaceStoragePage() {
  const queryClient = useQueryClient()

  const { data, isLoading } = $api.useQuery("get", "/admin/v1/workspace-storage")

  const items = ((data as { data?: Record<string, unknown>[] })?.data ?? []) as {
    id?: string
    org_id?: string
    created_at?: string
  }[]

  async function handleDelete(id: string) {
    await api.DELETE("/admin/v1/workspace-storage/{id}", {
      params: { path: { id } },
    })
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/v1/workspace-storage"] })
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Workspace Storage"
        description="Provisioned workspace databases."
      />

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex h-48 items-center justify-center rounded-lg border border-border">
          <p className="text-sm text-muted-foreground">No workspace storage found.</p>
        </div>
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Org ID</TableHead>
                <TableHead>Created</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="font-mono text-xs">
                    {item.id ?? "--"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {item.org_id ?? "--"}
                  </TableCell>
                  <TableCell>
                    <TimeAgo date={item.created_at} />
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
                          <AlertDialogTitle>Delete workspace storage?</AlertDialogTitle>
                          <AlertDialogDescription>
                            This will permanently delete this workspace database record. This action cannot be undone.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction
                            variant="destructive"
                            onClick={() => handleDelete(item.id!)}
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
    </div>
  )
}
