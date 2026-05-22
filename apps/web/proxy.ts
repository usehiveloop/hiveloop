import { NextRequest, NextResponse } from "next/server"

const SESSION_COOKIE = "__session"

export function proxy(req: NextRequest) {
  const hasSession = req.cookies.has(SESSION_COOKIE)
  const { pathname } = req.nextUrl

  // Protected routes: require a session.
  if (pathname.startsWith("/w") && !hasSession) {
    return NextResponse.redirect(new URL("/auth", req.url))
  }

  if (pathname === "/auth" && hasSession) {
    return NextResponse.redirect(new URL("/w", req.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: ["/w/:path*", "/auth"],
}
