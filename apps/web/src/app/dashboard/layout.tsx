import { getLogtoContext } from "@logto/next/server-actions";
import { getLogtoConfig } from "@/lib/logto";
import { getOrganizations } from "@/lib/logto-api";
import { getSelectedOrgId } from "@/lib/org";
import { DashboardShell } from "./dashboard-shell";
import { SetupWorkspace } from "./setup-workspace";

export const dynamic = "force-dynamic";

export default async function DashboardLayout({ children }: { children: React.ReactNode }) {
  const config = getLogtoConfig();
  const { claims } = await getLogtoContext(config);
  const userName = claims?.name ?? claims?.email ?? claims?.username ?? null;
  const orgIds = claims?.organizations ?? [];

  if (orgIds.length === 0) {
    return <SetupWorkspace />;
  }

  const orgs = await getOrganizations(orgIds);
  const selectedOrgId = await getSelectedOrgId();

  // Default to first org if none selected or if selection is stale
  const activeOrgId = selectedOrgId && orgIds.includes(selectedOrgId)
    ? selectedOrgId
    : orgIds[0] ?? null;

  // If we need to persist a default, let the client component handle it
  // via a server action (cookies can't be set in Server Components)
  const needsOrgSync = !!(activeOrgId && activeOrgId !== selectedOrgId);

  return (
    <DashboardShell
      userName={userName}
      organizations={orgs}
      activeOrgId={activeOrgId}
      syncOrgId={needsOrgSync ? activeOrgId : undefined}
    >
      {children}
    </DashboardShell>
  );
}
