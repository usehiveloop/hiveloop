"use client";

import { useState } from "react";
import { X, Copy, Check, CircleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { formatDate, type CreateAPIKeyResult } from "./utils";

export function KeyCreatedDialog({ keyResult, onClose }: { keyResult: CreateAPIKeyResult | null; onClose: () => void }) {
  const [copied, setCopied] = useState<string | null>(null);

  if (!keyResult) return null;

  function handleCopy(text: string, key: string) {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  }

  const exampleCode = `curl -H "Authorization: Bearer ${keyResult.key}" \\
    https://api.llmvault.dev/v1/connect/sessions \\
    -H "Content-Type: application/json" \\
    -d '{"allowed_providers":["openai"]}'`;

  return (
    <DialogContent className="sm:max-w-140 gap-6 overflow-hidden p-7" showCloseButton={false}>
      <div className="flex items-start justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <Badge variant="outline" className="flex size-8 shrink-0 items-center justify-center border-success/20 bg-success/10 p-0">
            <Check className="size-4 text-success-foreground" />
          </Badge>
          <DialogHeader className="min-w-0 space-y-0.5">
            <DialogTitle className="font-mono text-lg font-semibold">API Key Created</DialogTitle>
            <DialogDescription className="truncate text-[13px]">
              {keyResult.name} &middot; Scopes: {(keyResult.scopes ?? []).join(", ")}
              {keyResult.expires_at ? ` \u00B7 Expires ${formatDate(keyResult.expires_at)}` : ""}
            </DialogDescription>
          </DialogHeader>
        </div>
        <button onClick={onClose} className="shrink-0 text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </div>

      <div className="flex items-center gap-2 border border-warning/[0.13] bg-warning/5 px-3 py-2.5">
        <CircleAlert className="size-3.5 shrink-0 text-warning-foreground" />
        <span className="text-xs text-warning-foreground">This key is shown only once. Copy it now — you won&apos;t be able to see it again.</span>
      </div>

      <div className="flex min-w-0 flex-col gap-1.5">
        <Label className="text-xs">Your API Key</Label>
        <div className="flex items-center gap-2 overflow-hidden border border-border bg-code px-3 py-3">
          <div className="min-w-0 flex-1 overflow-x-auto">
            <span className="whitespace-nowrap font-mono text-xs leading-4 text-foreground">{keyResult.key}</span>
          </div>
          <Button size="sm" onClick={() => handleCopy(keyResult.key ?? "", "key")} className="shrink-0 gap-1.5">
            {copied === "key" ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
            Copy
          </Button>
        </div>
      </div>

      <div className="flex min-w-0 flex-col gap-1.5">
        <Label className="text-xs">Quick Start</Label>
        <p className="text-[13px] leading-4.5 text-muted-foreground">
          Use your API key to authenticate requests to the LLMVault management API:
        </p>
        <div className="overflow-hidden border border-border bg-code">
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <span className="font-mono text-[11px] text-dim">curl</span>
            <button onClick={() => handleCopy(exampleCode, "curl")} className="text-dim hover:text-foreground">
              {copied === "curl" ? <Check className="size-3" /> : <Copy className="size-3" />}
            </button>
          </div>
          <div className="overflow-x-auto px-3 py-3">
            <pre className="font-mono text-xs leading-5 text-muted-foreground">{exampleCode}</pre>
          </div>
        </div>
      </div>

      <DialogFooter className="justify-end rounded-none border-t border-border bg-transparent p-0 pt-4">
        <Button onClick={onClose}>Done</Button>
      </DialogFooter>
    </DialogContent>
  );
}
