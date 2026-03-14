"use client";

import { useState } from "react";
import { X, CircleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { useQueryClient } from "@tanstack/react-query";
import { $api } from "@/api/client";
import { SCOPE_OPTIONS, EXPIRY_OPTIONS, type CreateAPIKeyResult } from "./utils";

export function CreateAPIKeyDialog({ onCancel, onSuccess }: { onCancel: () => void; onSuccess: (result: CreateAPIKeyResult) => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["all"]);
  const [expiresIn, setExpiresIn] = useState("");
  const createMutation = $api.useMutation("post", "/v1/api-keys", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["get", "/v1/api-keys"] }),
  });

  function toggleScope(scope: string) {
    if (scope === "all") {
      setScopes(["all"]);
      return;
    }
    const withoutAll = scopes.filter((s) => s !== "all");
    if (withoutAll.includes(scope)) {
      const next = withoutAll.filter((s) => s !== scope);
      setScopes(next.length === 0 ? ["all"] : next);
    } else {
      setScopes([...withoutAll, scope]);
    }
  }

  function handleSubmit() {
    createMutation.mutate(
      { body: { name, scopes, ...(expiresIn ? { expires_in: expiresIn } : {}) } },
      { onSuccess: (data) => onSuccess(data) },
    );
  }

  return (
    <DialogContent className="sm:max-w-130 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <DialogTitle className="font-mono text-lg font-semibold">Create API Key</DialogTitle>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Create an API key for programmatic access to the LLMVault API. The key will be shown only once after creation.
      </DialogDescription>

      {createMutation.error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{createMutation.error instanceof Error ? createMutation.error.message : "Failed to create API key"}</span>
        </div>
      )}

      <div className="flex flex-col gap-4.5">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="key-name" className="text-xs">
            Name <span className="text-destructive">*</span>
          </Label>
          <Input
            id="key-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="h-10"
            placeholder="e.g. Production API Key"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">
            Scopes <span className="text-destructive">*</span>
          </Label>
          <div className="flex flex-wrap gap-2">
            {SCOPE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() => toggleScope(opt.value)}
                className={`border px-3 py-1.5 text-[13px] font-medium transition-colors ${
                  scopes.includes(opt.value)
                    ? "border-primary bg-primary/8 text-chart-2"
                    : "border-border text-muted-foreground hover:text-foreground"
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
          <span className="text-[11px] text-dim">
            Controls which API endpoints this key can access.
          </span>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="expires-in" className="text-xs">Expiration</Label>
          <Select value={expiresIn} onValueChange={(v) => setExpiresIn(v ?? "")}>
            <SelectTrigger className="h-10 w-full">
              <SelectValue placeholder="Never" />
            </SelectTrigger>
            <SelectContent>
              {EXPIRY_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onCancel} disabled={createMutation.isPending}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={!name} loading={createMutation.isPending}>
          Create Key
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
