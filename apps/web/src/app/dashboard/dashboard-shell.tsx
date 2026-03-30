"use client";

import { useState, useEffect, useRef, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import { motion, AnimatePresence } from "motion/react";
import { switchOrganization } from "./actions";
import { $api } from "@/api/client";
type Organization = { id: string; name: string; role: string; description?: string | null };
import {
  LayoutDashboard,
  KeyRound,
  Key,
  Coins,
  Users,
  Blocks,
  Cable,
  Clock,
  Settings,
  Lock,
  ChevronsUpDown,
  Check,
  Plus,
  Menu,
  X,
  LogOut,
} from "lucide-react";

type NavItem = { label: string; icon: typeof LayoutDashboard; href: string };
type NavSection = { title?: string; items: NavItem[] };

const navSections: NavSection[] = [
  {
    items: [
      { label: "Dashboard", icon: LayoutDashboard, href: "/dashboard" },
    ],
  },
  {
    title: "Security",
    items: [
      { label: "Credentials", icon: KeyRound, href: "/dashboard/credentials" },
      { label: "Tokens", icon: Coins, href: "/dashboard/tokens" },
      { label: "API Keys", icon: Key, href: "/dashboard/api-keys" },
      { label: "Identities", icon: Users, href: "/dashboard/identities" },
    ],
  },
  {
    title: "Experience",
    items: [
      { label: "Connect UI", icon: Blocks, href: "/dashboard/connect" },
      { label: "Integrations", icon: Cable, href: "/dashboard/integrations" },
    ],
  },
  {
    title: "Manage",
    items: [
      { label: "Audit Log", icon: Clock, href: "/dashboard/audit-log" },
      { label: "Settings", icon: Settings, href: "/dashboard/settings" },
    ],
  },
];

const FREE_PLAN_LIMITS = {
  credentials: 15,
  requests: 10_000,
  identities: 500,
};

function formatCompact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function PlanUsage() {
  const { data: usage } = $api.useQuery("get", "/v1/usage");

  const items = [
    {
      label: "Credentials",
      current: usage?.credentials?.total ?? 0,
      max: FREE_PLAN_LIMITS.credentials,
      color: "#EAB308",
    },
    {
      label: "Proxy Requests",
      current: usage?.requests?.last_30d ?? 0,
      max: FREE_PLAN_LIMITS.requests,
      color: "#EAB308",
    },
    {
      label: "Identities",
      current: usage?.identities?.total ?? 0,
      max: FREE_PLAN_LIMITS.identities,
      color: "#8B5CF6",
    },
  ];

  return (
    <div className="flex flex-col gap-4 border border-border bg-card p-4">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-foreground">Free Plan</span>
        <span className="bg-primary/8 px-2 py-0.5 text-[11px] font-medium text-chart-2">
          Upgrade
        </span>
      </div>
      <div className="flex flex-col gap-4">
        {items.map((item) => {
          const percent = Math.min(Math.round((item.current / item.max) * 100), 100);
          return (
            <div key={item.label} className="flex flex-col gap-1">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-dim">{item.label}</span>
                <span className="font-mono text-[11px] text-muted-foreground">
                  {formatCompact(item.current)} / {formatCompact(item.max)}
                </span>
              </div>
              <div className="h-0.75 w-full bg-secondary">
                <div className="h-full" style={{ width: `${percent}%`, backgroundColor: item.color }} />
              </div>
            </div>
          );
        })}
      </div>
      <span className="text-2xs text-dim">Resets in 18 days</span>
    </div>
  );
}

function WorkspaceSwitcher({
  organizations,
  activeOrgId,
}: {
  organizations: Organization[];
  activeOrgId: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [isPending, startTransition] = useTransition();
  const router = useRouter();
  const active = organizations.find((organization) => organization.id === activeOrgId) ?? organizations[0];

  function handleSwitch(orgId: string) {
    setOpen(false);
    if (orgId === activeOrgId) return;
    startTransition(async () => {
      await switchOrganization(orgId);
      router.refresh();
    });
  }

  if (!active) {
    return (
      <div className="flex items-center gap-3 bg-secondary/50 px-3 py-2.5">
        <span className="text-[13px] text-muted-foreground">No organizations</span>
      </div>
    );
  }

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-3 bg-secondary/50 px-3 py-2.5 transition-colors hover:bg-secondary"
      >
        <div className="flex size-7 shrink-0 items-center justify-center bg-primary">
          <span className="text-xs font-semibold text-white">{active.name[0]?.toUpperCase()}</span>
        </div>
        <div className="flex min-w-0 flex-1 flex-col text-left">
          <span className="text-[13px] font-medium leading-4 text-foreground">{active.name}</span>
          {active.description && (
            <span className="text-[11px] leading-3.5 text-dim">{active.description}</span>
          )}
        </div>
        <ChevronsUpDown className="size-3.5 shrink-0 text-dim" />
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute left-0 top-full z-50 mt-1 flex w-full flex-col border border-border bg-card shadow-lg">
            <div className="px-3 py-2">
              <span className="text-[11px] font-semibold uppercase tracking-wider text-dim">
                Organizations
              </span>
            </div>
            {organizations.map((org) => (
              <button
                key={org.id}
                className="flex items-center gap-3 px-3 py-2 transition-colors hover:bg-secondary/50"
                onClick={() => handleSwitch(org.id)}
                disabled={isPending}
              >
                <div className="flex size-6 shrink-0 items-center justify-center bg-primary">
                  <span className="text-2xs font-semibold text-white">{org.name[0]?.toUpperCase()}</span>
                </div>
                <div className="flex min-w-0 flex-1 flex-col text-left">
                  <span className="text-xs font-medium leading-4 text-foreground">{org.name}</span>
                  {org.description && (
                    <span className="text-2xs leading-3.5 text-dim">{org.description}</span>
                  )}
                </div>
                {org.id === activeOrgId && <Check className="size-3.5 shrink-0 text-primary" />}
              </button>
            ))}
            <div className="border-t border-border">
              <button
                className="flex w-full items-center gap-3 px-3 py-2 transition-colors hover:bg-secondary/50"
                onClick={() => setOpen(false)}
              >
                <div className="flex size-6 shrink-0 items-center justify-center border border-dashed border-dim">
                  <Plus className="size-3 text-dim" />
                </div>
                <span className="text-xs text-muted-foreground">Create organization</span>
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function SignOutButton() {
  async function handleSignOut() {
    await fetch("/api/auth/logout", { method: "POST" });
    window.location.href = "/auth/login";
  }

  return (
    <button
      onClick={handleSignOut}
      className="flex w-full items-center gap-2.5 px-3 py-2 text-sm font-normal text-muted-foreground transition-colors hover:text-foreground"
    >
      <LogOut className="size-4.5" />
      Sign out
    </button>
  );
}

function SidebarContent({
  pathname,
  userName,
  organizations,
  activeOrgId,
}: {
  pathname: string;
  userName?: string | null;
  organizations: Organization[];
  activeOrgId: string | null;
}) {
  return (
    <>
      {/* Workspace Switcher */}
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-2">
          <div className="flex size-6 items-center justify-center border border-primary bg-primary/20">
            <Lock className="size-3.5 text-primary" />
          </div>
          <span className="text-[17px] font-semibold leading-5.5 tracking-tight text-foreground" style={{ fontFamily: "var(--font-bricolage)" }}>
            llmvault
          </span>
        </div>
        <WorkspaceSwitcher organizations={organizations} activeOrgId={activeOrgId} />
      </div>

      {/* Nav Items */}
      <nav className="flex flex-1 flex-col gap-6">
        {navSections.map((section, si) => (
          <div key={si} className="flex flex-col gap-1">
            {section.title && (
              <span className="mb-1 px-3 text-[11px] font-semibold uppercase tracking-wider text-dim">
                {section.title}
              </span>
            )}
            {section.items.map((item) => {
              const isActive = item.href === "/dashboard"
                ? pathname === "/dashboard"
                : pathname.startsWith(item.href);
              return (
                <Link
                  key={item.label}
                  href={item.href}
                  className={`relative flex items-center gap-2.5 px-3 py-2 text-sm ${
                    isActive
                      ? "font-medium text-foreground"
                      : "font-normal text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {isActive && (
                    <motion.div
                      layoutId="nav-active"
                      className="absolute inset-0 bg-secondary"
                      transition={{ type: "spring", stiffness: 350, damping: 30 }}
                    />
                  )}
                  <span className="relative flex items-center gap-2.5">
                    <item.icon className={`size-4.5 ${isActive ? "stroke-primary text-primary" : ""}`} />
                    {item.label}
                  </span>
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <PlanUsage />

      {/* User & Sign Out */}
      <div className="flex flex-col gap-1 border-t border-border pt-4">
        {userName && (
          <div className="px-3 pb-1">
            <span className="text-[11px] text-dim">{userName}</span>
          </div>
        )}
        <SignOutButton />
      </div>
    </>
  );
}

export function DashboardShell({
  children,
  userName,
  organizations,
  activeOrgId,
  syncOrgId,
}: {
  children: React.ReactNode;
  userName: string | null;
  organizations: Organization[];
  activeOrgId: string | null;
  syncOrgId?: string;
}) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const pathname = usePathname();
  const synced = useRef(false);

  useEffect(() => {
    if (syncOrgId && !synced.current) {
      synced.current = true;
      switchOrganization(syncOrgId);
    }
  }, [syncOrgId]);

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Desktop sidebar */}
      <aside className="hidden w-90 shrink-0 flex-col gap-10 border-r border-border bg-background px-6 py-8 lg:flex">
        <SidebarContent pathname={pathname} userName={userName} organizations={organizations} activeOrgId={activeOrgId} />
      </aside>

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
              className="relative flex h-full w-75 max-w-[80vw] flex-col gap-8 bg-background px-5 py-6"
            >
              <button
                onClick={() => setSidebarOpen(false)}
                className="absolute right-4 top-4 text-muted-foreground hover:text-foreground"
              >
                <X className="size-5" />
              </button>
              <SidebarContent pathname={pathname} userName={userName} organizations={organizations} activeOrgId={activeOrgId} />
            </motion.aside>
          </div>
        )}
      </AnimatePresence>

      {/* Main content */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Mobile top bar */}
        <div className="flex items-center gap-3 border-b border-border px-4 py-3 lg:hidden">
          <button onClick={() => setSidebarOpen(true)} className="text-muted-foreground hover:text-foreground">
            <Menu className="size-5" />
          </button>
          <div className="flex items-center gap-2">
            <div className="flex size-5 items-center justify-center border border-primary bg-primary/20">
              <Lock className="size-3 text-primary" />
            </div>
            <span className="text-[15px] font-semibold tracking-tight text-foreground" style={{ fontFamily: "var(--font-bricolage)" }}>
              llmvault
            </span>
          </div>
        </div>
        <main className="flex flex-1 flex-col overflow-y-auto">
          {children}
        </main>
      </div>
    </div>
  );
}
