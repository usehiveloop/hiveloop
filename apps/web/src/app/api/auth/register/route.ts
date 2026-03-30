import { NextRequest, NextResponse } from "next/server";
import { apiRegister } from "@/lib/auth-api";
import { setAuthCookies } from "@/lib/auth";

export async function POST(req: NextRequest) {
  try {
    const { email, password, name } = await req.json();

    if (!email || !password || !name) {
      return NextResponse.json({ error: "Email, password, and name are required" }, { status: 400 });
    }

    const data = await apiRegister(email, password, name);
    await setAuthCookies(data.access_token, data.refresh_token);

    return NextResponse.json({ user: data.user, orgs: data.orgs });
  } catch (err) {
    const message = err instanceof Error ? err.message : "Registration failed";
    return NextResponse.json({ error: message }, { status: 400 });
  }
}
