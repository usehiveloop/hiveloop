"use client";

import { Unplug } from "lucide-react";

export default function ConnectionsPage() {
  return (
    <>
      <header className="flex shrink-0 items-center justify-between border-b border-border px-4 py-4 sm:px-6 lg:px-8 lg:py-5">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Connections
        </h1>
      </header>
      <div className="flex flex-1 flex-col items-center justify-center py-24">
        <div className="flex flex-col items-center gap-6 max-w-sm text-center">
          <div className="flex size-16 items-center justify-center rounded-full border border-border bg-card">
            <Unplug className="size-7 text-dim" />
          </div>
          <div className="flex flex-col gap-2">
            <span className="font-mono text-[15px] font-medium text-foreground">
              Coming soon
            </span>
            <span className="text-[13px] leading-5 text-muted-foreground">
              Connect to LLM providers and SaaS apps. Each connection gets its own MCP URL.
            </span>
          </div>
        </div>
      </div>
    </>
  );
}
