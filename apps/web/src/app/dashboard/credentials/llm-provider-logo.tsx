"use client";

import { useState } from "react";

/**
 * Extracts the domain from an API base URL for favicon lookup.
 * e.g. "https://api.openai.com/v1" → "openai.com"
 */
function extractDomain(apiUrl: string): string | null {
  try {
    const hostname = new URL(apiUrl).hostname;
    // Strip "api." prefix if present (api.openai.com → openai.com)
    return hostname.replace(/^api\./, "");
  } catch {
    return null;
  }
}

export function LLMProviderLogo({
  providerId,
  apiUrl,
  size = "size-9",
}: {
  providerId: string;
  apiUrl?: string;
  size?: string;
}) {
  const [errored, setErrored] = useState(false);
  const domain = apiUrl ? extractDomain(apiUrl) : null;
  const src = domain ? `https://www.google.com/s2/favicons?domain=${domain}&sz=64` : null;

  return (
    <div
      className={`shrink-0 rounded-lg bg-secondary ${size} flex items-center justify-center overflow-hidden`}
    >
      {!src || errored ? (
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
