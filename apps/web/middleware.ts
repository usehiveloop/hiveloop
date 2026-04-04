import { NextRequest, NextResponse } from "next/server"

const SESSION_COOKIE = "__session"

export function middleware(req: NextRequest) {
  const hasSession = req.cookies.has(SESSION_COOKIE)
  const { pathname } = req.nextUrl

  // Protected routes — require session
  if (pathname.startsWith("/w") && !hasSession) {
    return NextResponse.redirect(new URL("/auth", req.url))
  }

  // Auth page — redirect if already logged in
  if (pathname === "/auth" && hasSession) {
    return NextResponse.redirect(new URL("/w", req.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: ["/w/:path*", "/auth"],
}
