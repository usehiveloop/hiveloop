"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import { ProviderBadge } from "@/components/provider-badge";
import { ProviderLogo } from "./provider-logo";
import { formatDate, type IntegrationResponse } from "./utils";

export function IntegrationMobileCard({
  integration,
  onDelete,
}: {
  integration: IntegrationResponse;
  onDelete: () => void;
}) {
  return (
    <div className="flex flex-col gap-3 border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <Link
          href={`/dashboard/integrations/${integration.id}`}
          className="flex items-center gap-3"
        >
          <ProviderLogo providerId={integration.provider ?? ""} size="size-8" />
          <div className="flex flex-col gap-1">
            <span className="text-[13px] font-medium text-foreground">
              {integration.display_name}
            </span>
            <ProviderBadge provider={integration.provider ?? ""} />
          </div>
        </Link>
        <button
          onClick={onDelete}
          className="px-2 py-1 text-dim hover:text-destructive"
        >
          <Trash2 className="size-3.5" />
        </button>
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
