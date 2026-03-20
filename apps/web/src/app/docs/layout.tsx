"use client";

import { useState } from "react";
import { usePathname } from "next/navigation";
import Link from "next/link";
import { motion, AnimatePresence } from "motion/react";
import { Menu, X } from "lucide-react";
import { Nav } from "@/components/nav";
import { Footer } from "@/components/footer";

type DocNavItem = { label: string; href: string };
type DocNavSection = { title: string; items: DocNavItem[] };

const docsSections: DocNavSection[] = [
  {
    title: "Getting Started",
    items: [
      { label: "Introduction", href: "/docs/getting-started/introduction" },
      { label: "Quickstart", href: "/docs/getting-started/quickstart" },
      { label: "Installation", href: "/docs/getting-started/installation" },
      { label: "Authentication", href: "/docs/getting-started/authentication" },
    ],
  },
  {
    title: "Core Concepts",
    items: [
      { label: "Credentials", href: "/docs/core-concepts/credentials" },
      { label: "Tokens", href: "/docs/core-concepts/tokens" },
      { label: "Proxy", href: "/docs/core-concepts/proxy" },
      { label: "Providers", href: "/docs/core-concepts/providers" },
      { label: "Identities", href: "/docs/core-concepts/identities" },
      { label: "Rate Limiting", href: "/docs/core-concepts/rate-limiting" },
    ],
  },
  {
    title: "Connect",
    items: [
      { label: "Overview", href: "/docs/connect/overview" },
      { label: "Embedding", href: "/docs/connect/embedding" },
      { label: "Theming", href: "/docs/connect/theming" },
      { label: "Sessions", href: "/docs/connect/sessions" },
      { label: "Provider Connections", href: "/docs/connect/providers" },
      { label: "Integration Connections", href: "/docs/connect/integrations" },
      { label: "Frontend SDK", href: "/docs/connect/frontend-sdk" },
    ],
  },
  {
    title: "Dashboard",
    items: [
      { label: "Overview", href: "/docs/dashboard/overview" },
      { label: "Credentials", href: "/docs/dashboard/credentials" },
      { label: "Tokens", href: "/docs/dashboard/tokens" },
      { label: "API Keys", href: "/docs/dashboard/api-keys" },
      { label: "Identities", href: "/docs/dashboard/identities" },
      { label: "Integrations", href: "/docs/dashboard/integrations" },
      { label: "Audit Log", href: "/docs/dashboard/audit-log" },
      { label: "Team Management", href: "/docs/dashboard/team" },
      { label: "Billing", href: "/docs/dashboard/billing" },
    ],
  },
  {
    title: "SDKs",
    items: [
      { label: "TypeScript SDK", href: "/docs/sdks/typescript" },
      { label: "Python SDK", href: "/docs/sdks/python" },
      { label: "Go SDK", href: "/docs/sdks/go" },
      { label: "Frontend SDK", href: "/docs/sdks/frontend" },
    ],
  },
  {
    title: "Security",
    items: [
      { label: "Overview", href: "/docs/security/overview" },
      { label: "Encryption", href: "/docs/security/encryption" },
      { label: "Token Scoping", href: "/docs/security/token-scoping" },
      { label: "Audit Logging", href: "/docs/security/audit-logging" },
      { label: "Compliance", href: "/docs/security/compliance" },
    ],
  },
  {
    title: "Self-Hosting",
    items: [
      { label: "Overview", href: "/docs/self-hosting/overview" },
      { label: "Docker Compose", href: "/docs/self-hosting/docker-compose" },
      { label: "Kubernetes", href: "/docs/self-hosting/kubernetes" },
      { label: "Configuration", href: "/docs/self-hosting/configuration" },
      { label: "Environment Variables", href: "/docs/self-hosting/environment" },
    ],
  },
];

function DocsSidebarContent({ pathname }: { pathname: string }) {
  return (
    <nav className="flex flex-1 flex-col gap-0 overflow-y-auto">
      {docsSections.map((section) => (
        <div key={section.title} className="flex flex-col gap-1 px-4 py-6">
          <span className="px-2 pb-2 text-[11px] font-semibold uppercase leading-3.5 tracking-wider text-[#9794A3]">
            {section.title}
          </span>
          {section.items.map((item) => {
            const isActive = pathname === item.href;
            return (
              <Link
                key={item.href}
                href={item.href}
                className={`flex items-center px-2 py-2 text-sm leading-4.5 ${
                  isActive
                    ? "border-l-2 border-primary bg-[#8B5CF61A] font-medium text-[#A78BFA]"
                    : "border-l-2 border-transparent font-normal text-[#9794A3] hover:text-[#E4E1EC]"
                }`}
              >
                {item.label}
              </Link>
            );
          })}
        </div>
      ))}
    </nav>
  );
}

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-background">
      {/* Global nav */}
      <Nav />

      {/* Mobile sidebar overlay */}
      <AnimatePresence>
        {sidebarOpen && (
          <div className="fixed inset-0 z-50 lg:hidden">
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="absolute inset-0 bg-black/60"
              onClick={() => setSidebarOpen(false)}
            />
            <motion.aside
              initial={{ x: -300 }}
              animate={{ x: 0 }}
              exit={{ x: -300 }}
              transition={{ type: "spring", stiffness: 350, damping: 35 }}
              className="relative flex h-full w-75 flex-col bg-[#18171E] pt-6"
            >
              <button
                onClick={() => setSidebarOpen(false)}
                className="absolute right-4 top-4 text-[#9794A3] hover:text-[#E4E1EC]"
              >
                <X className="size-5" />
              </button>
              <DocsSidebarContent pathname={pathname} />
            </motion.aside>
          </div>
        )}
      </AnimatePresence>

      <div className="fixed left-0 right-0 top-16 z-40 flex items-center gap-3 border-b border-border bg-background px-4 py-3 lg:hidden">
        <button
          onClick={() => setSidebarOpen(true)}
          className="text-[#9794A3] hover:text-[#E4E1EC]"
        >
          <Menu className="size-5" />
        </button>
        <span className="text-sm text-[#9794A3]">Documentation</span>
      </div>

      {/* 3-column layout — fills between nav and footer, no page scroll */}
      <div className="mx-auto flex min-w-0 flex-1 overflow-hidden w-full pt-13 lg:pt-0">
        {/* Left sidebar — own scroll */}
        <aside className="hidden w-75 shrink-0 flex-col overflow-y-auto border-r border-border bg-[#18171E] pt-6 lg:flex">
          <DocsSidebarContent pathname={pathname} />
        </aside>

        {/* Main content — own scroll */}
        <main className="min-w-0 flex-1 overflow-y-auto px-6 py-10 sm:px-8 lg:px-12">
          <div className="mx-auto max-w-3xl">
            {children}
          </div>
        </main>

        {/* Right sidebar — ToC */}
        <aside className="hidden w-56 shrink-0 overflow-y-auto py-10 pr-6 xl:block">
          <div id="docs-toc" className="sticky top-10" />
        </aside>
      </div>

      {/* Footer — fixed at bottom */}
      <Footer />
    </div>
  );
}
