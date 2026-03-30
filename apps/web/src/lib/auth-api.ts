const API_URL = process.env.NEXT_PUBLIC_API_URL!;

export type AuthUser = { id: string; email: string; name: string; email_confirmed: boolean };
export type AuthOrg = { id: string; name: string; role: string };
export type AuthResponse = {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: AuthUser;
  orgs: AuthOrg[];
};
export type MeResponse = { user: AuthUser; orgs: AuthOrg[] };

export async function apiRegister(email: string, password: string, name: string): Promise<AuthResponse> {
  const res = await fetch(`${API_URL}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, name }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `Registration failed: ${res.status}`);
  }
  return res.json();
}

export async function apiLogin(email: string, password: string, orgId?: string): Promise<AuthResponse> {
  const res = await fetch(`${API_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password, org_id: orgId }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `Login failed: ${res.status}`);
  }
  return res.json();
}

export async function apiRefresh(refreshToken: string, orgId?: string): Promise<AuthResponse> {
  const res = await fetch(`${API_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken, org_id: orgId }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `Refresh failed: ${res.status}`);
  }
  return res.json();
}

export async function apiLogout(accessToken: string, refreshToken: string): Promise<void> {
  await fetch(`${API_URL}/auth/logout`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Authorization": `Bearer ${accessToken}` },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
}

export async function apiMe(accessToken: string): Promise<MeResponse> {
  const res = await fetch(`${API_URL}/auth/me`, {
    headers: { "Authorization": `Bearer ${accessToken}` },
  });
  if (!res.ok) {
    throw new Error(`Failed to fetch user info: ${res.status}`);
  }
  return res.json();
}
