"use client";

import { useState } from "react";
import { X, Copy, Check, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { ProviderLogo } from "./provider-logo";
import type { IntegrationResponse } from "./utils";

function getAuthModeCategory(
  authMode: string,
): "oauth" | "app" | "minimal" {
  switch (authMode) {
    case "OAUTH2":
    case "OAUTH1":
    case "TBA":
    case "MCP_OAUTH2":
      return "oauth";
    case "APP":
    case "CUSTOM":
      return "app";
    default:
      return "minimal";
  }
}

function CopyableField({
  label,
  value,
  fieldKey,
  copied,
  onCopy,
}: {
  label: string;
  value: string;
  fieldKey: string;
  copied: string | null;
  onCopy: (value: string, key: string) => void;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <Label className="text-xs">{label}</Label>
      <div className="flex items-center gap-2 overflow-hidden border border-border bg-code px-3 py-3">
        <div className="min-w-0 flex-1 overflow-x-auto">
          <span className="whitespace-nowrap font-mono text-xs leading-4 text-foreground">
            {value}
          </span>
        </div>
        <Button
          size="sm"
          onClick={() => onCopy(value, fieldKey)}
          className="shrink-0 gap-1.5"
        >
          {copied === fieldKey ? (
            <Check className="size-3.5" />
          ) : (
            <Copy className="size-3.5" />
          )}
          Copy
        </Button>
      </div>
    </div>
  );
}

export function IntegrationCreatedDialog({
  result,
  onClose,
}: {
  result: IntegrationResponse | null;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState<string | null>(null);

  if (!result) return null;

  function handleCopy(text: string, key: string) {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  }

  const config = (result.nango_config ?? {}) as Record<string, unknown>;
  const authMode = typeof config.auth_mode === "string" ? config.auth_mode : "";
  const category = getAuthModeCategory(authMode);

  const callbackUrl = typeof config.callback_url === "string" ? config.callback_url : "";
  const webhookUrl = typeof config.webhook_url === "string" ? config.webhook_url : "";
  const webhookSecret = typeof config.webhook_secret === "string" ? config.webhook_secret : "";
  const setupGuideUrl = typeof config.setup_guide_url === "string" ? config.setup_guide_url : "";
  const docsUrl = typeof config.docs === "string" ? config.docs : "";

  const setupUrl = callbackUrl
    ? callbackUrl.replace("/oauth/callback", "/app-auth/connect")
    : "";

  const bannerMessages: Record<string, string> = {
    oauth:
      "Copy the callback URL and add it to your OAuth app's redirect URI settings.",
    app: "Copy the URLs below and configure them in your app settings.",
    minimal:
      "Your integration is ready. Users can now connect through the Connect widget.",
  };

  const guideLink = setupGuideUrl || docsUrl;

  return (
    <DialogContent
      className="sm:max-w-140 gap-6 overflow-hidden p-7"
      showCloseButton={false}
    >
      <div className="flex items-start justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <Badge
            variant="outline"
            className="flex size-8 shrink-0 items-center justify-center border-success/20 bg-success/10 p-0"
          >
            <Check className="size-4 text-success-foreground" />
          </Badge>
          <DialogHeader className="min-w-0 space-y-0.5">
            <DialogTitle className="font-mono text-lg font-semibold">
              Integration Created
            </DialogTitle>
            <DialogDescription className="flex items-center gap-2 text-[13px]">
              <ProviderLogo
                providerId={result.provider ?? ""}
                size="size-4"
              />
              {result.display_name}
            </DialogDescription>
          </DialogHeader>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 text-dim hover:text-foreground"
        >
          <X className="size-4" />
        </button>
      </div>

      <div className="flex items-center gap-2 border border-chart-2/13 bg-chart-2/5 px-3 py-2.5">
        <Info className="size-3.5 shrink-0 text-chart-2" />
        <span className="text-xs text-chart-2">
          {bannerMessages[category]}
        </span>
      </div>

      <div className="flex flex-col gap-4">
        {category === "app" && setupUrl && (
          <CopyableField
            label="Setup URL"
            value={setupUrl}
            fieldKey="setup_url"
            copied={copied}
            onCopy={handleCopy}
          />
        )}

        {(category === "oauth" || category === "app") && callbackUrl && (
          <CopyableField
            label="Callback URL"
            value={callbackUrl}
            fieldKey="callback_url"
            copied={copied}
            onCopy={handleCopy}
          />
        )}

        {webhookUrl && (
          <CopyableField
            label="Webhook URL"
            value={webhookUrl}
            fieldKey="webhook_url"
            copied={copied}
            onCopy={handleCopy}
          />
        )}

        {webhookSecret && (
          <CopyableField
            label="Webhook Secret"
            value={webhookSecret}
            fieldKey="webhook_secret"
            copied={copied}
            onCopy={handleCopy}
          />
        )}

        {guideLink && (
          <a
            href={guideLink}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-chart-2 hover:underline"
          >
            View setup guide
          </a>
        )}
      </div>

      <DialogFooter className="justify-end rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button onClick={onClose}>Done</Button>
      </DialogFooter>
    </DialogContent>
  );
}
