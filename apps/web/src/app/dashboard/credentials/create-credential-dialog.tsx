"use client";

import { useState } from "react";
import { X, CircleAlert, ArrowLeft } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { fetchClient } from "@/api/client";
import { CredentialProviderPicker } from "./credential-provider-picker";
import { LLMProviderLogo } from "./llm-provider-logo";
import type { components } from "@/api/schema";

type ProviderSummary = components["schemas"]["providerSummary"];

export function CreateCredentialDialog({
  onCancel,
  onSuccess,
}: {
  onCancel: () => void;
  onSuccess: () => void;
}) {
  const [step, setStep] = useState<"select" | "configure">("select");
  const [selectedProvider, setSelectedProvider] =
    useState<ProviderSummary | null>(null);

  const [label, setLabel] = useState("");
  const [apiKey, setApiKey] = useState("");

  const mutation = useMutation({
    mutationFn: async () => {
      const body: components["schemas"]["createCredentialRequest"] = {
        provider_id: selectedProvider?.id ?? "",
        api_key: apiKey,
      };
      if (label) body.label = label;

      const { error } = await fetchClient.POST("/v1/credentials", {
        body,
      });
      if (error) throw new Error((error as { error?: string }).error ?? "Failed to create credential");
    },
    onSuccess: () => onSuccess(),
  });

  function handleSelectProvider(p: ProviderSummary) {
    setSelectedProvider(p);
    setLabel(p.name ?? "");
    setApiKey("");
    mutation.reset();
    setStep("configure");
  }

  function handleBack() {
    setStep("select");
    mutation.reset();
  }

  function handleSubmit() {
    if (!selectedProvider || !apiKey) return;
    mutation.mutate();
  }

  if (step === "select") {
    return (
      <CredentialProviderPicker
        onSelect={handleSelectProvider}
        onCancel={onCancel}
      />
    );
  }

  return (
    <DialogContent className="sm:max-w-140 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-3">
          <button
            onClick={handleBack}
            className="text-dim hover:text-foreground"
          >
            <ArrowLeft className="size-4" />
          </button>
          <div className="flex items-center gap-2.5">
            <LLMProviderLogo
              providerId={selectedProvider!.id ?? ""}
              apiUrl={selectedProvider!.api}
              size="size-7"
            />
            <DialogTitle className="font-mono text-lg font-semibold">
              {selectedProvider!.name}
            </DialogTitle>
          </div>
        </div>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Enter your API key for this provider.
      </DialogDescription>

      {mutation.error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">
            {mutation.error.message}
          </span>
        </div>
      )}

      <div className="flex flex-col gap-4.5">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="cred-label" className="text-xs">
            Label <span className="text-muted-foreground">(optional)</span>
          </Label>
          <Input
            id="cred-label"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            className="h-10"
            placeholder="e.g. Production key"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="cred-api-key" className="text-xs">
            API Key <span className="text-destructive">*</span>
          </Label>
          <Input
            id="cred-api-key"
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            className="h-10 font-mono text-[13px]"
            placeholder="Enter your API key"
            autoFocus
          />
        </div>
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button
          variant="outline"
          onClick={handleBack}
          disabled={mutation.isPending}
        >
          Back
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={!apiKey}
          loading={mutation.isPending}
        >
          Create Credential
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
