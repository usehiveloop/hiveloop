import { NextRequest, NextResponse } from "next/server"
import {
  getSessionFromHeader,
  stripSessionCookie,
  createSessionCookie,
  clearSessionCookie,
  type SessionData,
} from "@/lib/auth/session"

const API_URL = process.env.API_URL!

// Paths whose successful responses contain tokens that should be persisted.
const AUTH_PATHS = new Set([
  "auth/login",
  "auth/register",
  "auth/refresh",
])

const LOGOUT_PATH = "auth/logout"

// ---------------------------------------------------------------------------
// Concurrent refresh lock
// ---------------------------------------------------------------------------

let refreshPromise: Promise<SessionData | null> | null = null

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

async function safeRefresh(refreshToken: string): Promise<SessionData | null> {
  if (refreshPromise) return refreshPromise
  refreshPromise = refreshTokens(refreshToken).finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

// ---------------------------------------------------------------------------
// Build headers for upstream request
// ---------------------------------------------------------------------------

function buildUpstreamHeaders(
  req: NextRequest,
  session: SessionData | null
): Headers {
  const headers = new Headers()

  for (const key of ["content-type", "accept"]) {
    const value = req.headers.get(key)
    if (value) headers.set(key, value)
  }

  const rawCookies = req.headers.get("cookie")
  if (rawCookies) {
    const cleaned = stripSessionCookie(rawCookies)
    if (cleaned) headers.set("cookie", cleaned)
  }

  if (session) {
    headers.set("authorization", `Bearer ${session.access_token}`)
  }

  return headers
}

// ---------------------------------------------------------------------------
// Forward a request to the Go backend
// ---------------------------------------------------------------------------

async function forward(
  url: URL,
  method: string,
  headers: Headers,
  body: ArrayBuffer | undefined
) {
  return fetch(url, { method, headers, body })
}

// ---------------------------------------------------------------------------
// Main handler
// ---------------------------------------------------------------------------

async function handler(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params
  const apiPath = path.join("/")

  const url = new URL(`${API_URL}/${apiPath}`)
  req.nextUrl.searchParams.forEach((value, key) =>
    url.searchParams.append(key, value)
  )

  const rawCookies = req.headers.get("cookie")
  let session = await getSessionFromHeader(rawCookies)

  const body =
    req.method !== "GET" && req.method !== "HEAD"
      ? await req.arrayBuffer()
      : undefined

  // -----------------------------------------------------------------------
  // Logout interception: inject refresh_token into request body
  // -----------------------------------------------------------------------
  let upstreamBody: ArrayBuffer | undefined = body
  const isLogout = apiPath === LOGOUT_PATH && req.method === "POST"

  if (isLogout && session) {
    const payload = body ? JSON.parse(new TextDecoder().decode(body)) : {}
    payload.refresh_token = session.refresh_token
    upstreamBody = new TextEncoder().encode(JSON.stringify(payload)).buffer as ArrayBuffer
  }

  // -----------------------------------------------------------------------
  // Proactive refresh if token is about to expire
  // -----------------------------------------------------------------------
  if (session && !AUTH_PATHS.has(apiPath) && session.expires_at - Date.now() < 60_000) {
    const refreshed = await safeRefresh(session.refresh_token)
    if (refreshed) session = refreshed
  }

  // -----------------------------------------------------------------------
  // Forward to backend
  // -----------------------------------------------------------------------
  const headers = buildUpstreamHeaders(req, session)
  let upstream = await forward(url, req.method, headers, upstreamBody)

  // -----------------------------------------------------------------------
  // Auto-refresh on 401 (retry once)
  // -----------------------------------------------------------------------
  let refreshedSession: SessionData | null = null

  if (upstream.status === 401 && session && !AUTH_PATHS.has(apiPath) && !isLogout) {
    const newSession = await safeRefresh(session.refresh_token)
    if (newSession) {
      refreshedSession = newSession
      session = newSession
      const retryHeaders = buildUpstreamHeaders(req, newSession)
      upstream = await forward(url, req.method, retryHeaders, body)
    }
  }

  // -----------------------------------------------------------------------
  // Build response
  // -----------------------------------------------------------------------
  const responseHeaders = new Headers()
  upstream.headers.forEach((value, key) => {
    if (key.toLowerCase() === "transfer-encoding") return
    responseHeaders.set(key, value)
  })

  // -----------------------------------------------------------------------
  // Intercept auth responses — persist session, strip tokens from body
  // -----------------------------------------------------------------------
  if (AUTH_PATHS.has(apiPath) && upstream.ok) {
    const data = await upstream.json()

    if (data.access_token && data.refresh_token) {
      const newSession: SessionData = {
        access_token: data.access_token,
        refresh_token: data.refresh_token,
        expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
      }

      // Build clean response headers — don't carry over content-length
      // from the upstream response since we're returning a different body
      const authHeaders = new Headers()
      authHeaders.append("set-cookie", await createSessionCookie(newSession))

      const { access_token: _a, refresh_token: _r, expires_in: _e, ...safe } = data
      return NextResponse.json(safe, {
        status: upstream.status,
        headers: authHeaders,
      })
    }
  }

  // Attach updated session cookie if we refreshed mid-request
  if (refreshedSession) {
    responseHeaders.append(
      "set-cookie",
      await createSessionCookie(refreshedSession)
    )
  }

  // Proactive refresh — also persist the updated cookie
  if (
    !AUTH_PATHS.has(apiPath) &&
    !refreshedSession &&
    session &&
    rawCookies
  ) {
    const original = await getSessionFromHeader(rawCookies)
    if (original && original.access_token !== session.access_token) {
      responseHeaders.append(
        "set-cookie",
        await createSessionCookie(session)
      )
    }
  }

  // Logout — clear session cookie
  if (isLogout && upstream.ok) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  // Clear session on auth failure when refresh also failed
  if (upstream.status === 401 && session && !refreshedSession && !AUTH_PATHS.has(apiPath)) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  return new NextResponse(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  })
}

export const GET = handler
export const POST = handler
export const PUT = handler
export const PATCH = handler
export const DELETE = handler
