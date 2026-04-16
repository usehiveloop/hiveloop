import { NextRequest, NextResponse } from "next/server"
import { getSessionFromHeader, createSessionCookie, type SessionData } from "@/lib/auth/session"

const API_URL = process.env.API_URL ?? process.env.NEXT_PUBLIC_API_URL as string

/** Minimum remaining lifetime (ms) before we refresh. */
const MIN_TTL = 5 * 60 * 1000 // 5 minutes

async function refreshTokens(refreshToken: string): Promise<SessionData | null> {
  const res = await fetch(`${API_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
  if (!res.ok) return null
  const data = await res.json()
  if (!data.access_token || !data.refresh_token) return null
  return {
    access_token: data.access_token,
    refresh_token: data.refresh_token,
    expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
  }
}

/**
 * GET /api/auth/stream-token
 *
 * Returns a short-lived access token + org ID for direct backend SSE connections.
 * Refreshes the session if the token is close to expiry and persists the new cookie.
 */
export async function GET(req: NextRequest) {
  const cookieHeader = req.headers.get("cookie")
  let session = await getSessionFromHeader(cookieHeader)

  if (!session) {
    return NextResponse.json({ error: "not_authenticated" }, { status: 401 })
  }

  let setCookie: string | null = null

  // Refresh if token doesn't have enough remaining lifetime for a stream
  if (session.expires_at - Date.now() < MIN_TTL) {
    const refreshed = await refreshTokens(session.refresh_token)
    if (!refreshed) {
      return NextResponse.json({ error: "refresh_failed" }, { status: 401 })
    }
    session = refreshed
    setCookie = await createSessionCookie(refreshed)
  }

  const activeOrg = req.cookies.get("ziraloop_active_org")?.value ?? null

  const res = NextResponse.json({
    access_token: session.access_token,
    org_id: activeOrg,
    expires_at: session.expires_at,
  })

  if (setCookie) {
    res.headers.append("set-cookie", setCookie)
  }

  return res
}
