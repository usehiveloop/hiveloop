import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { SettingsShell } from "@/components/settings-shell"
import { HugeiconsIcon } from "@hugeicons/react"
import { CreditCardIcon, Download01Icon } from "@hugeicons/core-free-icons"

const INVOICES = [
  { id: "INV-2026-04", date: "Apr 1, 2026", amount: "$39.00", status: "Paid" },
  { id: "INV-2026-03", date: "Mar 1, 2026", amount: "$39.00", status: "Paid" },
  { id: "INV-2026-02", date: "Feb 1, 2026", amount: "$39.00", status: "Paid" },
  { id: "INV-2026-01", date: "Jan 1, 2026", amount: "$39.00", status: "Paid" },
]

const USED = 18_569
const TOTAL = 39_000
const PCT = USED / TOTAL

export default function Page() {
  return (
    <SettingsShell
      title="Billing"
      description="Plan, payment method, and invoices."
    >
      {/* Plan — flat dense row, deliberately not a hero card */}
      <section>
        <div className="flex items-baseline justify-between gap-4">
          <div className="flex items-baseline gap-2.5">
            <h2 className="text-[15px] font-medium">Pro</h2>
            <span className="text-[12px] text-muted-foreground">
              $39 / month, billed monthly
            </span>
          </div>
          <Button variant="outline" size="sm">
            Change plan
          </Button>
        </div>
        <p className="mt-1.5 text-[12px] text-muted-foreground">
          Renews{" "}
          <span className="text-foreground">May 1, 2026</span>. Includes 39,000
          monthly credits and unlimited workspaces.
        </p>
      </section>

      {/* Usage */}
      <section>
        <div className="flex items-baseline justify-between">
          <Label className="text-[13px] font-medium">Credits this period</Label>
          <span className="font-mono text-[12px] tabular-nums text-muted-foreground">
            <span className="text-foreground">{USED.toLocaleString("en-US")}</span>
            <span className="px-1 text-muted-foreground/50">/</span>
            {TOTAL.toLocaleString("en-US")}
          </span>
        </div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-primary"
            style={{ width: `${PCT * 100}%` }}
          />
        </div>
        <div className="mt-2 flex items-baseline justify-between text-[12px] text-muted-foreground">
          <span>Resets May 1. Top-up bundles never expire.</span>
          <a href="#" className="hover:text-foreground">View usage</a>
        </div>
      </section>

      {/* Payment method */}
      <section className="flex flex-col gap-2.5">
        <div>
          <Label className="text-[13px] font-medium">Payment method</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Charged on the first of each month.
          </p>
        </div>
        <div className="flex items-center justify-between rounded-lg border border-border/60 px-3.5 py-2.5">
          <div className="flex items-center gap-3">
            <div className="flex size-9 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <HugeiconsIcon icon={CreditCardIcon} strokeWidth={2} className="size-4" />
            </div>
            <div>
              <p className="text-[13px] font-medium">Visa ending in 4242</p>
              <p className="text-[12px] text-muted-foreground">Expires 09 / 2027</p>
            </div>
          </div>
          <Button variant="ghost" size="sm" className="h-8">
            Update
          </Button>
        </div>
      </section>

      {/* Billing email */}
      <section className="flex flex-col gap-2.5">
        <div>
          <Label className="text-[13px] font-medium">Billing email</Label>
          <p className="mt-0.5 text-[12px] text-muted-foreground">
            Receipts and invoices are sent here.
          </p>
        </div>
        <Input defaultValue="billing@acme.co" className="max-w-sm" />
      </section>

      {/* Invoices */}
      <section>
        <h2 className="mb-3 text-[13px] font-medium">Invoices</h2>
        <ul className="divide-y divide-border/60 rounded-lg border border-border/60">
          {INVOICES.map((inv) => (
            <li
              key={inv.id}
              className="grid grid-cols-[1fr_auto_auto_auto] items-center gap-4 px-3.5 py-2.5"
            >
              <div className="min-w-0">
                <p className="truncate text-[13px]">{inv.date}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">
                  {inv.id}
                </p>
              </div>
              <span className="font-mono text-[13px] tabular-nums">{inv.amount}</span>
              <span className="text-[12px] text-muted-foreground">{inv.status}</span>
              <button
                type="button"
                className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                aria-label={`Download invoice ${inv.id}`}
              >
                <HugeiconsIcon icon={Download01Icon} strokeWidth={2} className="size-4" />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </SettingsShell>
  )
}
