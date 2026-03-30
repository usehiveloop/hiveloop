import { redirect } from "next/navigation";
import { getAccessToken } from "@/lib/auth";
import { apiMe } from "@/lib/auth-api";
import { getSelectedOrgId } from "@/lib/org";
import { DashboardShell } from "./dashboard-shell";
import { SetupWorkspace } from "./setup-workspace";

export const dynamic = "force-dynamic";

export default async function DashboardLayout({ children }: { children: React.ReactNode }) {
  const accessToken = await getAccessToken();

  if (!accessToken) {
    redirect("/auth/login");
  }

  let userName: string | null = null;
  let userEmail: string | null = null;
  let emailConfirmed = false;
  let orgs: { id: string; name: string; role: string }[] = [];

  try {
    const me = await apiMe(accessToken);
    userName = me.user.name ?? me.user.email ?? null;
    userEmail = me.user.email ?? null;
    emailConfirmed = me.user.email_confirmed ?? false;
    orgs = me.orgs ?? [];
  } catch {
    redirect("/auth/login");
  }

  if (!emailConfirmed) {
    redirect(`/auth/confirm-email?email=${encodeURIComponent(userEmail ?? "")}`);
  }

  if (orgs.length === 0) {
    return <SetupWorkspace />;
  }

  const selectedOrgId = await getSelectedOrgId();
  const orgIds = orgs.map((o) => o.id);

  // Default to first org if none selected or if selection is stale
  const activeOrgId = selectedOrgId && orgIds.includes(selectedOrgId)
    ? selectedOrgId
    : orgIds[0] ?? null;

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
