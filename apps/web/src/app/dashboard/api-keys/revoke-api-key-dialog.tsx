"use client";

import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import type { APIKeyResponse } from "./utils";

export function RevokeAPIKeyDialog({
  target,
  isPending,
  onClose,
  onConfirm,
}: {
  target: APIKeyResponse | null;
  isPending: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <DialogContent className="sm:max-w-100 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <DialogTitle className="font-mono text-lg font-semibold">Revoke API Key</DialogTitle>
        <button onClick={onClose} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>
      <DialogDescription>
        Are you sure you want to revoke <strong>{target?.name}</strong>? This action cannot be undone. Any applications using this key will immediately lose access.
      </DialogDescription>
      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onClose} disabled={isPending}>Cancel</Button>
        <Button variant="destructive" onClick={onConfirm} loading={isPending}>
          Revoke Key
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
