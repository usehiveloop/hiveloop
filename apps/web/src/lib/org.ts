import { cookies } from "next/headers";

const COOKIE_NAME = "llmvault_org_id";

/**
 * Get the currently selected organization ID from the cookie.
 */
export async function getSelectedOrgId(): Promise<string | undefined> {
  const cookieStore = await cookies();
  return cookieStore.get(COOKIE_NAME)?.value;
}

/**
 * Set the selected organization ID in a cookie.
 * Must be called from a server action or route handler.
 */
export async function setSelectedOrgId(orgId: string): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.set(COOKIE_NAME, orgId, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 365, // 1 year
  });
}
