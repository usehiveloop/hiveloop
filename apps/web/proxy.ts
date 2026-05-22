import { NextRequest, NextResponse } from "next/server"

const SESSION_COOKIE = "__session"
const AUTH_ROUTES = new Set(["/auth/login", "/auth/signup"])

export function proxy(req: NextRequest) {
  const hasSession = req.cookies.has(SESSION_COOKIE)
  const { pathname } = req.nextUrl

  // Protected routes: require a session.
  if ((pathname === "/w" || pathname.startsWith("/w/")) && !hasSession) {
    return NextResponse.redirect(new URL("/auth/login", req.url))
  }

  if (AUTH_ROUTES.has(pathname) && hasSession) {
    return NextResponse.redirect(new URL("/w", req.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: ["/w/:path*", "/auth/login", "/auth/signup"],
}
