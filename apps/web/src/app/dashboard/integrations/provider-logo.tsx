"use client";

import { useState } from "react";

const INTEGRATIONS_LOGO_BASE =
  process.env.NEXT_PUBLIC_INTEGRATIONS_API ?? "https://integrations.dev.llmvault.dev";

export function ProviderLogo({ providerId, size = "size-9" }: { providerId: string; size?: string }) {
  const [errored, setErrored] = useState(false);
  const src = `${INTEGRATIONS_LOGO_BASE}/images/template-logos/${providerId}.svg`;

  return (
    <div className={`shrink-0 rounded-lg bg-secondary ${size} flex items-center justify-center overflow-hidden`}>
      {errored ? (
        <span className="text-[11px] font-semibold uppercase text-muted-foreground">
          {providerId.slice(0, 2)}
        </span>
      ) : (
        <img
          src={src}
          alt=""
          className="h-3/5 w-3/5 object-contain"
          onError={() => setErrored(true)}
        />
      )}
    </div>
  );
}
