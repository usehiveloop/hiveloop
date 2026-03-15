"use client";

import { useState } from "react";
import { X, CircleAlert } from "lucide-react";
import { useQuery, useMutation } from "@tanstack/react-query";
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
import { updateIntegration, listProviders } from "./api";
import { credentialFieldsForAuthMode } from "./credential-config";
import { CredentialFieldsForm } from "./credential-fields-form";
import type { IntegrationResponse, NangoProvider } from "./utils";

export function EditIntegrationDialog({
  integration,
  onCancel,
  onSuccess,
}: {
  integration: IntegrationResponse;
  onCancel: () => void;
  onSuccess: (result: IntegrationResponse) => void;
}) {
  const [displayName, setDisplayName] = useState(integration.display_name);
  const [showCredentials, setShowCredentials] = useState(false);
  const [credentials, setCredentials] = useState<Record<string, string>>({});
  const [webhookSecret, setWebhookSecret] = useState("");

  const { data: providers = [] } = useQuery<NangoProvider[]>({
    queryKey: ["nango-providers"],
    queryFn: listProviders,
    staleTime: 5 * 60 * 1000,
  });

  const provider = providers.find((p) => p.name === integration.provider);
  const credConfig = provider
    ? credentialFieldsForAuthMode(provider.auth_mode)
    : { fields: [] };

  const mutation = useMutation({
    mutationFn: (body: {
      display_name?: string;
      credentials?: Record<string, string>;
    }) => updateIntegration(integration.id!, body),
    onSuccess: (result) => onSuccess(result),
  });

  function handleSubmit() {
    const body: {
      display_name?: string;
      credentials?: Record<string, string>;
      webhook_secret?: string;
    } = {};

    if (displayName !== integration.display_name) {
      body.display_name = displayName;
    }

    if (showCredentials && provider && credConfig.fields.length > 0) {
      const creds: Record<string, string> = { type: provider.auth_mode };
      for (const f of credConfig.fields) {
        if (credentials[f.key]) {
          creds[f.key] = credentials[f.key];
        }
      }
      if (Object.keys(creds).length > 1) {
        body.credentials = creds;
      }
    }

    if (webhookSecret && provider?.webhook_user_defined_secret) {
      body.webhook_secret = webhookSecret;
    }

    mutation.mutate(body);
  }

  const hasChanges =
    displayName !== integration.display_name ||
    (showCredentials && Object.values(credentials).some((v) => v !== "")) ||
    webhookSecret !== "";

  return (
    <DialogContent className="sm:max-w-130 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <DialogTitle className="font-mono text-lg font-semibold">
          Edit Integration
        </DialogTitle>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Update the display name
        {credConfig.fields.length > 0 ? " or rotate credentials" : ""} for{" "}
        <strong className="font-mono">{integration.provider}</strong>{" "}
        integration.
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
          <Label className="text-xs">Provider</Label>
          <div className="flex items-center gap-2">
            <Input
              value={integration.provider}
              disabled
              className="h-10 font-mono text-[13px]"
            />
            {provider && (
              <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                {provider.auth_mode}
              </span>
            )}
          </div>
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="edit-display-name" className="text-xs">
            Display Name <span className="text-destructive">*</span>
          </Label>
          <Input
            id="edit-display-name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            className="h-10"
            placeholder="e.g. Slack Production"
          />
        </div>

        {credConfig.fields.length > 0 && (
          <div className="border-t border-border pt-4">
            {!showCredentials ? (
              <button
                type="button"
                onClick={() => setShowCredentials(true)}
                className="text-xs font-medium text-primary hover:underline"
              >
                Rotate credentials
              </button>
            ) : (
              <div className="flex flex-col gap-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-muted-foreground">
                    New Credentials
                  </span>
                  <button
                    type="button"
                    onClick={() => {
                      setShowCredentials(false);
                      setCredentials({});
                    }}
                    className="text-[11px] text-muted-foreground hover:text-foreground"
                  >
                    Cancel rotation
                  </button>
                </div>
                <CredentialFieldsForm
                  config={credConfig}
                  values={credentials}
                  onChange={(key, value) =>
                    setCredentials((prev) => ({ ...prev, [key]: value }))
                  }
                  idPrefix="edit-cred"
                  placeholderPrefix="Enter new"
                />
              </div>
            )}
          </div>
        )}

        {provider?.webhook_user_defined_secret && (
          <div className="border-t border-border pt-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="edit-webhook-secret" className="text-xs">
                Webhook Secret
              </Label>
              <Input
                id="edit-webhook-secret"
                value={webhookSecret}
                onChange={(e) => setWebhookSecret(e.target.value)}
                className="h-10 font-mono text-[13px]"
                placeholder={
                  (integration.nango_config as Record<string, unknown>)
                    ?.webhook_secret
                    ? "Current secret set — enter new to replace"
                    : "Paste webhook secret from provider"
                }
              />
              <span className="text-[11px] text-muted-foreground">
                Found in your provider&apos;s webhook settings.
              </span>
            </div>
          </div>
        )}
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button
          variant="outline"
          onClick={onCancel}
          disabled={mutation.isPending}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={!displayName || !hasChanges}
          loading={mutation.isPending}
        >
          Save Changes
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
