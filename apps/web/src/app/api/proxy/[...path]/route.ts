import { NextRequest, NextResponse } from "next/server";
import { getAccessToken, getRefreshToken, setAuthCookies } from "@/lib/auth";
import { apiRefresh } from "@/lib/auth-api";
import { getSelectedOrgId } from "@/lib/org";

const API_URL = process.env.NEXT_PUBLIC_API_URL!;

async function getValidToken(): Promise<string | undefined> {
  let token = await getAccessToken();
  if (token) return token;

  // Access token missing — try refreshing
  const refreshToken = await getRefreshToken();
  if (!refreshToken) return undefined;

  try {
    const orgId = await getSelectedOrgId();
    const data = await apiRefresh(refreshToken, orgId);
    await setAuthCookies(data.access_token, data.refresh_token);
    return data.access_token;
  } catch {
    return undefined;
  }
}

async function proxy(req: NextRequest) {
  const path = req.nextUrl.pathname.replace(/^\/api\/proxy/, "");
  const url = `${API_URL}${path}${req.nextUrl.search}`;

  const headers = new Headers();
  headers.set("Content-Type", "application/json");

  const token = await getValidToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  console.log("[proxy]", {
    method: req.method,
    path,
    url,
    hasToken: !!token,
  });

  const res = await fetch(url, {
    method: req.method,
    headers,
    body: req.method !== "GET" && req.method !== "HEAD" ? await req.text() : undefined,
  });

  const resBody = await res.text();

  console.log("[proxy] response", {
    status: res.status,
    path,
    body: resBody.substring(0, 200),
  });

  return new NextResponse(resBody, {
    status: res.status,
    statusText: res.statusText,
    headers: { "Content-Type": res.headers.get("Content-Type") ?? "application/json" },
  });
}

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
