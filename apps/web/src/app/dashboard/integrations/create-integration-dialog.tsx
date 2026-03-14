"use client";

import { useState, useMemo } from "react";
import { X, CircleAlert, Search } from "lucide-react";
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
import { createIntegration, listProviders } from "./api";
import type { IntegrationResponse, NangoProvider } from "./utils";

type AuthMode = string;

function credentialFieldsForAuthMode(authMode: AuthMode): {
  fields: { key: string; label: string; type: "input" | "textarea" }[];
  optional?: string[];
  message?: string;
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
        fields: [
          { key: "app_link", label: "App Link", type: "input" },
        ],
      };
    case "MCP_OAUTH2":
      return {
        fields: [
          { key: "client_id", label: "Client ID", type: "input" },
          { key: "client_secret", label: "Client Secret", type: "input" },
        ],
        message: "Required for static client registration.",
      };
    default:
      return { fields: [], message: "No credentials required for this provider." };
  }
}

export function CreateIntegrationDialog({
  onCancel,
  onSuccess,
}: {
  onCancel: () => void;
  onSuccess: (result: IntegrationResponse) => void;
}) {
  const [providerSearch, setProviderSearch] = useState("");
  const [selectedProvider, setSelectedProvider] = useState<NangoProvider | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [credentials, setCredentials] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);

  const { data: providers = [], isLoading: providersLoading } = useQuery({
    queryKey: ["nango-providers"],
    queryFn: listProviders,
    staleTime: 5 * 60 * 1000,
  });

  const filteredProviders = useMemo(() => {
    if (!providerSearch) return providers.slice(0, 50);
    const q = providerSearch.toLowerCase();
    return providers
      .filter(
        (p) =>
          p.name.toLowerCase().includes(q) ||
          p.display_name.toLowerCase().includes(q),
      )
      .slice(0, 50);
  }, [providers, providerSearch]);

  const credConfig = selectedProvider
    ? credentialFieldsForAuthMode(selectedProvider.auth_mode)
    : null;

  function handleSelectProvider(p: NangoProvider) {
    setSelectedProvider(p);
    setCredentials({});
    if (!displayName) {
      setDisplayName(p.display_name || p.name.charAt(0).toUpperCase() + p.name.slice(1));
    }
  }

  function updateCredential(key: string, value: string) {
    setCredentials((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSubmit() {
    if (!selectedProvider) return;
    setError(null);
    setIsPending(true);
    try {
      const body: {
        provider: string;
        display_name: string;
        credentials?: Record<string, string>;
      } = {
        provider: selectedProvider.name,
        display_name: displayName,
      };

      // Build credentials if auth mode requires them
      if (credConfig && credConfig.fields.length > 0) {
        const creds: Record<string, string> = { type: selectedProvider.auth_mode };
        for (const f of credConfig.fields) {
          if (credentials[f.key]) {
            creds[f.key] = credentials[f.key];
          }
        }
        // Only send credentials if there's more than just the type
        if (Object.keys(creds).length > 1) {
          body.credentials = creds;
        }
      }

      const result = await createIntegration(body);
      onSuccess(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create integration");
    } finally {
      setIsPending(false);
    }
  }

  return (
    <DialogContent className="sm:max-w-140 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <DialogTitle className="font-mono text-lg font-semibold">
          Add Integration
        </DialogTitle>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </DialogHeader>

      <DialogDescription>
        Connect a third-party provider to enable OAuth connections for your
        users.
      </DialogDescription>

      {error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{error}</span>
        </div>
      )}

      <div className="flex flex-col gap-4.5">
        {/* Provider picker */}
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">
            Provider <span className="text-destructive">*</span>
          </Label>

          {selectedProvider ? (
            <div className="flex items-center justify-between border border-primary bg-primary/5 px-3 py-2">
              <div className="flex items-center gap-2">
                <span className="font-mono text-[13px] font-medium text-foreground">
                  {selectedProvider.name}
                </span>
                <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
                  {selectedProvider.auth_mode}
                </span>
              </div>
              <button
                type="button"
                onClick={() => {
                  setSelectedProvider(null);
                  setCredentials({});
                  setProviderSearch("");
                }}
                className="text-xs text-muted-foreground hover:text-foreground"
              >
                Change
              </button>
            </div>
          ) : (
            <>
              <div className="relative">
                <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
                <Input
                  value={providerSearch}
                  onChange={(e) => setProviderSearch(e.target.value)}
                  className="h-9 pl-9 font-mono text-[13px]"
                  placeholder="Search providers..."
                  autoFocus
                />
              </div>
              <div className="max-h-40 overflow-y-auto border border-border">
                {providersLoading ? (
                  <div className="px-3 py-4 text-center text-xs text-muted-foreground">
                    Loading providers...
                  </div>
                ) : filteredProviders.length === 0 ? (
                  <div className="px-3 py-4 text-center text-xs text-muted-foreground">
                    No providers found
                  </div>
                ) : (
                  filteredProviders.map((p) => (
                    <button
                      key={p.name}
                      type="button"
                      onClick={() => handleSelectProvider(p)}
                      className="flex w-full items-center justify-between px-3 py-2 text-left text-[13px] hover:bg-muted/50"
                    >
                      <span className="font-mono font-medium">{p.name}</span>
                      <span className="text-[11px] text-muted-foreground">
                        {p.auth_mode}
                      </span>
                    </button>
                  ))
                )}
              </div>
            </>
          )}
        </div>

        {/* Display name */}
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

        {/* Dynamic credential fields */}
        {selectedProvider && credConfig && (
          <>
            {credConfig.fields.length > 0 ? (
              <div className="flex flex-col gap-3 border-t border-border pt-4">
                <span className="text-xs font-medium text-muted-foreground">
                  Credentials
                </span>
                {credConfig.message && (
                  <span className="text-[11px] text-dim">{credConfig.message}</span>
                )}
                {credConfig.fields.map((f) => {
                  const isOptional = credConfig.optional?.includes(f.key);
                  return (
                    <div key={f.key} className="flex flex-col gap-1.5">
                      <Label htmlFor={`cred-${f.key}`} className="text-xs">
                        {f.label}{" "}
                        {!isOptional && (
                          <span className="text-destructive">*</span>
                        )}
                      </Label>
                      {f.type === "textarea" ? (
                        <textarea
                          id={`cred-${f.key}`}
                          value={credentials[f.key] ?? ""}
                          onChange={(e) => updateCredential(f.key, e.target.value)}
                          className="flex min-h-20 w-full border border-input bg-background px-3 py-2 font-mono text-[13px] ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                          placeholder={`Enter ${f.label.toLowerCase()}...`}
                        />
                      ) : (
                        <Input
                          id={`cred-${f.key}`}
                          value={credentials[f.key] ?? ""}
                          onChange={(e) => updateCredential(f.key, e.target.value)}
                          className="h-10 font-mono text-[13px]"
                          placeholder={`Enter ${f.label.toLowerCase()}...`}
                          type={f.key.includes("secret") || f.key === "private_key" ? "password" : "text"}
                        />
                      )}
                    </div>
                  );
                })}
              </div>
            ) : credConfig.message ? (
              <div className="border-t border-border pt-4">
                <span className="text-xs text-muted-foreground">
                  {credConfig.message}
                </span>
              </div>
            ) : null}
          </>
        )}
      </div>

      <DialogFooter className="flex-row justify-end gap-2.5 rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button variant="outline" onClick={onCancel} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          disabled={!selectedProvider || !displayName}
          loading={isPending}
        >
          Create Integration
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
