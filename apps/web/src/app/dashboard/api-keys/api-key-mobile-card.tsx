"use client";

import { MoreHorizontal } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { StatusBadge, type Status } from "@/components/status-badge";
import { formatDate, relativeTime, type APIKeyResponse } from "./utils";

export function APIKeyMobileCard({ apiKey, onRevoke }: { apiKey: APIKeyResponse & { status: Status }; onRevoke: () => void }) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col">
          <span className="text-[13px] font-medium text-foreground">{apiKey.name}</span>
          <span className="font-mono text-[11px] text-dim">{apiKey.key_prefix}...&bull;&bull;&bull;&bull;</span>
        </div>
        <div className="flex items-center gap-2">
          <StatusBadge status={apiKey.status} />
          {apiKey.status !== "Revoked" && (
            <button onClick={onRevoke} className="text-dim hover:text-foreground">
              <MoreHorizontal className="size-4" />
            </button>
          )}
        </div>
      </div>
      <div className="flex flex-wrap gap-1">
        {(apiKey.scopes ?? []).map((s) => (
          <Badge key={s} variant="outline" className="text-[11px] font-normal">{s}</Badge>
        ))}
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>Created {apiKey.created_at ? formatDate(apiKey.created_at) : ""}</span>
        <span>{apiKey.last_used_at ? `Used ${relativeTime(apiKey.last_used_at)}` : "Never used"}</span>
      </div>
    </div>
  );
}
