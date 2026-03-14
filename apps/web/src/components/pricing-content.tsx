"use client";

import { useState } from "react";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { CheckIcon } from "@/components/icons";
import { UsageTierCard, pricingTiers } from "@/components/usage-tier-card";

const volumeTiers = [
  ...pricingTiers.map((t) => ({ label: t.shortLabel, price: `$${t.price}`, per: t.per1K })),
  { label: "25M+", price: "Custom", per: "Let\u2019s talk" },
];

const freeFeatures = [
  "10,000 proxy requests",
  "10 credentials",
  "All providers",
  "7-day audit log",
  "Community support",
];

const enterpriseFeatures = [
  "Everything in Usage",
  "Self-hosted deployment",
  "SSO / SAML",
  "Volume discounts",
  "Dedicated support + SLA",
];

export function PricingContent() {
  const [selectedIndex, setSelectedIndex] = useState(0);

  return (
    <>
      <section className="flex items-start gap-5 px-20 pb-25">
        {/* Free Tier */}
        <div className="flex w-80 shrink-0 flex-col gap-7 border border-border bg-surface p-9">
          <div className="flex flex-col gap-2.5">
            <span className="font-mono text-[13px] font-medium leading-4 tracking-wider uppercase text-[#9794A3]">
              Free
            </span>
            <div className="flex items-baseline">
              <span className="font-mono text-[40px] font-medium leading-10 text-[#E4E1EC]">$0</span>
            </div>
            <span className="text-sm leading-[21px] text-[#9794A3]">
              10,000 requests/mo. No credit card.
            </span>
          </div>

          <div className="h-px w-full bg-border" />

          <div className="flex flex-col gap-3.5">
            {freeFeatures.map((feature) => (
              <div key={feature} className="flex items-center gap-2.5">
                <CheckIcon />
                <span className="text-sm leading-4.5 text-[#E4E1EC]">{feature}</span>
              </div>
            ))}
          </div>

          <Button render={<Link href="/get-started" />} variant="outline" className="h-auto w-full py-3 text-sm font-medium text-[#E4E1EC]">
            Start building
          </Button>
        </div>

        {/* Usage Tier */}
        <UsageTierCard selectedIndex={selectedIndex} onSelectTier={setSelectedIndex} />

        {/* Enterprise Tier */}
        <div className="flex w-80 shrink-0 flex-col gap-7 border border-border bg-surface p-9">
          <div className="flex flex-col gap-2.5">
            <span className="font-mono text-[13px] font-medium leading-4 tracking-wider uppercase text-[#9794A3]">
              Enterprise
            </span>
            <div className="flex items-baseline">
              <span className="font-mono text-[40px] font-medium leading-10 text-[#E4E1EC]">Custom</span>
            </div>
            <span className="text-sm leading-[21px] text-[#9794A3]">
              Volume pricing + self-hosted.
            </span>
          </div>

          <div className="h-px w-full bg-border" />

          <div className="flex flex-col gap-3.5">
            {enterpriseFeatures.map((feature) => (
              <div key={feature} className="flex items-center gap-2.5">
                <CheckIcon />
                <span className="text-sm leading-4.5 text-[#E4E1EC]">{feature}</span>
              </div>
            ))}
          </div>

          <Button render={<Link href="/contact" />} variant="outline" className="h-auto w-full py-3 text-sm font-medium text-[#E4E1EC]">
            Talk to an engineer
          </Button>
        </div>
      </section>

      <section className="flex flex-col items-center gap-6 px-20 pb-25">
        <span className="font-mono text-sm font-medium leading-4.5 text-[#9794A3]">
          Volume pricing — the more you proxy, the less you pay per request
        </span>
        <div className="flex w-full">
          {volumeTiers.map((tier, i) => {
            const isHighlighted = i === selectedIndex;
            const isCustom = tier.price === "Custom";
            return (
              <button
                key={tier.label}
                onClick={() => {
                  if (i < pricingTiers.length) setSelectedIndex(i);
                }}
                className={`flex flex-1 flex-col items-center gap-1.5 px-4 py-5 transition-colors ${
                  isHighlighted
                    ? "bg-primary"
                    : "border border-border bg-surface"
                } ${i < pricingTiers.length ? "cursor-pointer" : "cursor-default"}`}
              >
                <span
                  className={`font-mono text-[15px] font-semibold leading-4.5 ${
                    isHighlighted ? "text-white" : "text-[#E4E1EC]"
                  }`}
                >
                  {tier.label}
                </span>
                <span
                  className={`font-mono text-xl font-medium leading-6 ${
                    isHighlighted ? "text-white" : isCustom ? "text-[#9794A3]" : "text-[#E4E1EC]"
                  }`}
                >
                  {tier.price}
                </span>
                <span
                  className={`text-xs leading-4 ${
                    isHighlighted ? "text-white/70" : "text-[#9794A3]"
                  }`}
                >
                  {tier.per}
                </span>
              </button>
            );
          })}
        </div>
      </section>
    </>
  );
}
