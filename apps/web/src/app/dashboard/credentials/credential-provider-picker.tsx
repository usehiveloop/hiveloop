"use client";

import { useState, useMemo, useRef } from "react";
import { X, Search, ChevronRight } from "lucide-react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Input } from "@/components/ui/input";
import {
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { $api } from "@/api/client";
import { LLMProviderLogo } from "./llm-provider-logo";
import type { components } from "@/api/schema";

type ProviderSummary = components["schemas"]["providerSummary"];

const ROW_HEIGHT = 52;

function VirtualProviderList({
  providers,
  isLoading,
  onSelect,
}: {
  providers: ProviderSummary[];
  isLoading: boolean;
  onSelect: (p: ProviderSummary) => void;
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
    <div
      ref={scrollRef}
      className="h-[420px] overflow-y-auto border-t border-border"
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const p = providers[virtualRow.index];
          return (
            <button
              key={p.id}
              type="button"
              onClick={() => onSelect(p)}
              className="absolute left-0 flex w-full items-center gap-3.5 border-b border-border px-7 text-left transition-colors hover:bg-secondary/50"
              style={{
                height: virtualRow.size,
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <LLMProviderLogo providerId={p.id ?? ""} apiUrl={p.api} />
              <div className="flex grow flex-col gap-0.5">
                <span className="text-[14px] font-semibold leading-4.5 text-foreground">
                  {p.name}
                </span>
                <span className="text-xs leading-4 text-muted-foreground">
                  {p.model_count} {p.model_count === 1 ? "model" : "models"}
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

export function CredentialProviderPicker({
  onSelect,
  onCancel,
}: {
  onSelect: (p: ProviderSummary) => void;
  onCancel: () => void;
}) {
  const [search, setSearch] = useState("");

  const { data: providers = [], isLoading } = $api.useQuery(
    "get",
    "/v1/providers",
  );

  const filtered = useMemo(() => {
    if (!search) return providers;
    const q = search.toLowerCase();
    return providers.filter(
      (p) =>
        (p.name ?? "").toLowerCase().includes(q) ||
        (p.id ?? "").toLowerCase().includes(q),
    );
  }, [providers, search]);

  return (
    <DialogContent className="sm:max-w-140 gap-0 p-0" showCloseButton={false}>
      <div className="flex items-center justify-between px-7 pt-7 pb-4">
        <DialogHeader className="space-y-0 p-0">
          <DialogTitle className="font-mono text-lg font-semibold">
            Select a provider
          </DialogTitle>
          <DialogDescription className="mt-1 text-[13px]">
            Choose an LLM provider for your Llm key.
          </DialogDescription>
        </DialogHeader>
        <button onClick={onCancel} className="text-dim hover:text-foreground">
          <X className="size-4" />
        </button>
      </div>

      <div className="px-7 pb-4">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-dim" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-9 pl-9 font-mono text-[13px]"
            placeholder="Search providers..."
            autoFocus
          />
        </div>
      </div>

      <VirtualProviderList
        providers={filtered}
        isLoading={isLoading}
        onSelect={onSelect}
      />
    </DialogContent>
  );
}
