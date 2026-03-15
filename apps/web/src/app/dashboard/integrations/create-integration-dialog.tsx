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
import { createIntegration } from "./api";
import { credentialFieldsForAuthMode } from "./credential-config";
import { CredentialFieldsForm } from "./credential-fields-form";
import { ProviderPicker } from "./provider-picker";
import { ProviderLogo } from "./provider-logo";
import type { IntegrationResponse, NangoProvider } from "./utils";

export function CreateIntegrationDialog({
  onCancel,
  onSuccess,
}: {
  onCancel: () => void;
  onSuccess: (result: IntegrationResponse) => void;
}) {
  const [step, setStep] = useState<"select" | "configure">("select");
  const [selectedProvider, setSelectedProvider] =
    useState<NangoProvider | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [credentials, setCredentials] = useState<Record<string, string>>({});
  const [webhookSecret, setWebhookSecret] = useState("");

  const credConfig = selectedProvider
    ? credentialFieldsForAuthMode(selectedProvider.auth_mode)
    : null;

  const mutation = useMutation({
    mutationFn: (body: {
      provider: string;
      display_name: string;
      credentials?: Record<string, string>;
    }) => createIntegration(body),
    onSuccess: (result) => onSuccess(result),
  });

  function handleSelectProvider(p: NangoProvider) {
    setSelectedProvider(p);
    setDisplayName(
      p.display_name || p.name.charAt(0).toUpperCase() + p.name.slice(1),
    );
    setCredentials({});
    setWebhookSecret("");
    mutation.reset();
    setStep("configure");
  }

  function handleBack() {
    setStep("select");
    mutation.reset();
  }

  function handleSubmit() {
    if (!selectedProvider) return;

    const body: {
      provider: string;
      display_name: string;
      credentials?: Record<string, string>;
      webhook_secret?: string;
    } = {
      provider: selectedProvider.name,
      display_name: displayName,
    };

    if (credConfig && credConfig.fields.length > 0) {
      const creds: Record<string, string> = {
        type: selectedProvider.auth_mode,
      };
      for (const f of credConfig.fields) {
        if (credentials[f.key]) {
          creds[f.key] = credentials[f.key];
        }
      }
      if (Object.keys(creds).length > 1) {
        body.credentials = creds;
      }
    }

    if (webhookSecret && selectedProvider.webhook_user_defined_secret) {
      body.webhook_secret = webhookSecret;
    }

    mutation.mutate(body);
  }

  if (step === "select") {
    return (
      <ProviderPicker onSelect={handleSelectProvider} onCancel={onCancel} />
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
            <ProviderLogo
              providerId={selectedProvider!.name}
              size="size-7"
            />
            <DialogTitle className="font-mono text-lg font-semibold">
              {selectedProvider!.display_name || selectedProvider!.name}
            </DialogTitle>
          </div>
        </div>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Configure the integration details and credentials.
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
          <Label htmlFor="display-name" className="text-xs">
            Display Name <span className="text-destructive">*</span>
          </Label>
          <Input
            id="display-name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            className="h-10"
            placeholder="e.g. Slack Production"
          />
        </div>

        {credConfig && (
          <CredentialFieldsForm
            config={credConfig}
            values={credentials}
            onChange={(key, value) =>
              setCredentials((prev) => ({ ...prev, [key]: value }))
            }
          />
        )}

        {selectedProvider?.webhook_user_defined_secret && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="webhook-secret" className="text-xs">
              Webhook Secret
            </Label>
            <Input
              id="webhook-secret"
              value={webhookSecret}
              onChange={(e) => setWebhookSecret(e.target.value)}
              className="h-10 font-mono text-[13px]"
              placeholder="Paste webhook secret from provider"
            />
            <span className="text-[11px] text-muted-foreground">
              Found in your provider&apos;s webhook settings.
            </span>
          </div>
        )}
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
          disabled={!selectedProvider || !displayName}
          loading={mutation.isPending}
        >
          Create Integration
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
