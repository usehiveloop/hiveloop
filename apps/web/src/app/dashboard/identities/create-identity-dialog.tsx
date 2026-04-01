"use client";

import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { fetchClient } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { CircleAlert, X } from "lucide-react";

export function CreateIdentityDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [externalId, setExternalId] = useState("");

  useEffect(() => {
    if (!open) setExternalId("");
  }, [open]);

  const mutation = useMutation({
    mutationFn: async () => {
      const { error } = await fetchClient.POST("/v1/identities", {
        body: { external_id: externalId },
      });
      if (error) throw new Error((error as { error?: string }).error ?? "Failed to create identity");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/identities"] });
      onOpenChange(false);
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-120 gap-6 p-7"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <DialogTitle>Create Identity</DialogTitle>
            <DialogDescription>
              Identities represent your end-users. Use them to scope agents and track usage.
            </DialogDescription>
          </div>
          <button
            onClick={() => onOpenChange(false)}
            className="text-muted-foreground hover:text-foreground"
          >
            <X className="size-5" />
          </button>
        </DialogHeader>

        {mutation.error && (
          <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
            <CircleAlert className="size-3.5 shrink-0 text-destructive" />
            <span className="text-xs text-destructive">
              {(mutation.error as Error).message}
            </span>
          </div>
        )}

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="external-id" className="text-xs">
            External ID
          </Label>
          <Input
            id="external-id"
            placeholder="user_123 or user@example.com"
            value={externalId}
            onChange={(e) => setExternalId(e.target.value)}
            className="h-10 font-mono"
          />
          <p className="text-[11px] leading-snug text-muted-foreground">
            A unique identifier for this user in your system (e.g., user ID or email).
          </p>
        </div>

        <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            loading={mutation.isPending}
            disabled={!externalId.trim()}
          >
            Create Identity
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
