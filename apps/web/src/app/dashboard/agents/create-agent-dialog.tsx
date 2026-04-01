"use client";

import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { $api, fetchClient } from "@/api/client";
import type { components } from "@/api/schema";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { CircleAlert, ChevronDown, X } from "lucide-react";

type AgentRequest = components["schemas"]["createAgentRequest"];

function FieldHint({ children }: { children: React.ReactNode }) {
  return <p className="text-[11px] leading-snug text-muted-foreground">{children}</p>;
}

export function CreateAgentDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();

  // Form state
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [identityId, setIdentityId] = useState("");
  const [credentialId, setCredentialId] = useState("");
  const [model, setModel] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [sandboxType, setSandboxType] = useState<"shared" | "dedicated">("shared");
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Fetch identities
  const { data: identitiesData } = $api.useQuery("get", "/v1/identities", {
    params: { query: { limit: 100 } },
  });
  const identities = identitiesData?.data ?? [];

  // Fetch credentials
  const { data: credentialsData } = $api.useQuery("get", "/v1/credentials", {
    params: { query: { limit: 100 } },
  });
  const credentials = credentialsData?.data ?? [];

  // Get selected credential's provider
  const selectedCredential = credentials.find((c: { id?: string }) => c.id === credentialId);
  const providerId = selectedCredential?.provider_id ?? "";

  // Fetch models for the selected provider
  const { data: modelsData } = $api.useQuery(
    "get",
    "/v1/providers/{id}/models",
    { params: { path: { id: providerId } } },
    { enabled: !!providerId }
  );
  const models = modelsData ?? [];

  // Reset model when credential changes
  useEffect(() => {
    setModel("");
  }, [credentialId]);

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setName("");
      setDescription("");
      setIdentityId("");
      setCredentialId("");
      setModel("");
      setSystemPrompt("");
      setSandboxType("shared");
      setShowAdvanced(false);
    }
  }, [open]);

  const mutation = useMutation({
    mutationFn: async () => {
      const body: AgentRequest = {
        name,
        identity_id: identityId,
        credential_id: credentialId,
        model,
        system_prompt: systemPrompt,
        sandbox_type: sandboxType,
      };
      if (description) body.description = description;

      const { error } = await fetchClient.POST("/v1/agents", { body });
      if (error) throw new Error((error as { error?: string }).error ?? "Failed to create agent");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] });
      onOpenChange(false);
    },
  });

  const canSubmit = name && identityId && credentialId && model && systemPrompt;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-160 max-h-[85vh] gap-6 overflow-y-auto p-7"
        showCloseButton={false}
      >
        <DialogHeader className="flex-row items-center justify-between space-y-0">
          <div>
            <DialogTitle>Create Agent</DialogTitle>
            <DialogDescription>
              Configure an AI agent with a model, identity, and sandbox environment.
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

        <div className="flex flex-col gap-4.5">
          {/* Name */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="agent-name" className="text-xs">
              Name
            </Label>
            <Input
              id="agent-name"
              placeholder="My Agent"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="h-10"
            />
          </div>

          {/* Identity */}
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">Identity</Label>
            <Select value={identityId} onValueChange={(v) => setIdentityId(v ?? "")}>
              <SelectTrigger className="h-10">
                <SelectValue placeholder="Select an identity" />
              </SelectTrigger>
              <SelectContent>
                {identities.map((identity) => (
                  <SelectItem key={identity.id} value={identity.id ?? ""}>
                    {identity.external_id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldHint>The user identity this agent acts on behalf of.</FieldHint>
          </div>

          {/* Credential */}
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">Credential</Label>
            <Select value={credentialId} onValueChange={(v) => setCredentialId(v ?? "")}>
              <SelectTrigger className="h-10">
                <SelectValue placeholder="Select a credential" />
              </SelectTrigger>
              <SelectContent>
                {credentials.map((cred) => (
                  <SelectItem key={cred.id} value={cred.id ?? ""}>
                    {cred.label || cred.provider_id} ({cred.provider_id})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldHint>The API key used to call the LLM provider.</FieldHint>
          </div>

          {/* Model */}
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">Model</Label>
            <Select value={model} onValueChange={(v) => setModel(v ?? "")} disabled={!providerId}>
              <SelectTrigger className="h-10">
                <SelectValue
                  placeholder={providerId ? "Select a model" : "Select a credential first"}
                />
              </SelectTrigger>
              <SelectContent>
                {(models as { id?: string; name?: string }[]).map((m) => (
                  <SelectItem key={m.id} value={m.id ?? ""}>
                    {m.name ?? m.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* System Prompt */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="system-prompt" className="text-xs">
              System Prompt
            </Label>
            <Textarea
              id="system-prompt"
              placeholder="You are a helpful assistant..."
              value={systemPrompt}
              onChange={(e) => setSystemPrompt(e.target.value)}
              className="min-h-24"
            />
          </div>

          {/* Sandbox Type */}
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">Sandbox Type</Label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setSandboxType("shared")}
                className={`flex-1 border px-4 py-2.5 text-[13px] transition-colors ${
                  sandboxType === "shared"
                    ? "border-primary/50 bg-primary/8 text-foreground"
                    : "border-border bg-card text-muted-foreground hover:text-foreground"
                }`}
              >
                <div className="font-medium">Shared</div>
                <div className="text-[11px] text-muted-foreground">
                  Reuse sandbox across conversations
                </div>
              </button>
              <button
                type="button"
                onClick={() => setSandboxType("dedicated")}
                className={`flex-1 border px-4 py-2.5 text-[13px] transition-colors ${
                  sandboxType === "dedicated"
                    ? "border-primary/50 bg-primary/8 text-foreground"
                    : "border-border bg-card text-muted-foreground hover:text-foreground"
                }`}
              >
                <div className="font-medium">Dedicated</div>
                <div className="text-[11px] text-muted-foreground">
                  New sandbox per conversation
                </div>
              </button>
            </div>
          </div>
        </div>

        {/* Advanced settings */}
        <div className="border-t border-border pt-2">
          <button
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="flex w-full items-center justify-between py-2 text-sm font-medium text-foreground"
          >
            Advanced settings
            <ChevronDown
              className={`size-4 transition-transform ${showAdvanced ? "rotate-180" : ""}`}
            />
          </button>
          {showAdvanced && (
            <div className="flex flex-col gap-4.5 pb-1 pt-3">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="agent-desc" className="text-xs">
                  Description <span className="text-muted-foreground">(optional)</span>
                </Label>
                <Input
                  id="agent-desc"
                  placeholder="A brief description of this agent"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className="h-10"
                />
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            loading={mutation.isPending}
            disabled={!canSubmit}
          >
            Create Agent
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
