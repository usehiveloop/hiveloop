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
      { label: "Quickstart", href: "/docs/quickstart" },
      { label: "Installation", href: "/docs/installation" },
      { label: "Authentication", href: "/docs/authentication" },
    ],
  },
  {
    title: "Core Concepts",
    items: [
      { label: "Credentials", href: "/docs/credentials" },
      { label: "Tokens", href: "/docs/tokens" },
      { label: "Proxy", href: "/docs/proxy" },
      { label: "Providers", href: "/docs/providers" },
    ],
  },
  {
    title: "API Reference",
    items: [
      { label: "Credentials API", href: "/docs/api/credentials" },
      { label: "Tokens API", href: "/docs/api/tokens" },
      { label: "Proxy API", href: "/docs/api/proxy" },
    ],
  },
  {
    title: "Infrastructure",
    items: [
      { label: "Architecture", href: "/docs/architecture" },
      { label: "Security", href: "/docs/security" },
      { label: "Self-Hosting", href: "/docs/self-hosting" },
    ],
  },
  {
    title: "SDKs",
    items: [
      { label: "TypeScript", href: "/docs/sdk/typescript" },
      { label: "Python", href: "/docs/sdk/python" },
      { label: "Go", href: "/docs/sdk/go" },
    ],
  },
];

function DocsSidebarContent({ pathname }: { pathname: string }) {
  return (
    <nav className="flex flex-1 flex-col gap-0 overflow-y-auto">
      {docsSections.map((section) => (
        <div key={section.title} className="flex flex-col gap-1 px-4 py-2">
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

      {/* Mobile top bar (below nav, replaces sidebar on small screens) */}
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
      <div className="mx-auto flex min-w-0 flex-1 max-w-7xl overflow-hidden pt-13 lg:pt-0">
        {/* Left sidebar — own scroll */}
        <aside className="hidden w-75 shrink-0 flex-col overflow-y-auto border-r border-border bg-[#18171E] pl-14 pt-6 lg:flex">
          <DocsSidebarContent pathname={pathname} />
        </aside>

        {/* Main content — own scroll */}
        <main className="min-w-0 flex-1 overflow-y-auto px-8 py-10 lg:max-w-185 lg:px-15">
          {children}
        </main>

        {/* Right sidebar — fixed, no scroll */}
        <aside className="hidden w-70 shrink-0 border-l border-border px-6 pr-20 py-10 xl:block">
          <div id="docs-toc" />
        </aside>
      </div>

      {/* Footer — fixed at bottom */}
      <Footer />
    </div>
  );
}
