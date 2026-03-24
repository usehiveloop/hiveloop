"use client";

import { useState } from "react";
import { X, CircleAlert, ArrowLeft, ChevronDown } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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

function FieldHint({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[11px] leading-snug text-muted-foreground">{children}</p>
  );
}

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

  // Advanced fields
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [baseUrl, setBaseUrl] = useState("");
  const [authScheme, setAuthScheme] = useState("");
  const [identityId, setIdentityId] = useState("");
  const [externalId, setExternalId] = useState("");
  const [remaining, setRemaining] = useState("");
  const [refillAmount, setRefillAmount] = useState("");
  const [refillInterval, setRefillInterval] = useState("");
  const [meta, setMeta] = useState("");

  const mutation = useMutation({
    mutationFn: async () => {
      const body: components["schemas"]["createCredentialRequest"] = {
        provider_id: selectedProvider?.id ?? "",
        api_key: apiKey,
      };
      if (label) body.label = label;
      if (baseUrl) body.base_url = baseUrl;
      if (authScheme) body.auth_scheme = authScheme;
      if (identityId) body.identity_id = identityId;
      if (externalId) body.external_id = externalId;
      if (remaining) body.remaining = parseInt(remaining, 10);
      if (refillAmount) body.refill_amount = parseInt(refillAmount, 10);
      if (refillInterval) body.refill_interval = refillInterval;
      if (meta.trim()) {
        try {
          body.meta = JSON.parse(meta);
        } catch {
          throw new Error("Metadata must be valid JSON");
        }
      }

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
    setShowAdvanced(false);
    setBaseUrl("");
    setAuthScheme("");
    setIdentityId("");
    setExternalId("");
    setRemaining("");
    setRefillAmount("");
    setRefillInterval("");
    setMeta("");
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
    <DialogContent className="sm:max-w-140 max-h-[85vh] gap-6 overflow-y-auto p-7" showCloseButton={false}>
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

      {/* Advanced settings accordion */}
      <div className="border-t border-border pt-2">
        <button
          type="button"
          onClick={() => setShowAdvanced(!showAdvanced)}
          className="flex w-full items-center justify-between py-2 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
        >
          Advanced settings
          <ChevronDown
            className={`size-4 transition-transform duration-200 ${showAdvanced ? "rotate-180" : ""}`}
          />
        </button>

        {showAdvanced && (
          <div className="flex flex-col gap-4.5 pt-3 pb-1">
            {/* Connection settings */}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cred-base-url" className="text-xs">
                Base URL
              </Label>
              <Input
                id="cred-base-url"
                value={baseUrl}
                onChange={(e) => setBaseUrl(e.target.value)}
                className="h-10 font-mono text-[13px]"
                placeholder="e.g. https://api.openai.com/v1"
              />
              <FieldHint>
                The provider's API base URL. Auto-detected from the selected provider if left empty.
              </FieldHint>
            </div>

            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cred-auth-scheme" className="text-xs">
                Auth Scheme
              </Label>
              <Input
                id="cred-auth-scheme"
                value={authScheme}
                onChange={(e) => setAuthScheme(e.target.value)}
                className="h-10"
                placeholder="e.g. bearer, x-api-key, api-key, query_param"
              />
              <FieldHint>
                How the API key is sent to the provider. Defaults to the provider's standard scheme (usually <code className="text-[11px] bg-muted px-1 rounded">bearer</code>).
              </FieldHint>
            </div>

            {/* Identity linking */}
            <div className="border-t border-border pt-4">
              <p className="text-xs font-medium text-muted-foreground mb-3">Identity</p>
              <div className="flex flex-col gap-4.5">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="cred-identity-id" className="text-xs">
                    Identity ID
                  </Label>
                  <Input
                    id="cred-identity-id"
                    value={identityId}
                    onChange={(e) => setIdentityId(e.target.value)}
                    className="h-10 font-mono text-[13px]"
                    placeholder="e.g. 550e8400-e29b-41d4-a716-446655440000"
                  />
                  <FieldHint>
                    UUID of an existing identity to link this credential to. Used for shared rate limiting across credentials.
                  </FieldHint>
                </div>

                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="cred-external-id" className="text-xs">
                    External ID
                  </Label>
                  <Input
                    id="cred-external-id"
                    value={externalId}
                    onChange={(e) => setExternalId(e.target.value)}
                    className="h-10"
                    placeholder="e.g. user_12345"
                  />
                  <FieldHint>
                    Your own identifier for the end-user. If no identity exists with this external ID, one will be created automatically.
                  </FieldHint>
                </div>
              </div>
            </div>

            {/* Rate limiting */}
            <div className="border-t border-border pt-4">
              <p className="text-xs font-medium text-muted-foreground mb-3">Rate Limiting</p>
              <div className="flex flex-col gap-4.5">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="cred-remaining" className="text-xs">
                    Remaining Requests
                  </Label>
                  <Input
                    id="cred-remaining"
                    type="number"
                    value={remaining}
                    onChange={(e) => setRemaining(e.target.value)}
                    className="h-10"
                    placeholder="e.g. 1000"
                    min={0}
                  />
                  <FieldHint>
                    Maximum number of proxy requests allowed. Decremented with each request until the next refill.
                  </FieldHint>
                </div>

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
                    placeholder="e.g. 1000"
                    min={0}
                  />
                  <FieldHint>
                    Number of requests to restore when the refill interval elapses.
                  </FieldHint>
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
                    placeholder="e.g. 1h, 24h, 168h"
                  />
                  <FieldHint>
                    How often the remaining count resets to the refill amount. Uses Go duration format (e.g. <code className="text-[11px] bg-muted px-1 rounded">1h</code> = hourly, <code className="text-[11px] bg-muted px-1 rounded">24h</code> = daily).
                  </FieldHint>
                </div>
              </div>
            </div>

            {/* Metadata */}
            <div className="border-t border-border pt-4">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="cred-meta" className="text-xs">
                  Metadata
                </Label>
                <Textarea
                  id="cred-meta"
                  value={meta}
                  onChange={(e) => setMeta(e.target.value)}
                  className="font-mono text-[13px]"
                  placeholder={'{\n  "team": "ml-platform",\n  "environment": "production"\n}'}
                  rows={4}
                />
                <FieldHint>
                  Arbitrary JSON metadata to attach to this credential. Useful for tagging, filtering, or storing custom attributes.
                </FieldHint>
              </div>
            </div>
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
          disabled={!apiKey}
          loading={mutation.isPending}
        >
          Create Credential
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
