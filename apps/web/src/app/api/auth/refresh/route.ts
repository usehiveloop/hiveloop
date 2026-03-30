import { NextRequest, NextResponse } from "next/server";
import { getRefreshToken, setAuthCookies } from "@/lib/auth";
import { apiRefresh } from "@/lib/auth-api";

export async function POST(req: NextRequest) {
  try {
    const refreshToken = await getRefreshToken();

    if (!refreshToken) {
      return NextResponse.json({ error: "No refresh token" }, { status: 401 });
    }

    const body = await req.json().catch(() => ({}));
    const orgId = body.org_id as string | undefined;

    const data = await apiRefresh(refreshToken, orgId);
    await setAuthCookies(data.access_token, data.refresh_token);

    return NextResponse.json({ user: data.user, orgs: data.orgs });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Refresh failed";
    return NextResponse.json({ error: message }, { status: 401 });
  }
}
