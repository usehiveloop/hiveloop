import { type NextRequest, NextResponse } from "next/server";

const API_URL = process.env.NEXT_PUBLIC_API_URL!;

export async function middleware(request: NextRequest) {
  let accessToken = request.cookies.get("llmvault_access_token")?.value;
  let response = NextResponse.next();

  // If no access token, try refreshing
  if (!accessToken) {
    const refreshToken = request.cookies.get("llmvault_refresh_token")?.value;
    if (!refreshToken) {
      return NextResponse.redirect(new URL("/auth/login", request.url));
    }

    try {
      const orgId = request.cookies.get("llmvault_org_id")?.value;
      const res = await fetch(`${API_URL}/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken, org_id: orgId }),
      });

      if (!res.ok) {
        return NextResponse.redirect(new URL("/auth/login", request.url));
      }

      const data = await res.json();
      const isProduction = process.env.NODE_ENV === "production";
      accessToken = data.access_token;

      response.cookies.set("llmvault_access_token", data.access_token, {
        httpOnly: true,
        secure: isProduction,
        sameSite: "lax",
        path: "/",
        maxAge: 60 * 15,
      });

      response.cookies.set("llmvault_refresh_token", data.refresh_token, {
        httpOnly: true,
        secure: isProduction,
        sameSite: "lax",
        path: "/",
        maxAge: 60 * 60 * 24 * 30,
      });
    } catch {
      return NextResponse.redirect(new URL("/auth/login", request.url));
    }
  }

  // Check email confirmation via /auth/me
  try {
    const meRes = await fetch(`${API_URL}/auth/me`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });

    if (meRes.ok) {
      const me = await meRes.json();
      const isConfirmPage = request.nextUrl.pathname === "/auth/confirm-email";

      if (!me.user.email_confirmed && !isConfirmPage) {
        return NextResponse.redirect(
          new URL(`/auth/confirm-email?email=${encodeURIComponent(me.user.email)}`, request.url),
        );
      }

      if (me.user.email_confirmed && isConfirmPage) {
        return NextResponse.redirect(new URL("/dashboard", request.url));
      }
    }
  } catch {
    // If /auth/me fails, let the request through — dashboard layout will handle it
  }

  return response;
}

export const config = {
  matcher: ["/dashboard/:path*", "/auth/confirm-email"],
};
