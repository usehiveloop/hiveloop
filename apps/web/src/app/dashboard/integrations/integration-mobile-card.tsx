"use client";

import { MoreHorizontal } from "lucide-react";
import { ProviderBadge } from "@/components/provider-badge";
import { formatDate, type IntegrationResponse } from "./utils";

export function IntegrationMobileCard({
  integration,
  onEdit,
  onDelete,
}: {
  integration: IntegrationResponse;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <div className="flex flex-col gap-1">
          <span className="text-[13px] font-medium text-foreground">
            {integration.display_name}
          </span>
          <ProviderBadge provider={integration.provider} />
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={onEdit}
            className="px-2 py-1 text-xs text-muted-foreground hover:text-foreground"
          >
            Edit
          </button>
          <button
            onClick={onDelete}
            className="text-dim hover:text-foreground"
          >
            <MoreHorizontal className="size-4" />
          </button>
        </div>
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          Created {integration.created_at ? formatDate(integration.created_at) : ""}
        </span>
        <span>
          Updated {integration.updated_at ? formatDate(integration.updated_at) : ""}
        </span>
      </div>
    </div>
  );
}
