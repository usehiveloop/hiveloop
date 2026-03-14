"use client";

import { useState, useMemo, useRef } from "react";
import { X, CircleAlert, Search, ArrowLeft, ChevronRight } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
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
import { ProviderLogo } from "./provider-logo";
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

const ROW_HEIGHT = 52;

function VirtualProviderList({
  providers,
  isLoading,
  onSelect,
}: {
  providers: NangoProvider[];
  isLoading: boolean;
  onSelect: (p: NangoProvider) => void;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: providers.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center border-t border-border py-16">
        <div className="size-5 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
      </div>
    );
  }

  if (providers.length === 0) {
    return (
      <div className="border-t border-border py-16 text-center text-sm text-muted-foreground">
        No providers found
      </div>
    );
  }

  return (
    <div ref={scrollRef} className="h-[420px] overflow-y-auto border-t border-border">
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const p = providers[virtualRow.index];
          return (
            <button
              key={p.name}
              type="button"
              onClick={() => onSelect(p)}
              className="absolute left-0 flex w-full items-center gap-3.5 border-b border-border px-7 text-left transition-colors hover:bg-secondary/50"
              style={{
                height: virtualRow.size,
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <ProviderLogo providerId={p.name} />
              <div className="flex grow flex-col gap-0.5">
                <span className="text-[14px] font-semibold leading-4.5 text-foreground">
                  {p.display_name || p.name}
                </span>
                <span className="text-xs leading-4 text-muted-foreground">
                  {p.auth_mode}
                </span>
              </div>
              <ChevronRight className="size-4 shrink-0 text-dim" />
            </button>
          );
        })}
      </div>
    </div>
  );
}

export function CreateIntegrationDialog({
  onCancel,
  onSuccess,
}: {
  onCancel: () => void;
  onSuccess: (result: IntegrationResponse) => void;
}) {
  const [step, setStep] = useState<"select" | "configure">("select");
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
    if (!providerSearch) return providers;
    const q = providerSearch.toLowerCase();
    return providers.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.display_name.toLowerCase().includes(q),
    );
  }, [providers, providerSearch]);

  const credConfig = selectedProvider
    ? credentialFieldsForAuthMode(selectedProvider.auth_mode)
    : null;

  function handleSelectProvider(p: NangoProvider) {
    setSelectedProvider(p);
    setDisplayName(p.display_name || p.name.charAt(0).toUpperCase() + p.name.slice(1));
    setCredentials({});
    setError(null);
    setStep("configure");
  }

  function handleBack() {
    setStep("select");
    setError(null);
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

      if (credConfig && credConfig.fields.length > 0) {
        const creds: Record<string, string> = { type: selectedProvider.auth_mode };
        for (const f of credConfig.fields) {
          if (credentials[f.key]) {
            creds[f.key] = credentials[f.key];
          }
        }
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

  // ── Step 1: Provider selection ──
  if (step === "select") {
    return (
      <DialogContent className="sm:max-w-[560px] gap-0 p-0" showCloseButton={false}>
        <div className="flex items-center justify-between px-7 pt-7 pb-4">
          <DialogHeader className="space-y-0 p-0">
            <DialogTitle className="font-mono text-lg font-semibold">
              Select a provider
            </DialogTitle>
            <DialogDescription className="mt-1 text-[13px]">
              Choose a provider to create an integration with.
            </DialogDescription>
          </DialogHeader>
          <button onClick={onCancel} className="text-dim hover:text-foreground">
            <X className="size-4" />
          </button>
        </div>

        {/* Search */}
        <div className="px-7 pb-4">
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
        </div>

        {/* Provider list (virtualized) */}
        <VirtualProviderList
          providers={filteredProviders}
          isLoading={providersLoading}
          onSelect={handleSelectProvider}
        />
      </DialogContent>
    );
  }

  // ── Step 2: Configure integration ──
  return (
    <DialogContent className="sm:max-w-140 gap-6 p-7" showCloseButton={false}>
      <DialogHeader className="flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-3">
          <button onClick={handleBack} className="text-dim hover:text-foreground">
            <ArrowLeft className="size-4" />
          </button>
          <div className="flex items-center gap-2.5">
            <ProviderLogo providerId={selectedProvider!.name} size="size-7" />
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

      {error && (
        <div className="flex items-center gap-2 border border-destructive/20 bg-destructive/5 px-3 py-2.5">
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          <span className="text-xs text-destructive">{error}</span>
        </div>
      )}

      <div className="flex flex-col gap-4.5">
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
        <Button variant="outline" onClick={handleBack} disabled={isPending}>
          Back
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
