"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const tabs = [
  { label: "General", href: "/dashboard/settings" },
  { label: "Domains", href: "/dashboard/settings/domains" },
  { label: "Team", href: "/dashboard/settings/team" },
  { label: "Billing", href: "/dashboard/settings/billing" },
];

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <>
      <header className="flex shrink-0 flex-col gap-4 border-b border-border px-4 py-5 sm:px-6 lg:px-8">
        <h1 className="font-mono text-lg font-medium tracking-tight text-foreground sm:text-xl">
          Settings
        </h1>
        <nav className="flex items-center gap-1">
          {tabs.map((tab) => {
            const isActive =
              tab.href === "/dashboard/settings"
                ? pathname === "/dashboard/settings"
                : pathname.startsWith(tab.href);
            return (
              <Link
                key={tab.href}
                href={tab.href}
                className={`px-3 py-1.5 text-[13px] font-medium transition-colors ${
                  isActive
                    ? "bg-primary/8 text-chart-2"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {tab.label}
              </Link>
            );
          })}
        </nav>
      </header>
      {children}
    </>
  );
}
