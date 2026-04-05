import { NextRequest, NextResponse } from "next/server"

const SESSION_COOKIE = "__zeus_session"

export function middleware(req: NextRequest) {
  const hasSession = req.cookies.has(SESSION_COOKIE)
  const { pathname } = req.nextUrl

  // Protected routes require session
  if (pathname.startsWith("/dashboard") && !hasSession) {
    return NextResponse.redirect(new URL("/auth", req.url))
  }

  // Auth page redirects if already logged in
  if (pathname === "/auth" && hasSession) {
    return NextResponse.redirect(new URL("/dashboard", req.url))
  }

  // Root redirects
  if (pathname === "/") {
    return NextResponse.redirect(
      new URL(hasSession ? "/dashboard" : "/auth", req.url)
    )
  }

  return NextResponse.next()
}

export const config = {
  matcher: ["/", "/dashboard/:path*", "/auth"],
}
