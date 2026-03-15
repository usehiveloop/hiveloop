"use client";

import { useState, useRef, useEffect } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { CheckIcon } from "@/components/icons";

export const pricingTiers = [
  { label: "500,000 requests", shortLabel: "500K", requests: 500_000, price: 20, perRequest: 0.00004, per1K: "$0.04 / 1K" },
  { label: "1,000,000 requests", shortLabel: "1M", requests: 1_000_000, price: 35, perRequest: 0.000035, per1K: "$0.035 / 1K" },
  { label: "2,500,000 requests", shortLabel: "2.5M", requests: 2_500_000, price: 75, perRequest: 0.00003, per1K: "$0.03 / 1K" },
  { label: "5,000,000 requests", shortLabel: "5M", requests: 5_000_000, price: 125, perRequest: 0.000025, per1K: "$0.025 / 1K" },
  { label: "10,000,000 requests", shortLabel: "10M", requests: 10_000_000, price: 200, perRequest: 0.00002, per1K: "$0.02 / 1K" },
];

const usageFeatures = [
  "Unlimited credentials",
  "Unlimited token mints",
  "All providers",
  "90-day audit log",
  "Email support (48h SLA)",
];

export function UsageTierCard({
  selectedIndex,
  onSelectTier,
}: {
  selectedIndex: number;
  onSelectTier: (index: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const selected = pricingTiers[selectedIndex];

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    document.addEventListener("mousedown", handleClickOutside);

    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  return (
    <div className="relative flex min-w-0 flex-1 flex-col gap-8 border-2 border-primary bg-surface p-11">
      <div className="absolute -top-3.25 left-1/2 -translate-x-1/2 bg-primary px-4 py-1">
        <span className="font-mono text-[11px] font-medium leading-3.5 tracking-[0.08em] uppercase text-white">
          Recommended
        </span>
      </div>

      <div className="flex flex-col gap-2.5">
        <span className="font-mono text-[13px] font-medium leading-4 tracking-wider uppercase text-primary">
          Usage
        </span>
        <span className="text-[15px] leading-5.5 text-[#9794A3]">
          Unlimited credentials. Pricing scales with your proxy volume.
        </span>
      </div>

      <div className="flex flex-col gap-3">
        <span className="text-[13px] font-medium leading-4 tracking-wider uppercase text-[#9794A3]">
          Monthly proxy requests
        </span>
        <div ref={ref} className="relative">
          <button
            onClick={() => setOpen(!open)}
            className="flex w-full items-center justify-between border border-[#3D3D47] bg-background px-4 py-3.5"
          >
            <span className="font-mono text-[15px] font-medium leading-4.5 text-[#E4E1EC]">
              {selected.label}
            </span>
            <svg
              width="16"
              height="16"
              viewBox="0 0 16 16"
              fill="none"
              className={`transition-transform ${open ? "rotate-180" : ""}`}
            >
              <path d="M4 6L8 10L12 6" stroke="#9794A3" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          {open && (
            <div className="absolute top-full left-0 z-10 mt-1 flex w-full flex-col border border-[#3D3D47] bg-background">
              {pricingTiers.map((tier, i) => (
                <button
                  key={tier.requests}
                  onClick={() => {
                    onSelectTier(i);
                    setOpen(false);
                  }}
                  className={`flex w-full items-center px-4 py-3 text-left font-mono text-[15px] font-medium leading-4.5 transition-colors ${
                    i === selectedIndex
                      ? "bg-primary/10 text-primary"
                      : "text-[#E4E1EC] hover:bg-[#2C2C35]"
                  }`}
                >
                  {tier.label}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="flex items-baseline gap-1.5">
        <span className="font-mono text-[56px] font-medium leading-14 text-[#E4E1EC]">
          ${selected.price}
        </span>
        <span className="text-lg leading-5.5 text-[#9794A3]">/month</span>
      </div>

      <div className="flex items-center gap-2 border border-[#8B5CF626] bg-[#8B5CF614] px-3.5 py-2.5">
        <span className="font-mono text-[13px] leading-4 text-chart-2">
          ${selected.perRequest}
        </span>
        <span className="text-[13px] leading-4 text-[#9794A3]">per request at this tier</span>
      </div>

      <div className="h-px w-full bg-border" />

      <div className="flex flex-col gap-3.5">
        {usageFeatures.map((feature) => (
          <div key={feature} className="flex items-center gap-2.5">
            <CheckIcon />
            <span className="text-sm leading-4.5 text-[#E4E1EC]">{feature}</span>
          </div>
        ))}
      </div>

      <Button render={<Link href="/get-started" />} className="h-auto w-full py-3.5 text-[15px] font-medium">
        Get started
      </Button>
    </div>
  );
}
