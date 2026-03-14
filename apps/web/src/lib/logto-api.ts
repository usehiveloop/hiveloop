/**
 * Server-side Logto Management API client.
 * Uses M2M client credentials to fetch organization details.
 */

const LOGTO_ENDPOINT = process.env.LOGTO_ENDPOINT!;
const LOGTO_M2M_APP_ID = process.env.LOGTO_M2M_APP_ID!;
const LOGTO_M2M_APP_SECRET = process.env.LOGTO_M2M_APP_SECRET!;

// Logto self-hosted Management API resource indicator
const MANAGEMENT_API_RESOURCE = "https://default.logto.app/api";

export type LogtoOrganization = {
  id: string;
  name: string;
  description?: string | null;
};

let cachedToken: { token: string; expiresAt: number } | null = null;

async function getManagementToken(): Promise<string> {
  if (cachedToken && Date.now() < cachedToken.expiresAt - 30_000) {
    return cachedToken.token;
  }

  const res = await fetch(`${LOGTO_ENDPOINT}/oidc/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "client_credentials",
      client_id: LOGTO_M2M_APP_ID,
      client_secret: LOGTO_M2M_APP_SECRET,
      resource: MANAGEMENT_API_RESOURCE,
      scope: "all",
    }),
  });

  if (!res.ok) {
    throw new Error(`Failed to get management token: ${res.status}`);
  }

  const data = await res.json();
  cachedToken = {
    token: data.access_token,
    expiresAt: Date.now() + data.expires_in * 1000,
  };
  return data.access_token;
}

/**
 * Fetch organization details by IDs from the Logto Management API.
 */
export async function getOrganizations(
  orgIds: string[],
): Promise<LogtoOrganization[]> {
  if (orgIds.length === 0) return [];

  const token = await getManagementToken();

  // Fetch all orgs and filter to the ones the user belongs to.
  // Logto Management API: GET /api/organizations
  const res = await fetch(`${LOGTO_ENDPOINT}/api/organizations`, {
    headers: { Authorization: `Bearer ${token}` },
    next: { revalidate: 60 },
  });

  if (!res.ok) {
    throw new Error(`Failed to fetch organizations: ${res.status}`);
  }

  const allOrgs: LogtoOrganization[] = await res.json();
  const idSet = new Set(orgIds);
  return allOrgs.filter((org) => idSet.has(org.id));
}
