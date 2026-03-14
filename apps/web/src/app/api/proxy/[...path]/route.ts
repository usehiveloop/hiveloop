import { NextRequest, NextResponse } from "next/server";
import { getAccessToken } from "@logto/next/server-actions";
import { getLogtoConfig } from "@/lib/logto";
import { getSelectedOrgId } from "@/lib/org";

const API_URL = process.env.NEXT_PUBLIC_API_URL!;

async function proxy(req: NextRequest) {
  const path = req.nextUrl.pathname.replace(/^\/api\/proxy/, "");
  const url = `${API_URL}${path}${req.nextUrl.search}`;

  const headers = new Headers();
  headers.set("Content-Type", "application/json");

  let token: string | undefined;
  let orgId: string | undefined;
  let resource: string | undefined;
  let authError: string | undefined;

  try {
    const config = getLogtoConfig();
    resource = config.resources?.[0];
    orgId = await getSelectedOrgId();
    token = await getAccessToken(config, resource, orgId);
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }
  } catch (e) {
    authError = e instanceof Error ? e.message : String(e);
  }

  console.log("[proxy]", {
    method: req.method,
    path,
    url,
    token,
    resource,
    orgId: orgId ?? "(none)",
    hasToken: !!token,
    tokenPrefix: token ? token.substring(0, 30) + "..." : "(none)",
    authError: authError ?? "(none)",
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
