import { NextRequest, NextResponse } from "next/server";
import { apiLogin } from "@/lib/auth-api";
import { setAuthCookies } from "@/lib/auth";

export async function POST(req: NextRequest) {
  try {
    const { email, password } = await req.json();

    if (!email || !password) {
      return NextResponse.json({ error: "Email and password are required" }, { status: 400 });
    }

    const data = await apiLogin(email, password);
    await setAuthCookies(data.access_token, data.refresh_token);

    return NextResponse.json({ user: data.user, orgs: data.orgs });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Login failed";
    return NextResponse.json({ error: message }, { status: 401 });
  }
}
