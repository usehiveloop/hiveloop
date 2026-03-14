"use client";

import { useState } from "react";
import { X, CircleAlert } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
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
import type { IntegrationResponse } from "./utils";

type CredField = { key: string; label: string; type: "input" | "textarea" };

function credentialFieldsForAuthMode(authMode: string): {
  fields: CredField[];
  optional?: string[];
} {
  switch (authMode) {
    case "OAUTH2":
    case "OAUTH1":
    case "TBA":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
          { key: "scopes", label: "Scopes", type: "input" },
        ],
        optional: ["scopes"],
      };
    case "APP":
      return {
        fields: [
          { key: "app_id", label: "App ID", type: "input" },
          { key: "app_link", label: "App Link", type: "input" },
          { key: "private_key", label: "Private Key", type: "textarea" },
        ],
      };
    case "CUSTOM":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
          { key: "app_id", label: "App ID", type: "input" },
          { key: "app_link", label: "App Link", type: "input" },
          { key: "private_key", label: "Private Key", type: "textarea" },
        ],
      };
    case "INSTALL_PLUGIN":
      return {
        fields: [{ key: "app_link", label: "App Link", type: "input" }],
      };
    case "MCP_OAUTH2":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
        ],
      };
    default:
      return { fields: [] };
  }
}

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
  const [error, setError] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);

  const { data: providers = [] } = useQuery({
    queryKey: ["nango-providers"],
    queryFn: listProviders,
    staleTime: 5 * 60 * 1000,
  });

  const provider = providers.find((p) => p.name === integration.provider);
  const credConfig = provider
    ? credentialFieldsForAuthMode(provider.auth_mode)
    : { fields: [] };

  function updateCredential(key: string, value: string) {
    setCredentials((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSubmit() {
    setError(null);
    setIsPending(true);
    try {
      const body: {
        display_name?: string;
        credentials?: Record<string, string>;
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

      const result = await updateIntegration(integration.id, body);
      onSuccess(result);
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Failed to update integration",
      );
    } finally {
      setIsPending(false);
    }
  }

  const hasChanges =
    displayName !== integration.display_name ||
    (showCredentials && Object.values(credentials).some((v) => v !== ""));

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

      {error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{error}</span>
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

        {/* Credential rotation */}
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
                {credConfig.fields.map((f) => {
                  const isOptional = credConfig.optional?.includes(f.key);
                  return (
                    <div key={f.key} className="flex flex-col gap-1.5">
                      <Label htmlFor={`edit-cred-${f.key}`} className="text-xs">
                        {f.label}{" "}
                        {!isOptional && (
                          <span className="text-destructive">*</span>
                        )}
                      </Label>
                      {f.type === "textarea" ? (
                        <textarea
                          id={`edit-cred-${f.key}`}
                          value={credentials[f.key] ?? ""}
                          onChange={(e) =>
                            updateCredential(f.key, e.target.value)
                          }
                          className="flex min-h-20 w-full border border-input bg-background px-3 py-2 font-mono text-[13px] ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                          placeholder={`Enter new ${f.label.toLowerCase()}...`}
                        />
                      ) : (
                        <Input
                          id={`edit-cred-${f.key}`}
                          value={credentials[f.key] ?? ""}
                          onChange={(e) =>
                            updateCredential(f.key, e.target.value)
                          }
                          className="h-10 font-mono text-[13px]"
                          placeholder={`Enter new ${f.label.toLowerCase()}...`}
                          type={
                            f.key.includes("secret") || f.key === "private_key"
                              ? "password"
                              : "text"
                          }
                        />
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onCancel} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={!displayName || !hasChanges}
          loading={isPending}
        >
          Save Changes
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
