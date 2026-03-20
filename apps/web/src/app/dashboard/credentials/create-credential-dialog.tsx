"use client";

import { useState } from "react";
import { X, CircleAlert, ArrowLeft } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

const AUTH_SCHEMES = [
  { value: "bearer", label: "Bearer" },
  { value: "x-api-key", label: "X-API-Key" },
  { value: "api-key", label: "API-Key" },
  { value: "query_param", label: "Query Param" },
] as const;

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
  const [baseUrl, setBaseUrl] = useState("");
  const [authScheme, setAuthScheme] = useState("bearer");
  const [apiKey, setApiKey] = useState("");
  const [remaining, setRemaining] = useState("");
  const [refillAmount, setRefillAmount] = useState("");
  const [refillInterval, setRefillInterval] = useState("");

  const mutation = useMutation({
    mutationFn: async () => {
      const body: components["schemas"]["createCredentialRequest"] = {
        provider_id: selectedProvider?.id ?? "",
        base_url: baseUrl,
        auth_scheme: authScheme,
        api_key: apiKey,
      };
      if (label) body.label = label;
      if (remaining) body.remaining = Number(remaining);
      if (refillAmount) body.refill_amount = Number(refillAmount);
      if (refillInterval) body.refill_interval = refillInterval;

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
    setBaseUrl(p.api ?? "");
    setAuthScheme("bearer");
    setApiKey("");
    setRemaining("");
    setRefillAmount("");
    setRefillInterval("");
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
        Configure the credential details for this provider.
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
            Label
          </Label>
          <Input
            id="cred-label"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            className="h-10"
            placeholder="e.g. OpenAI Production"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="cred-base-url" className="text-xs">
            Base URL <span className="text-destructive">*</span>
          </Label>
          <Input
            id="cred-base-url"
            value={baseUrl}
            onChange={(e) => setBaseUrl(e.target.value)}
            className="h-10 font-mono text-[13px]"
            placeholder="https://api.openai.com/v1"
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">
            Auth Scheme <span className="text-destructive">*</span>
          </Label>
          <Select value={authScheme} onValueChange={(v) => v && setAuthScheme(v)}>
            <SelectTrigger className="h-10 w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {AUTH_SCHEMES.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
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
            placeholder="sk-..."
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="cred-remaining" className="text-xs">
            Remaining (request cap)
          </Label>
          <Input
            id="cred-remaining"
            type="number"
            value={remaining}
            onChange={(e) => setRemaining(e.target.value)}
            className="h-10"
            placeholder="e.g. 10000"
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="cred-refill-amount" className="text-xs">
              Refill Amount
            </Label>
            <Input
              id="cred-refill-amount"
              type="number"
              value={refillAmount}
              onChange={(e) => setRefillAmount(e.target.value)}
              className="h-10"
              placeholder="e.g. 10000"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="cred-refill-interval" className="text-xs">
              Refill Interval
            </Label>
            <Input
              id="cred-refill-interval"
              value={refillInterval}
              onChange={(e) => setRefillInterval(e.target.value)}
              className="h-10"
              placeholder="e.g. 24h"
            />
          </div>
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
          disabled={!apiKey || !baseUrl}
          loading={mutation.isPending}
        >
          Create Credential
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
