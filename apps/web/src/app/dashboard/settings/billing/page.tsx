"use client";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

const plans = [
  {
    id: "free",
    name: "Free",
    price: "$0",
    period: "/month",
    current: true,
    features: ["15 credentials", "10k proxy requests/mo", "500 identities", "7-day audit log"],
  },
  {
    id: "pro",
    name: "Pro",
    price: "$49",
    period: "/month",
    current: false,
    features: ["100 credentials", "100k proxy requests/mo", "5,000 identities", "90-day audit log", "Priority support"],
  },
  {
    id: "enterprise",
    name: "Enterprise",
    price: "Custom",
    period: "",
    current: false,
    features: ["Unlimited credentials", "Unlimited proxy requests", "Unlimited identities", "1-year audit log", "SSO & SAML", "Dedicated support"],
  },
];

const invoices = [
  { id: "inv_001", date: "Mar 1, 2026", amount: "$0.00", status: "Paid" },
  { id: "inv_002", date: "Feb 1, 2026", amount: "$0.00", status: "Paid" },
  { id: "inv_003", date: "Jan 1, 2026", amount: "$0.00", status: "Paid" },
];

export default function SettingsBillingPage() {
  return (
    <div className="flex flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
      {/* Current Plan */}
      <div className="flex flex-col gap-4 border border-border bg-card p-5">
        <div className="flex flex-col gap-1">
          <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
            Current Plan
          </span>
          <div className="flex items-center gap-2">
            <span className="text-lg font-medium text-foreground">Free</span>
            <Badge variant="outline" className="h-auto border-success/20 bg-success/8 text-[11px] text-success-foreground">
              Active
            </Badge>
          </div>
          <p className="text-[13px] text-muted-foreground">
            Your plan renews on April 1, 2026.
          </p>
        </div>
      </div>

      {/* Plans */}
      <div className="grid gap-4 sm:grid-cols-3">
        {plans.map((plan) => (
          <div
            key={plan.id}
            className={`flex flex-col gap-4 border p-5 ${
              plan.current
                ? "border-primary bg-card"
                : "border-border bg-card"
            }`}
          >
            <div className="flex flex-col gap-1">
              <span className="text-[13px] font-semibold text-foreground">{plan.name}</span>
              <div className="flex items-baseline gap-0.5">
                <span className="font-mono text-2xl font-medium text-foreground">{plan.price}</span>
                {plan.period && (
                  <span className="text-[13px] text-muted-foreground">{plan.period}</span>
                )}
              </div>
            </div>
            <ul className="flex flex-col gap-2">
              {plan.features.map((f) => (
                <li key={f} className="text-[13px] text-muted-foreground">
                  {f}
                </li>
              ))}
            </ul>
            <div className="mt-auto">
              {plan.current ? (
                <Button variant="outline" size="lg" className="w-full" disabled>
                  Current Plan
                </Button>
              ) : (
                <Button size="lg" className="w-full">
                  {plan.id === "enterprise" ? "Contact Sales" : "Upgrade"}
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>

      {/* Invoices */}
      <div className="flex flex-col gap-4 border border-border bg-card p-5">
        <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
          Invoices
        </span>
        <div className="flex flex-col">
          {invoices.map((inv) => (
            <div
              key={inv.id}
              className="flex items-center justify-between border-b border-border py-3 last:border-b-0"
            >
              <div className="flex items-center gap-4">
                <span className="font-mono text-[13px] text-foreground">{inv.id}</span>
                <span className="text-[13px] text-muted-foreground">{inv.date}</span>
              </div>
              <div className="flex items-center gap-4">
                <span className="font-mono text-[13px] text-foreground">{inv.amount}</span>
                <Badge variant="outline" className="h-auto border-success/20 bg-success/8 text-[11px] text-success-foreground">
                  {inv.status}
                </Badge>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
